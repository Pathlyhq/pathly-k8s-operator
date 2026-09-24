package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient wires a client against a test server, with no real waiting.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *[]time.Duration) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	var waits []time.Duration
	c := New(srv.URL, "sp_test_token", WithSleep(func(d time.Duration) {
		waits = append(waits, d)
	}))
	return c, &waits
}

func TestAuthorizationHeaderAndUserAgent(t *testing.T) {
	var gotAuth, gotUA, gotAccept string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{"planId":"pro"}`))
	})

	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if gotAuth != "Bearer sp_test_token" {
		t.Errorf("Authorization header = %q", gotAuth)
	}
	if !strings.Contains(gotUA, "pathly-k8s-operator") {
		t.Errorf("User-Agent = %q, it must identify the provider in the API logs", gotUA)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
}

func TestBaseURLFallsBackToProduction(t *testing.T) {
	c := New("   ", "sp_x")
	if c.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, DefaultBaseURL)
	}
	// A trailing slash in the configuration must not produce `//v1/...`.
	if got := New("https://api.example.com/", "sp_x").baseURL; got != "https://api.example.com" {
		t.Errorf("baseURL = %q", got)
	}
}

func TestCreateScenarioSendsIdempotencyKey(t *testing.T) {
	var gotKey, gotBody, gotType string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		gotType = r.Header.Get("Content-Type")
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"mon-1","name":"Checkout","type":"http","url":"https://example.com"}`))
	})

	name := "Checkout"
	got, err := c.CreateScenario(context.Background(), ScenarioInput{Name: &name}, "idem-key-0001")
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	if gotKey != "idem-key-0001" {
		t.Errorf("Idempotency-Key = %q: without it, a replayed apply creates a duplicate", gotKey)
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q", gotType)
	}
	// Absent fields must not be sent: the API would otherwise keep a zero in
	// place of the existing setting.
	if strings.Contains(gotBody, "intervalSec") {
		t.Errorf("body = %q, it must only carry the fields that were set", gotBody)
	}
	if got.ID != "mon-1" || got.URL == nil || *got.URL != "https://example.com" {
		t.Errorf("decoded scenario = %+v", got)
	}
}

func TestRetryHonoursRetryAfter(t *testing.T) {
	calls := 0
	c, waits := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"Too many calls for this key."}`))
			return
		}
		_, _ = w.Write([]byte(`{"planId":"pro"}`))
	})

	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping after a 429: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want a retry", calls)
	}
	if len(*waits) != 1 || (*waits)[0] != 7*time.Second {
		t.Errorf("waits = %v, the Retry-After header must win", *waits)
	}
}

func TestRetryAfterIsCapped(t *testing.T) {
	c, waits := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "86400")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	// An absurd value must not freeze the pipeline for a day.
	_ = c.Ping(context.Background())
	for _, d := range *waits {
		if d > maxRetryWait {
			t.Errorf("wait = %v, cap %v", d, maxRetryWait)
		}
	}
}

func TestRetryAfterInvalidFallsBackToBackoff(t *testing.T) {
	c, waits := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "soon")
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("a persistent 503 must end in an error")
	}
	if len(*waits) != maxAttempts-1 {
		t.Errorf("waits = %v, want %d retries", *waits, maxAttempts-1)
	}
	for _, d := range *waits {
		if d <= 0 {
			t.Errorf("zero wait: the retry would be immediate")
		}
	}
}

func TestClientErrorsAreNotRetried(t *testing.T) {
	calls := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid body"}`))
	})

	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("a 400 must surface")
	}
	// Retrying a request refused for its content can only get it refused
	// again, while eating into the rate limit.
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestNotFoundIsDetectable(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Scenario not found"}`))
	})

	_, err := c.GetScenario(context.Background(), "mon-unknown")
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound = false for %v: Read would not know to drop the resource from the state", err)
	}
	if !strings.Contains(err.Error(), "Scenario not found") {
		t.Errorf("message lost: %v", err)
	}
	if IsNotFound(nil) {
		t.Error("IsNotFound(nil) must be false")
	}
}

func TestUnauthorizedAndForbiddenCarryGuidance(t *testing.T) {
	for status, expect := range map[int]string{
		http.StatusUnauthorized: "PATHLY_API_TOKEN",
		http.StatusForbidden:    "scopes",
	} {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"refused"}`))
		})
		// Not Ping: it deliberately tolerates a 403, which only proves the key
		// does not carry `org:read`.
		_, err := c.GetScenario(context.Background(), "mon-1")
		if err == nil || !strings.Contains(err.Error(), expect) {
			t.Errorf("HTTP %d: message = %v, it must say what to fix", status, err)
		}
	}
}

func TestErrorWithoutJSONBodyStillReadable(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>gateway</html>"))
	})

	// An intermediary may answer HTML: the message must not be empty.
	err := c.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "gateway") {
		t.Errorf("error = %v", err)
	}
}

func TestEmptyErrorBodyUsesStatusText(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})

	err := c.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Conflict") {
		t.Errorf("error = %v", err)
	}
}

func TestListScenariosFollowsPagination(t *testing.T) {
	page := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			if got := r.URL.Query().Get("limit"); got != "200" {
				t.Errorf("limit = %q, pagination must ask for the maximum", got)
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"a"}],"nextCursor":"cursor-1"}`))
			return
		}
		if got := r.URL.Query().Get("cursor"); got != "cursor-1" {
			t.Errorf("cursor = %q", got)
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"b"}],"nextCursor":null}`))
	})

	got, err := c.ListScenarios(context.Background())
	if err != nil {
		t.Fatalf("ListScenarios: %v", err)
	}
	// Stopping at the first page would produce incomplete for_each loops.
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("scenarios = %+v", got)
	}
}

func TestMuteSendsNullToWake(t *testing.T) {
	var body map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	if err := c.MuteScenario(context.Background(), "mon-1", nil); err != nil {
		t.Fatalf("MuteScenario: %v", err)
	}
	// `mutedUntil` must be present and null: omitted, the API would refuse the body.
	value, present := body["mutedUntil"]
	if !present || value != nil {
		t.Errorf("body = %v", body)
	}
}

func TestDeleteNeedsNoResponseBody(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.DeleteScenario(context.Background(), "mon-1"); err != nil {
		t.Errorf("DeleteScenario: %v", err)
	}
}

func TestTransportErrorIsRetriedThenReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // the server no longer answers: a transport error is guaranteed

	var waits []time.Duration
	c := New(url, "sp_x", WithSleep(func(d time.Duration) { waits = append(waits, d) }))
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("an unreachable transport must surface an error")
	}
	if len(waits) != maxAttempts-1 {
		t.Errorf("retries = %d, want %d", len(waits), maxAttempts-1)
	}
}

func TestContextCancellationStopsImmediately(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"planId":"pro"}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := c.Ping(ctx); err == nil {
		t.Error("a cancelled context must interrupt the call")
	}
}

func TestUpsertSlaTargetUsesPut(t *testing.T) {
	var method string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		_, _ = w.Write([]byte(`{"id":"sla-1","objectivePct":99.9,"windowDays":30}`))
	})

	got, err := c.UpsertSlaTarget(context.Background(), SlaTargetInput{ObjectivePct: 99.9, WindowDays: 30}, "idem")
	if err != nil {
		t.Fatalf("UpsertSlaTarget: %v", err)
	}
	if method != http.MethodPut {
		t.Errorf("method = %s, want PUT", method)
	}
	if got.ObjectivePct == nil || *got.ObjectivePct != 99.9 {
		t.Errorf("target = %+v", got)
	}
}

func TestWebhookSecretOnlyOnCreate(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"h1","events":["run.failed"],"secret":"whsec_x","urlFingerprint":"fp1"}`))
			return
		}
		// A read never returns the secret nor the URL.
		_, _ = w.Write([]byte(`{"id":"h1","events":["run.failed"],"hasSecret":true,"urlFingerprint":"fp1"}`))
	})

	created, err := c.CreateWebhook(context.Background(), WebhookInput{URL: "https://hooks.example.com"}, "idem")
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	if created.Secret == nil || *created.Secret == "" {
		t.Fatal("the signing secret must be returned on create, it is only returned once")
	}

	read, err := c.GetWebhook(context.Background(), "h1")
	if err != nil {
		t.Fatalf("GetWebhook: %v", err)
	}
	if read.Secret != nil {
		t.Error("a read must not return the secret")
	}
	if read.URLFingerprint == nil {
		t.Error("the URL fingerprint is the only way to see a destination changed outside of Terraform")
	}
}

func TestMalformedJSONIsReported(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	})

	if err := c.Ping(context.Background()); err == nil {
		t.Error("an unreadable response must surface instead of leaving an empty state")
	}
}
