package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	pathlyv1 "github.com/pathlyhq/pathly-k8s-operator/api/v1alpha1"
	pathlyclient "github.com/pathlyhq/pathly-k8s-operator/internal/client"
)

func testManager(t *testing.T) ctrl.Manager {
	t.Helper()
	mgr, err := ctrl.NewManager(&rest.Config{}, ctrl.Options{
		Scheme:                 schemeWithPathly(t),
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}

func TestSetupWithManagerAll(t *testing.T) {
	mgr := testManager(t)
	s := mgr.GetScheme()
	if err := (&ConnectionReconciler{Client: mgr.GetClient(), Scheme: s}).SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	if err := (&ScenarioReconciler{Client: mgr.GetClient(), Scheme: s}).SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	if err := (&WebhookReconciler{Client: mgr.GetClient(), Scheme: s}).SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	if err := (&MaintenanceWindowReconciler{Client: mgr.GetClient(), Scheme: s}).SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	if err := (&SlaTargetReconciler{Client: mgr.GetClient(), Scheme: s}).SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
}

func TestClientFromConnectionTokenError(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn := &pathlyv1.PathlyConnection{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "default"},
		Spec: pathlyv1.PathlyConnectionSpec{
			SecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "missing-secret"},
				Key:                  "token",
			},
		},
	}
	kube := fake.NewClientBuilder().WithScheme(s).WithObjects(conn).Build()
	_, err := ClientFromConnection(context.Background(), kube, ns, corev1.LocalObjectReference{Name: "default"})
	if err == nil {
		t.Fatal("expected token error")
	}
}

func TestConnectionReconcileGetErrorAndStatusFail(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)

	t.Run("get non-notfound", func(t *testing.T) {
		kube := fake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return errors.New("apiserver down")
			},
		}).Build()
		r := &ConnectionReconciler{Client: kube, Scheme: s}
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "default"}}); err == nil {
			t.Fatal("expected get error")
		}
	})

	t.Run("status update fails", func(t *testing.T) {
		kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(conn).WithObjects(conn.DeepCopy(), secret.DeepCopy()).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					return errors.New("status write failed")
				},
			}).Build()
		r := &ConnectionReconciler{Client: kube, Scheme: s}
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "default"}}); err == nil {
			t.Fatal("expected status error")
		}
	})
}

func TestFinalizerUpdateFailures(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)
	factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	cases := []struct {
		name string
		obj  client.Object
		rec  func(client.Client) reconcile.Reconciler
	}{
		{
			name: "scenario",
			obj: &pathlyv1.PathlyScenario{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "home", UID: "u1", Generation: 1},
				Spec:       pathlyv1.PathlyScenarioSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, Name: "Home"},
			},
			rec: func(c client.Client) reconcile.Reconciler {
				return &ScenarioReconciler{Client: c, Scheme: s, NewClient: factory}
			},
		},
		{
			name: "webhook",
			obj: &pathlyv1.PathlyWebhook{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "wh", UID: "u2", Generation: 1},
				Spec:       pathlyv1.PathlyWebhookSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, URL: "https://x"},
			},
			rec: func(c client.Client) reconcile.Reconciler {
				return &WebhookReconciler{Client: c, Scheme: s, NewClient: factory}
			},
		},
		{
			name: "mw",
			obj: &pathlyv1.PathlyMaintenanceWindow{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "mw", UID: "u3", Generation: 1},
				Spec:       pathlyv1.PathlyMaintenanceWindowSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}},
			},
			rec: func(c client.Client) reconcile.Reconciler {
				return &MaintenanceWindowReconciler{Client: c, Scheme: s, NewClient: factory}
			},
		},
		{
			name: "sla",
			obj: &pathlyv1.PathlySlaTarget{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "sla", UID: "u4", Generation: 1},
				Spec:       pathlyv1.PathlySlaTargetSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, ObjectivePct: 99, WindowDays: 30},
			},
			rec: func(c client.Client) reconcile.Reconciler {
				return &SlaTargetReconciler{Client: c, Scheme: s, NewClient: factory}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"-finalizer-update-fail", func(t *testing.T) {
			kube := fake.NewClientBuilder().WithScheme(s).WithObjects(conn.DeepCopy(), secret.DeepCopy(), tc.obj).
				WithInterceptorFuncs(interceptor.Funcs{
					Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
						return errors.New("update denied")
					},
				}).Build()
			r := tc.rec(kube)
			nn := types.NamespacedName{Namespace: ns, Name: tc.obj.GetName()}
			if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: nn}); err == nil {
				t.Fatal("expected update error")
			}
		})

		t.Run(tc.name+"-finalizer-ok", func(t *testing.T) {
			obj := tc.obj.DeepCopyObject().(client.Object)
			obj.SetResourceVersion("")
			kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(obj).WithObjects(conn.DeepCopy(), secret.DeepCopy(), obj).Build()
			r := tc.rec(kube)
			nn := types.NamespacedName{Namespace: ns, Name: obj.GetName()}
			res, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: nn})
			if err != nil {
				t.Fatal(err)
			}
			if !res.Requeue {
				t.Fatal("expected requeue after finalizer")
			}
		})
	}
}

func TestReconcileGetNonNotFound(t *testing.T) {
	s := schemeWithPathly(t)
	kube := fake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			return errors.New("boom get")
		},
	}).Build()
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "x"}}
	for _, r := range []reconcile.Reconciler{
		&ScenarioReconciler{Client: kube, Scheme: s},
		&WebhookReconciler{Client: kube, Scheme: s},
		&MaintenanceWindowReconciler{Client: kube, Scheme: s},
		&SlaTargetReconciler{Client: kube, Scheme: s},
	} {
		if _, err := r.Reconcile(context.Background(), req); err == nil {
			t.Fatal("expected get error")
		}
	}
}

func TestDeleteRemoteHardErrorsAndUpdateFail(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)
	hard := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
			return
		}
		http.NotFound(w, r)
	})

	type delCase struct {
		name string
		obj  client.Object
		rec  func(client.Client) reconcile.Reconciler
	}
	cases := []delCase{
		{
			name: "scenario",
			obj: &pathlyv1.PathlyScenario{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "home", UID: "d1", Generation: 1, Finalizers: []string{finalizerName}},
				Spec:       pathlyv1.PathlyScenarioSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, Name: "H"},
				Status:     pathlyv1.PathlyScenarioStatus{ID: "mon-1"},
			},
			rec: func(c client.Client) reconcile.Reconciler {
				return &ScenarioReconciler{Client: c, Scheme: s, NewClient: hard}
			},
		},
		{
			name: "webhook",
			obj: &pathlyv1.PathlyWebhook{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "wh", UID: "d2", Generation: 1, Finalizers: []string{finalizerName}},
				Spec:       pathlyv1.PathlyWebhookSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, URL: "https://x"},
				Status:     pathlyv1.PathlyWebhookStatus{ID: "wh-1"},
			},
			rec: func(c client.Client) reconcile.Reconciler {
				return &WebhookReconciler{Client: c, Scheme: s, NewClient: hard}
			},
		},
		{
			name: "mw",
			obj: &pathlyv1.PathlyMaintenanceWindow{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "mw", UID: "d3", Generation: 1, Finalizers: []string{finalizerName}},
				Spec:       pathlyv1.PathlyMaintenanceWindowSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}},
				Status:     pathlyv1.PathlyMaintenanceWindowStatus{ID: "mw-1"},
			},
			rec: func(c client.Client) reconcile.Reconciler {
				return &MaintenanceWindowReconciler{Client: c, Scheme: s, NewClient: hard}
			},
		},
		{
			name: "sla",
			obj: &pathlyv1.PathlySlaTarget{
				ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "sla", UID: "d4", Generation: 1, Finalizers: []string{finalizerName}},
				Spec:       pathlyv1.PathlySlaTargetSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, ObjectivePct: 99, WindowDays: 7},
				Status:     pathlyv1.PathlySlaTargetStatus{ID: "sla-1"},
			},
			rec: func(c client.Client) reconcile.Reconciler {
				return &SlaTargetReconciler{Client: c, Scheme: s, NewClient: hard}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"-delete-hard", func(t *testing.T) {
			obj := tc.obj.DeepCopyObject().(client.Object)
			kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(obj).WithObjects(conn.DeepCopy(), secret.DeepCopy(), obj).Build()
			_ = kube.Delete(context.Background(), obj)
			r := tc.rec(kube)
			if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: obj.GetName()}}); err == nil {
				t.Fatal("expected delete hard error")
			}
		})

		t.Run(tc.name+"-delete-update-fail", func(t *testing.T) {
			okDel := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			obj := tc.obj.DeepCopyObject().(client.Object)
			var rec reconcile.Reconciler
			switch tc.name {
			case "scenario":
				rec = &ScenarioReconciler{Client: nil, Scheme: s, NewClient: okDel}
			case "webhook":
				rec = &WebhookReconciler{Client: nil, Scheme: s, NewClient: okDel}
			case "mw":
				rec = &MaintenanceWindowReconciler{Client: nil, Scheme: s, NewClient: okDel}
			case "sla":
				rec = &SlaTargetReconciler{Client: nil, Scheme: s, NewClient: okDel}
			}
			kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(obj).WithObjects(conn.DeepCopy(), secret.DeepCopy(), obj).
				WithInterceptorFuncs(interceptor.Funcs{
					Update: func(ctx context.Context, c client.WithWatch, o client.Object, opts ...client.UpdateOption) error {
						return errors.New("cannot remove finalizer")
					},
				}).Build()
			_ = kube.Delete(context.Background(), obj)
			switch r := rec.(type) {
			case *ScenarioReconciler:
				r.Client = kube
				rec = r
			case *WebhookReconciler:
				r.Client = kube
				rec = r
			case *MaintenanceWindowReconciler:
				r.Client = kube
				rec = r
			case *SlaTargetReconciler:
				r.Client = kube
				rec = r
			}
			if _, err := rec.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: obj.GetName()}}); err == nil {
				t.Fatal("expected finalizer update error")
			}
		})
	}
}

func TestCreateUpdateGetFailuresAndStatusWrite(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)

	failCreate := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	})

	t.Run("scenario create fail", func(t *testing.T) {
		sc := &pathlyv1.PathlyScenario{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "home", UID: "c1", Generation: 1, Finalizers: []string{finalizerName}},
			Spec:       pathlyv1.PathlyScenarioSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, Name: "H"},
		}
		kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sc).WithObjects(conn.DeepCopy(), secret.DeepCopy(), sc).Build()
		r := &ScenarioReconciler{Client: kube, Scheme: s, NewClient: failCreate}
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}}); err == nil {
			t.Fatal("expected create fail")
		}
	})

	t.Run("scenario create status fail", func(t *testing.T) {
		ok := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"mon-1","name":"H","type":"http"}`))
		})
		sc := &pathlyv1.PathlyScenario{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "home", UID: "c2", Generation: 1, Finalizers: []string{finalizerName}},
			Spec:       pathlyv1.PathlyScenarioSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, Name: "H"},
		}
		kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sc).WithObjects(conn.DeepCopy(), secret.DeepCopy(), sc).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					return errors.New("status fail")
				},
			}).Build()
		r := &ScenarioReconciler{Client: kube, Scheme: s, NewClient: ok}
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}}); err == nil {
			t.Fatal("expected status fail")
		}
	})

	t.Run("scenario get hard + update fail + sync status fail", func(t *testing.T) {
		calls := 0
		factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			switch {
			case r.Method == http.MethodGet:
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"getfail"}`))
			case r.Method == http.MethodPatch:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"patchfail"}`))
			default:
				_, _ = w.Write([]byte(`{"id":"mon-1","name":"Old","type":"http","url":"https://a","enabled":true,"intervalSec":1}`))
			}
		})
		sc := &pathlyv1.PathlyScenario{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "home", UID: "c3", Generation: 1, Finalizers: []string{finalizerName}},
			Spec:       pathlyv1.PathlyScenarioSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, Name: "New", URL: ptr("https://b")},
			Status:     pathlyv1.PathlyScenarioStatus{ID: "mon-1"},
		}
		kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sc).WithObjects(conn.DeepCopy(), secret.DeepCopy(), sc).Build()
		r := &ScenarioReconciler{Client: kube, Scheme: s, NewClient: factory}
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}}); err == nil {
			t.Fatal("expected get hard fail")
		}
		_ = calls

		// update failure path: GET ok but PATCH fails
		factory2 := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"id":"mon-1","name":"Old","type":"http","url":"https://a"}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"patch"}`))
		})
		sc2 := sc.DeepCopy()
		sc2.ResourceVersion = ""
		kube2 := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sc2).WithObjects(conn.DeepCopy(), secret.DeepCopy(), sc2).Build()
		r2 := &ScenarioReconciler{Client: kube2, Scheme: s, NewClient: factory2}
		if _, err := r2.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}}); err == nil {
			t.Fatal("expected patch fail")
		}

		// sync status fail when up to date
		factory3 := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":"mon-1","name":"New","type":"http","url":"https://b"}`))
		})
		sc3 := &pathlyv1.PathlyScenario{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "home", UID: "c4", Generation: 1, Finalizers: []string{finalizerName}},
			Spec:       pathlyv1.PathlyScenarioSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, Name: "New", URL: ptr("https://b")},
			Status:     pathlyv1.PathlyScenarioStatus{ID: "mon-1"},
		}
		kube3 := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sc3).WithObjects(conn.DeepCopy(), secret.DeepCopy(), sc3).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					return errors.New("status sync fail")
				},
			}).Build()
		r3 := &ScenarioReconciler{Client: kube3, Scheme: s, NewClient: factory3}
		if _, err := r3.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}}); err == nil {
			t.Fatal("expected sync status fail")
		}
	})

	t.Run("webhook mw sla create status get sync fail", func(t *testing.T) {
		// webhook: missing connection, create status fail, get hard, sync status fail
		wh := &pathlyv1.PathlyWebhook{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "wh", UID: "w1", Generation: 1, Finalizers: []string{finalizerName}},
			Spec:       pathlyv1.PathlyWebhookSpec{ConnectionRef: corev1.LocalObjectReference{Name: "missing"}, URL: "https://x"},
		}
		kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(wh).WithObjects(wh).Build()
		if _, err := (&WebhookReconciler{Client: kube, Scheme: s}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "wh"}}); err == nil {
			t.Fatal("webhook missing conn")
		}

		okCreate := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":"wh-1","events":["run.failed"]}`))
				return
			}
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"get"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"wh-1","events":["run.failed"]}`))
		})
		wh2 := &pathlyv1.PathlyWebhook{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "wh2", UID: "w2", Generation: 1, Finalizers: []string{finalizerName}},
			Spec:       pathlyv1.PathlyWebhookSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, URL: "https://x"},
		}
		kube2 := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(wh2).WithObjects(conn.DeepCopy(), secret.DeepCopy(), wh2).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					return errors.New("status")
				},
			}).Build()
		if _, err := (&WebhookReconciler{Client: kube2, Scheme: s, NewClient: okCreate}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "wh2"}}); err == nil {
			t.Fatal("webhook create status")
		}

		wh3 := &pathlyv1.PathlyWebhook{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "wh3", UID: "w3", Generation: 1, Finalizers: []string{finalizerName}},
			Spec:       pathlyv1.PathlyWebhookSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, URL: "https://x"},
			Status:     pathlyv1.PathlyWebhookStatus{ID: "wh-1"},
		}
		kube3 := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(wh3).WithObjects(conn.DeepCopy(), secret.DeepCopy(), wh3).Build()
		if _, err := (&WebhookReconciler{Client: kube3, Scheme: s, NewClient: okCreate}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "wh3"}}); err == nil {
			t.Fatal("webhook get hard")
		}

		okGet := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":"wh-1","events":["run.failed"]}`))
		})
		kube4 := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(wh3).WithObjects(conn.DeepCopy(), secret.DeepCopy(), wh3.DeepCopy()).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					return errors.New("sync status")
				},
			}).Build()
		if _, err := (&WebhookReconciler{Client: kube4, Scheme: s, NewClient: okGet}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "wh3"}}); err == nil {
			t.Fatal("webhook sync status")
		}

		// MW: missing conn, create fail, create status fail, get hard, sync status fail
		mw := &pathlyv1.PathlyMaintenanceWindow{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "mw", UID: "m1", Generation: 1, Finalizers: []string{finalizerName}},
			Spec:       pathlyv1.PathlyMaintenanceWindowSpec{ConnectionRef: corev1.LocalObjectReference{Name: "nope"}},
		}
		kubeMW := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(mw).WithObjects(mw).Build()
		if _, err := (&MaintenanceWindowReconciler{Client: kubeMW, Scheme: s}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "mw"}}); err == nil {
			t.Fatal("mw missing")
		}
		mw2 := mw.DeepCopy()
		mw2.Name = "mw2"
		mw2.Spec.ConnectionRef.Name = "default"
		mw2.ResourceVersion = ""
		kubeMW2 := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(mw2).WithObjects(conn.DeepCopy(), secret.DeepCopy(), mw2).Build()
		if _, err := (&MaintenanceWindowReconciler{Client: kubeMW2, Scheme: s, NewClient: failCreate}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "mw2"}}); err == nil {
			t.Fatal("mw create fail")
		}

		okMW := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"id":"mw-1","reason":"x"}`))
				return
			}
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"g"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"mw-1"}`))
		})
		mw3 := mw2.DeepCopy()
		mw3.Name = "mw3"
		mw3.ResourceVersion = ""
		kubeMW3 := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(mw3).WithObjects(conn.DeepCopy(), secret.DeepCopy(), mw3).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					return errors.New("status")
				},
			}).Build()
		if _, err := (&MaintenanceWindowReconciler{Client: kubeMW3, Scheme: s, NewClient: okMW}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "mw3"}}); err == nil {
			t.Fatal("mw create status")
		}
		mw4 := mw2.DeepCopy()
		mw4.Name = "mw4"
		mw4.Status.ID = "mw-1"
		mw4.ResourceVersion = ""
		kubeMW4 := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(mw4).WithObjects(conn.DeepCopy(), secret.DeepCopy(), mw4).Build()
		if _, err := (&MaintenanceWindowReconciler{Client: kubeMW4, Scheme: s, NewClient: okMW}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "mw4"}}); err == nil {
			t.Fatal("mw get hard")
		}
		okMWGet := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":"mw-1","reason":"x"}`))
		})
		kubeMW5 := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(mw4).WithObjects(conn.DeepCopy(), secret.DeepCopy(), mw4.DeepCopy()).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					return errors.New("sync")
				},
			}).Build()
		if _, err := (&MaintenanceWindowReconciler{Client: kubeMW5, Scheme: s, NewClient: okMWGet}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "mw4"}}); err == nil {
			t.Fatal("mw sync status")
		}

		// SLA
		sla := &pathlyv1.PathlySlaTarget{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "sla", UID: "s1", Generation: 1, Finalizers: []string{finalizerName}},
			Spec:       pathlyv1.PathlySlaTargetSpec{ConnectionRef: corev1.LocalObjectReference{Name: "no"}, ObjectivePct: 99, WindowDays: 30},
		}
		kubeS := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sla).WithObjects(sla).Build()
		if _, err := (&SlaTargetReconciler{Client: kubeS, Scheme: s}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "sla"}}); err == nil {
			t.Fatal("sla missing")
		}
		sla2 := sla.DeepCopy()
		sla2.Name = "sla2"
		sla2.Spec.ConnectionRef.Name = "default"
		sla2.ResourceVersion = ""
		kubeS2 := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sla2).WithObjects(conn.DeepCopy(), secret.DeepCopy(), sla2).Build()
		if _, err := (&SlaTargetReconciler{Client: kubeS2, Scheme: s, NewClient: failCreate}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "sla2"}}); err == nil {
			t.Fatal("sla upsert fail")
		}
		okSLA := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":"sla-1","objectivePct":99,"windowDays":30}`))
		})
		kubeS3 := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sla2).WithObjects(conn.DeepCopy(), secret.DeepCopy(), sla2.DeepCopy()).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
					return errors.New("status")
				},
			}).Build()
		if _, err := (&SlaTargetReconciler{Client: kubeS3, Scheme: s, NewClient: okSLA}).Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "sla2"}}); err == nil {
			t.Fatal("sla status")
		}
	})
}

func TestScenarioDeleteNotFoundOK(t *testing.T) {
	s := schemeWithPathly(t)
	ns := "demo"
	conn, secret := seedConnection(ns)
	factory := factoryForServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"gone"}`))
	})
	sc := &pathlyv1.PathlyScenario{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "home", UID: "z", Generation: 1, Finalizers: []string{finalizerName}},
		Spec:       pathlyv1.PathlyScenarioSpec{ConnectionRef: corev1.LocalObjectReference{Name: "default"}, Name: "H"},
		Status:     pathlyv1.PathlyScenarioStatus{ID: "mon-x"},
	}
	kube := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(sc).WithObjects(conn, secret, sc).Build()
	_ = kube.Delete(context.Background(), sc)
	r := &ScenarioReconciler{Client: kube, Scheme: s, NewClient: factory}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "home"}}); err != nil {
		t.Fatal(err)
	}
}

// silence unused import if interceptor types need schema
var (
	_ = apierrors.IsNotFound
	_ = schema.GroupVersionKind{}
	_ = httptest.NewServer
	_ = pathlyclient.DefaultBaseURL
	_ = time.Second
	_ = runtime.Scheme{}
)
