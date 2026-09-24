package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pathlyv1 "github.com/pathlyhq/pathly-k8s-operator/api/v1alpha1"
	pathlyclient "github.com/pathlyhq/pathly-k8s-operator/internal/client"
)

func schemeWithPathly(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := pathlyv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func ptr[T any](v T) *T { return &v }

func seedConnection(ns string) (*pathlyv1.PathlyConnection, *corev1.Secret) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "pathly-token"},
		Data:       map[string][]byte{"token": []byte("sp_test")},
	}
	conn := &pathlyv1.PathlyConnection{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "default", Generation: 1},
		Spec: pathlyv1.PathlyConnectionSpec{
			SecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "pathly-token"},
				Key:                  "token",
			},
		},
	}
	return conn, secret
}

func TestClientFromConnection(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)
	base := "https://api.example.com/"
	conn.Spec.BaseURL = &base
	kube := fake.NewClientBuilder().WithScheme(s).WithObjects(conn, secret).Build()

	cli, err := ClientFromConnection(context.Background(), kube, ns, corev1.LocalObjectReference{Name: "default"})
	if err != nil {
		t.Fatalf("ClientFromConnection: %v", err)
	}
	if cli == nil {
		t.Fatal("nil client")
	}

	if _, err := ClientFromConnection(context.Background(), kube, ns, corev1.LocalObjectReference{}); err == nil {
		t.Fatal("empty connectionRef must fail")
	}
	if _, err := ClientFromConnection(context.Background(), kube, ns, corev1.LocalObjectReference{Name: "missing"}); err == nil {
		t.Fatal("missing connection must fail")
	}
}

func TestTokenFromSecretFallbacks(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "creds"},
		Data:       map[string][]byte{"apiToken": []byte("sp_alt")},
	}
	kube := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	tok, err := tokenFromSecret(context.Background(), kube, ns, corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "creds"},
	})
	if err != nil || tok != "sp_alt" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}

	empty := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "empty"}, Data: map[string][]byte{}}
	kube2 := fake.NewClientBuilder().WithScheme(s).WithObjects(empty).Build()
	if _, err := tokenFromSecret(context.Background(), kube2, ns, corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "empty"},
		Key:                  "token",
	}); err == nil || !strings.Contains(err.Error(), errNoToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestConnectionReconciler(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)
	kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(conn).WithObjects(conn, secret).Build()
	r := &ConnectionReconciler{Client: kube, Scheme: s}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "default"}}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got := &pathlyv1.PathlyConnection{}
	if err := kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "default"}, got); err != nil {
		t.Fatal(err)
	}
	if !got.Status.Ready {
		t.Fatalf("status = %+v", got.Status)
	}

	// Not found is a no-op.
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "gone"}}); err != nil {
		t.Fatalf("missing: %v", err)
	}
}

func factoryForServer(t *testing.T, handler http.HandlerFunc) PathlyClientFactory {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return func(ctx context.Context, kube client.Client, namespace, connectionName string) (*pathlyclient.Client, error) {
		return pathlyclient.New(srv.URL, "sp_test", pathlyclient.WithSleep(func(time.Duration) {})), nil
	}
}

func TestScenarioReconcilerCreateUpdateDelete(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)

	var lastMethod, lastPath string
	var createBody map[string]any
	factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
		lastMethod, lastPath = r.Method, r.URL.Path
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/scenarios":
			_ = json.NewDecoder(r.Body).Decode(&createBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"mon-1","name":"Home","type":"http","url":"https://example.com","enabled":true,"intervalSec":300}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/scenarios/mon-1":
			_, _ = w.Write([]byte(`{"id":"mon-1","name":"Home","type":"http","url":"https://example.com","enabled":true,"intervalSec":300}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/scenarios/mon-1":
			_, _ = w.Write([]byte(`{"id":"mon-1","name":"Home-v2","type":"http","url":"https://example.com","enabled":true,"intervalSec":60}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/scenarios/mon-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})

	sc := &pathlyv1.PathlyScenario{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "home", UID: "uid-1", Generation: 1},
		Spec: pathlyv1.PathlyScenarioSpec{
			ConnectionRef: corev1.LocalObjectReference{Name: "default"},
			Name:          "Home",
			Type:          "http",
			URL:           ptr("https://example.com"),
			IntervalSec:   ptr(int64(300)),
			Enabled:       ptr(true),
		},
	}
	kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sc, conn).WithObjects(conn, secret, sc).Build()
	r := &ScenarioReconciler{Client: kube, Scheme: s, NewClient: factory}

	// Finalizer add
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}})
	if err != nil {
		t.Fatalf("finalizer: %v", err)
	}
	if !res.Requeue {
		t.Fatal("expected requeue after finalizer")
	}

	// Create
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	got := &pathlyv1.PathlyScenario{}
	_ = kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "home"}, got)
	if got.Status.ID != "mon-1" || !got.Status.Ready {
		t.Fatalf("after create: %+v", got.Status)
	}
	if lastMethod != http.MethodPost {
		t.Fatalf("lastMethod=%s", lastMethod)
	}

	// Sync (GET)
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Drift → update
	got = &pathlyv1.PathlyScenario{}
	if err := kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "home"}, got); err != nil {
		t.Fatal(err)
	}
	got.Spec.Name = "Home-v2"
	got.Spec.IntervalSec = ptr(int64(60))
	if err := kube.Update(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if lastMethod != http.MethodPatch {
		t.Fatalf("expected PATCH, got %s %s", lastMethod, lastPath)
	}

	// Delete via Delete API (sets DeletionTimestamp while finalizer keeps the object)
	got = &pathlyv1.PathlyScenario{}
	_ = kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "home"}, got)
	if err := kube.Delete(context.Background(), got); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}}); err != nil {
		t.Fatalf("delete reconcile: %v", err)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestScenarioReconcilerRemoteMissing(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)
	factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})
	sc := &pathlyv1.PathlyScenario{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "home", UID: "uid-2", Generation: 1, Finalizers: []string{finalizerName}},
		Spec: pathlyv1.PathlyScenarioSpec{
			ConnectionRef: corev1.LocalObjectReference{Name: "default"},
			Name:          "Home",
		},
		Status: pathlyv1.PathlyScenarioStatus{ID: "gone"},
	}
	kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sc).WithObjects(conn, secret, sc).Build()
	r := &ScenarioReconciler{Client: kube, Scheme: s, NewClient: factory}
	res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !res.Requeue {
		t.Fatal("expected requeue to recreate")
	}
	got := &pathlyv1.PathlyScenario{}
	_ = kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "home"}, got)
	if got.Status.ID != "" {
		t.Fatalf("ID should be cleared, got %q", got.Status.ID)
	}
}

func TestWebhookReconciler(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)
	factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/webhooks":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"wh-1","events":["run.failed"],"enabled":true,"hasSecret":true,"urlFingerprint":"abc"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/webhooks/wh-1":
			_, _ = w.Write([]byte(`{"id":"wh-1","events":["run.failed"],"enabled":true,"hasSecret":true,"urlFingerprint":"abc"}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	wh := &pathlyv1.PathlyWebhook{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "alerts", UID: "uid-wh", Generation: 1, Finalizers: []string{finalizerName}},
		Spec: pathlyv1.PathlyWebhookSpec{
			ConnectionRef: corev1.LocalObjectReference{Name: "default"},
			URL:           "https://hooks.example.com/pathly",
			Events:        []string{"run.failed"},
		},
	}
	kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(wh).WithObjects(conn, secret, wh).Build()
	r := &WebhookReconciler{Client: kube, Scheme: s, NewClient: factory}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "alerts"}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	got := &pathlyv1.PathlyWebhook{}
	_ = kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "alerts"}, got)
	if got.Status.ID != "wh-1" || !got.Status.Ready {
		t.Fatalf("%+v", got.Status)
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "alerts"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func TestMaintenanceWindowReconciler(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)
	factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"mw-1","weekday":0,"startMinute":180,"durationMin":120,"reason":"backup"}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"mw-1","weekday":0,"startMinute":180,"durationMin":120,"reason":"backup"}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mw := &pathlyv1.PathlyMaintenanceWindow{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "backup", UID: "uid-mw", Generation: 1, Finalizers: []string{finalizerName}},
		Spec: pathlyv1.PathlyMaintenanceWindowSpec{
			ConnectionRef: corev1.LocalObjectReference{Name: "default"},
			Weekday:       ptr(int64(0)),
			StartMinute:   ptr(int64(180)),
			DurationMin:   ptr(int64(120)),
			Reason:        ptr("backup"),
		},
	}
	kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(mw).WithObjects(conn, secret, mw).Build()
	r := &MaintenanceWindowReconciler{Client: kube, Scheme: s, NewClient: factory}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "backup"}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	got := &pathlyv1.PathlyMaintenanceWindow{}
	_ = kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "backup"}, got)
	if got.Status.ID != "mw-1" {
		t.Fatalf("%+v", got.Status)
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "backup"}}); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func TestSlaTargetReconciler(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)
	factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"id":"sla-1","monitorId":"mon-1","name":"99.9","objectivePct":99.9,"windowDays":30,"enabled":true}`))
	})
	sla := &pathlyv1.PathlySlaTarget{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "obj", UID: "uid-sla", Generation: 1, Finalizers: []string{finalizerName}},
		Spec: pathlyv1.PathlySlaTargetSpec{
			ConnectionRef: corev1.LocalObjectReference{Name: "default"},
			MonitorID:     ptr("mon-1"),
			Name:          ptr("99.9"),
			ObjectivePct:  99.9,
			WindowDays:    30,
			Enabled:       ptr(true),
		},
	}
	kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sla).WithObjects(conn, secret, sla).Build()
	r := &SlaTargetReconciler{Client: kube, Scheme: s, NewClient: factory}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "obj"}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got := &pathlyv1.PathlySlaTarget{}
	_ = kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "obj"}, got)
	if got.Status.ID != "sla-1" || !got.Status.Ready {
		t.Fatalf("%+v", got.Status)
	}
}

func TestHelpers(t *testing.T) {
	if CopyStringPtr(nil) != nil {
		t.Fatal("nil string")
	}
	if CopyInt64Ptr(nil) != nil || CopyBoolPtr(nil) != nil || CopyFloat64Ptr(nil) != nil {
		t.Fatal("nil ptrs")
	}
	s := "x"
	i := int64(1)
	b := true
	f := 1.5
	if *CopyStringPtr(&s) != "x" || *CopyInt64Ptr(&i) != 1 || !*CopyBoolPtr(&b) || *CopyFloat64Ptr(&f) != 1.5 {
		t.Fatal("copy")
	}
	if statusMessage(nil) != "synced" {
		t.Fatal(statusMessage(nil))
	}
	key := idempotencyKey("uid", "create")
	if len(key) != 32 {
		t.Fatalf("key len %d", len(key))
	}
	_ = stringPtr("y")

	want := pathlyv1.PathlyScenarioSpec{Name: "A", URL: ptr("https://a"), Enabled: ptr(true), IntervalSec: ptr(int64(60)), Method: ptr("GET"), ExpectedStatus: ptr(int64(200))}
	got := &pathlyclient.Scenario{Name: "A", URL: ptr("https://a"), Enabled: ptr(true), IntervalSec: ptr(int64(60)), Method: ptr("GET"), ExpectedStatus: ptr(int64(200))}
	if !scenarioUpToDate(want, got) {
		t.Fatal("should be up to date")
	}
	got.Name = "B"
	if scenarioUpToDate(want, got) {
		t.Fatal("name drift")
	}
}

func TestResolveClientDefault(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)
	kube := fake.NewClientBuilder().WithScheme(s).WithObjects(conn, secret).Build()
	cli, err := resolveClient(nil, context.Background(), kube, ns, "default")
	if err != nil || cli == nil {
		t.Fatalf("cli=%v err=%v", cli, err)
	}
}

func TestScenarioInputDefaults(t *testing.T) {
	in := scenarioInput(pathlyv1.PathlyScenarioSpec{Name: "x"})
	if in.Type == nil || *in.Type != "http" {
		t.Fatalf("%+v", in)
	}
}

func TestScenarioReconcilerMissingConnection(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	sc := &pathlyv1.PathlyScenario{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "home", UID: "uid-x", Generation: 1, Finalizers: []string{finalizerName}},
		Spec: pathlyv1.PathlyScenarioSpec{
			ConnectionRef: corev1.LocalObjectReference{Name: "missing"},
			Name:          "Home",
		},
	}
	kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sc).WithObjects(sc).Build()
	r := &ScenarioReconciler{Client: kube, Scheme: s}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}}); err == nil {
		t.Fatal("expected error")
	}
	got := &pathlyv1.PathlyScenario{}
	_ = kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "home"}, got)
	if got.Status.Ready {
		t.Fatal("should not be ready")
	}
}

func TestWebhookAndMWDeleteAndRemoteMissing(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)

	t.Run("webhook delete and recreate", func(t *testing.T) {
		n404 := 0
		factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if r.Method == http.MethodGet {
				n404++
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"gone"}`))
				return
			}
			http.NotFound(w, r)
		})
		wh := &pathlyv1.PathlyWebhook{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "alerts", UID: "uid-wh2", Generation: 1, Finalizers: []string{finalizerName}},
			Spec:       pathlyv1.PathlyWebhookSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, URL: "https://hooks.example.com"},
			Status:     pathlyv1.PathlyWebhookStatus{ID: "wh-gone"},
		}
		kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(wh).WithObjects(conn.DeepCopy(), secret.DeepCopy(), wh).Build()
		r := &WebhookReconciler{Client: kube, Scheme: s, NewClient: factory}
		res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "alerts"}})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Requeue {
			t.Fatal("expected requeue")
		}
		_ = n404

		got := &pathlyv1.PathlyWebhook{}
		_ = kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "alerts"}, got)
		if err := kube.Delete(context.Background(), got); err != nil {
			t.Fatal(err)
		}
		// After clear ID, delete path with empty ID
		got = &pathlyv1.PathlyWebhook{}
		_ = kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "alerts"}, got)
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "alerts"}}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("mw delete", func(t *testing.T) {
		factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"gone"}`))
				return
			}
			http.NotFound(w, r)
		})
		mw := &pathlyv1.PathlyMaintenanceWindow{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "backup", UID: "uid-mw2", Generation: 1, Finalizers: []string{finalizerName}},
			Spec:       pathlyv1.PathlyMaintenanceWindowSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}},
			Status:     pathlyv1.PathlyMaintenanceWindowStatus{ID: "mw-x"},
		}
		kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(mw).WithObjects(conn.DeepCopy(), secret.DeepCopy(), mw).Build()
		r := &MaintenanceWindowReconciler{Client: kube, Scheme: s, NewClient: factory}
		res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "backup"}})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Requeue {
			t.Fatal("requeue")
		}
		got := &pathlyv1.PathlyMaintenanceWindow{}
		_ = kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "backup"}, got)
		_ = kube.Delete(context.Background(), got)
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "backup"}}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("sla delete", func(t *testing.T) {
		factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		})
		sla := &pathlyv1.PathlySlaTarget{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "obj", UID: "uid-sla2", Generation: 1, Finalizers: []string{finalizerName}},
			Spec: pathlyv1.PathlySlaTargetSpec{
				ConnectionRef: corev1.LocalObjectReference{Name: "default"},
				ObjectivePct:  99.9,
				WindowDays:    30,
			},
			Status: pathlyv1.PathlySlaTargetStatus{ID: "sla-x"},
		}
		kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sla).WithObjects(conn.DeepCopy(), secret.DeepCopy(), sla).Build()
		r := &SlaTargetReconciler{Client: kube, Scheme: s, NewClient: factory}
		_ = kube.Delete(context.Background(), sla)
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "obj"}}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestScenarioUpToDateBranches(t *testing.T) {
	base := pathlyv1.PathlyScenarioSpec{
		Name: "A", URL: ptr("https://a"), Enabled: ptr(true),
		IntervalSec: ptr(int64(60)), Method: ptr("GET"), ExpectedStatus: ptr(int64(200)),
	}
	ok := &pathlyclient.Scenario{
		Name: "A", URL: ptr("https://a"), Enabled: ptr(true),
		IntervalSec: ptr(int64(60)), Method: ptr("GET"), ExpectedStatus: ptr(int64(200)),
	}
	cases := []struct {
		mutate func(*pathlyclient.Scenario)
	}{
		{func(g *pathlyclient.Scenario) { g.URL = nil }},
		{func(g *pathlyclient.Scenario) { g.Enabled = nil }},
		{func(g *pathlyclient.Scenario) { g.IntervalSec = nil }},
		{func(g *pathlyclient.Scenario) { g.Method = nil }},
		{func(g *pathlyclient.Scenario) { g.ExpectedStatus = nil }},
	}
	for _, c := range cases {
		g := *ok
		c.mutate(&g)
		if scenarioUpToDate(base, &g) {
			t.Fatal("expected drift")
		}
	}
}

func TestConnectionReconcilerNotReady(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn := &pathlyv1.PathlyConnection{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "default", Generation: 1},
		Spec: pathlyv1.PathlyConnectionSpec{
			SecretRef: corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "missing"}, Key: "token"},
		},
	}
	kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(conn).WithObjects(conn).Build()
	r := &ConnectionReconciler{Client: kube, Scheme: s}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "default"}}); err != nil {
		t.Fatal(err)
	}
	got := &pathlyv1.PathlyConnection{}
	_ = kube.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "default"}, got)
	if got.Status.Ready {
		t.Fatal("expected not ready")
	}
}

func TestNotFoundReconciles(t *testing.T) {
	s := schemeWithPathly(t)
	kube := fake.NewClientBuilder().WithScheme(s).Build()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "x"}}
	for _, r := range []reconcile.Reconciler{
		&ScenarioReconciler{Client: kube, Scheme: s},
		&WebhookReconciler{Client: kube, Scheme: s},
		&MaintenanceWindowReconciler{Client: kube, Scheme: s},
		&SlaTargetReconciler{Client: kube, Scheme: s},
	} {
		if _, err := r.Reconcile(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWebhookCreateFails(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)
	factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	})
	wh := &pathlyv1.PathlyWebhook{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "alerts", UID: "uid", Generation: 1, Finalizers: []string{finalizerName}},
		Spec:       pathlyv1.PathlyWebhookSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, URL: "https://x"},
	}
	kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(wh).WithObjects(conn, secret, wh).Build()
	r := &WebhookReconciler{Client: kube, Scheme: s, NewClient: factory}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "alerts"}}); err == nil {
		t.Fatal("expected error")
	}
}

func TestDeepCopySmoke(t *testing.T) {
	conn, secret := seedConnection("ns")
	_ = secret
	if conn.DeepCopy() == nil || conn.DeepCopyObject() == nil {
		t.Fatal("connection deepcopy")
	}
	sc := &pathlyv1.PathlyScenario{Spec: pathlyv1.PathlyScenarioSpec{Name: "n", Regions: []string{"eu"}, Tags: []string{"t"}, URL: ptr("u"), Enabled: ptr(true)}}
	if sc.DeepCopy().Spec.Name != "n" {
		t.Fatal("scenario")
	}
	wh := &pathlyv1.PathlyWebhook{Spec: pathlyv1.PathlyWebhookSpec{URL: "u", Events: []string{"e"}}}
	_ = wh.DeepCopy()
	mw := &pathlyv1.PathlyMaintenanceWindow{Spec: pathlyv1.PathlyMaintenanceWindowSpec{Reason: ptr("r"), Weekday: ptr(int64(1))}}
	_ = mw.DeepCopy()
	sla := &pathlyv1.PathlySlaTarget{Spec: pathlyv1.PathlySlaTargetSpec{ObjectivePct: 99, WindowDays: 30, Enabled: ptr(true)}}
	_ = sla.DeepCopy()
	_ = (&pathlyv1.PathlyConnectionList{Items: []pathlyv1.PathlyConnection{*conn}}).DeepCopy()
	_ = (&pathlyv1.PathlyScenarioList{Items: []pathlyv1.PathlyScenario{*sc}}).DeepCopy()
	_ = (&pathlyv1.PathlyWebhookList{Items: []pathlyv1.PathlyWebhook{*wh}}).DeepCopy()
	_ = (&pathlyv1.PathlyMaintenanceWindowList{Items: []pathlyv1.PathlyMaintenanceWindow{*mw}}).DeepCopy()
	_ = (&pathlyv1.PathlySlaTargetList{Items: []pathlyv1.PathlySlaTarget{*sla}}).DeepCopy()
}
