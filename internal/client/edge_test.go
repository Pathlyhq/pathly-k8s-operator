package client

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

/*
Failure paths the test server cannot produce.

They are not theoretical: a connection cut in the middle of the response and a
request body that cannot be encoded do happen in production, and that is
exactly where a provider returning an empty error leaves the user with no lead.
*/

// brokenBody simulates a connection cut after the response header.
type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) {
	return 0, errors.New("connection reset while reading")
}
func (brokenBody) Close() error { return nil }

type brokenTransport struct{ calls *int }

func (t brokenTransport) RoundTrip(*http.Request) (*http.Response, error) {
	*t.calls++
	return &http.Response{StatusCode: http.StatusOK, Body: brokenBody{}, Header: http.Header{}}, nil
}

func TestUnreadableResponseIsRetriedThenReported(t *testing.T) {
	calls := 0
	var waits []time.Duration
	c := New("https://api.example.com", "sp_x",
		WithHTTPClient(&http.Client{Transport: brokenTransport{calls: &calls}}),
		WithSleep(func(d time.Duration) { waits = append(waits, d) }),
	)

	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("an unreadable response must surface, not leave an empty state")
	}
	if !strings.Contains(err.Error(), "reading the response") {
		t.Errorf("error = %v, it must say where the read failed", err)
	}
	// A cut is transient: it is retried, like a 5xx.
	if calls != maxAttempts || len(waits) != maxAttempts-1 {
		t.Errorf("calls = %d, waits = %v", calls, waits)
	}
}

func TestUnencodableBodyFailsBeforeAnyCall(t *testing.T) {
	calls := 0
	c := New("https://api.example.com", "sp_x",
		WithHTTPClient(&http.Client{Transport: brokenTransport{calls: &calls}}),
		WithSleep(func(time.Duration) {}),
	)

	// A channel cannot be encoded to JSON: the case can only come from a bug
	// in the provider, and must fail before reaching the network.
	err := c.do(context.Background(), request{
		method: http.MethodPost,
		path:   "/v1/scenarios",
		body:   make(chan int),
	})
	if err == nil || !strings.Contains(err.Error(), "encoding the body") {
		t.Fatalf("error = %v", err)
	}
	if calls != 0 {
		t.Errorf("calls = %d, none must go out", calls)
	}
}

func TestInvalidMethodIsReportedWithoutRetry(t *testing.T) {
	calls := 0
	var waits []time.Duration
	c := New("https://api.example.com", "sp_x",
		WithHTTPClient(&http.Client{Transport: brokenTransport{calls: &calls}}),
		WithSleep(func(d time.Duration) { waits = append(waits, d) }),
	)

	err := c.do(context.Background(), request{method: "GET SCENARIOS", path: "/v1/scenarios"})
	if err == nil || !strings.Contains(err.Error(), "building request") {
		t.Fatalf("error = %v", err)
	}
	// A request that cannot even be built will not get better by being
	// replayed: the retry would be noise in the logs.
	if calls != 0 || len(waits) != 0 {
		t.Errorf("calls = %d, waits = %v", calls, waits)
	}
}

// A key limited to scenarios does not carry `org:read`: configuring the
// provider must accept it, otherwise least privilege becomes impossible.
func TestPingAcceptsAKeyWithoutOrgRead(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"This key does not carry the org:read scope."}`))
	})

	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("a 403 proves the key is valid: %v", err)
	}
}

func TestPingRejectsAnInvalidKey(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Invalid, revoked or expired API key."}`))
	})

	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("a refused key must fail at configuration time, not on every resource")
	}
	if !strings.Contains(err.Error(), "PATHLY_API_TOKEN") {
		t.Errorf("the message must say what to check: %v", err)
	}
}

/*
A validation refusal must name the offending field.

Without the detail, the apply used to show "/v1/webhooks: Invalid body (HTTP
400)": the user knew something was wrong, without knowing what, on a resource
that carries eight attributes.
*/
func TestValidationDetailsReachTheDiagnostic(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid body","details":[
		  {"path":["events",0],"message":"Invalid enum value"},
		  {"path":[],"message":"url is required"}
		]}`))
	})

	_, err := c.CreateWebhook(context.Background(), WebhookInput{URL: "https://h.example.com"}, "idem")
	if err == nil {
		t.Fatal("a refused body must surface")
	}
	for _, want := range []string{
		"Invalid body",
		// The index must show up: "events" alone does not say which of the two
		// events that were set is refused.
		"events.0",
		"Invalid enum value",
		"body",
		"url is required",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message must contain %q: %v", want, err)
		}
	}
}

// A missing or malformed detail must not hide the main message.
func TestErrorsWithoutDetailsStayReadable(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Plan limit reached"}`))
	})

	_, err := c.CreateScenario(context.Background(), ScenarioInput{}, "idem")
	if err == nil || !strings.Contains(err.Error(), "Plan limit reached") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "[") {
		t.Errorf("no detail to show, so no bracket: %v", err)
	}
}

func TestListScenariosReportsRefusal(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"This key does not carry the scenarios:read scope."}`))
	})

	got, err := c.ListScenarios(context.Background())
	if err == nil {
		t.Fatal("a refusal must surface: an empty list would destroy a whole fleet")
	}
	if got != nil {
		t.Errorf("scenarios = %+v, the partial list must not be returned", got)
	}
}

// Every write and every read must return the API error as it is. A single one
// of these paths swallowing the error is enough to store a nonexistent
// resource in the state, which Terraform will never recreate.
func TestEveryOperationPropagatesAPIErrors(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"missing scope"}`))
	})

	name := "Checkout"
	cases := map[string]func() error{
		"CreateScenario": func() error {
			_, err := c.CreateScenario(ctx, ScenarioInput{Name: &name}, "idem")
			return err
		},
		"UpdateScenario": func() error {
			_, err := c.UpdateScenario(ctx, "mon-1", ScenarioInput{Name: &name})
			return err
		},
		"GetScenario": func() error {
			_, err := c.GetScenario(ctx, "mon-1")
			return err
		},
		"CreateMaintenanceWindow": func() error {
			_, err := c.CreateMaintenanceWindow(ctx, MaintenanceWindowInput{}, "idem")
			return err
		},
		"GetMaintenanceWindow": func() error {
			_, err := c.GetMaintenanceWindow(ctx, "mw-1")
			return err
		},
		"CreateWebhook": func() error {
			_, err := c.CreateWebhook(ctx, WebhookInput{URL: "https://hooks.example.com"}, "idem")
			return err
		},
		"GetWebhook": func() error {
			_, err := c.GetWebhook(ctx, "h1")
			return err
		},
		"UpsertSlaTarget": func() error {
			_, err := c.UpsertSlaTarget(ctx, SlaTargetInput{ObjectivePct: 99, WindowDays: 30}, "idem")
			return err
		},
		"GetSlaTarget": func() error {
			_, err := c.GetSlaTarget(ctx, "sla-1")
			return err
		},
		"DeleteScenario":          func() error { return c.DeleteScenario(ctx, "mon-1") },
		"DeleteMaintenanceWindow": func() error { return c.DeleteMaintenanceWindow(ctx, "mw-1") },
		"DeleteWebhook":           func() error { return c.DeleteWebhook(ctx, "h1") },
		"DeleteSlaTarget":         func() error { return c.DeleteSlaTarget(ctx, "sla-1") },
		"MuteScenario":            func() error { return c.MuteScenario(ctx, "mon-1", nil) },
	}

	for name, call := range cases {
		err := call()
		if err == nil {
			t.Errorf("%s: the API refusal must surface", name)
			continue
		}
		if !strings.Contains(err.Error(), "missing scope") {
			t.Errorf("%s: message lost (%v)", name, err)
		}
	}
}
