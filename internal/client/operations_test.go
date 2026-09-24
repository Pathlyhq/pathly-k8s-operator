package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

/*
Every operation of the client, once: expected HTTP method and path.

The detail of the behavior (idempotency, retries, errors) is covered in
client_test.go. What is checked here is dumber and just as useful: a misspelled
path or an inverted method only shows up at run time, against the real API, and
costs a debugging round trip.
*/
func TestEveryOperationTargetsTheRightRoute(t *testing.T) {
	cases := []struct {
		name   string
		call   func(c *Client) error
		method string
		path   string
	}{
		{
			name:   "create scenario",
			method: http.MethodPost,
			path:   "/v1/scenarios",
			call: func(c *Client) error {
				_, err := c.CreateScenario(context.Background(), ScenarioInput{}, "idem")
				return err
			},
		},
		{
			name:   "read scenario",
			method: http.MethodGet,
			path:   "/v1/scenarios/mon-1",
			call: func(c *Client) error {
				_, err := c.GetScenario(context.Background(), "mon-1")
				return err
			},
		},
		{
			name:   "update scenario",
			method: http.MethodPatch,
			path:   "/v1/scenarios/mon-1",
			call: func(c *Client) error {
				_, err := c.UpdateScenario(context.Background(), "mon-1", ScenarioInput{})
				return err
			},
		},
		{
			name:   "delete scenario",
			method: http.MethodDelete,
			path:   "/v1/scenarios/mon-1",
			call:   func(c *Client) error { return c.DeleteScenario(context.Background(), "mon-1") },
		},
		{
			name:   "mute",
			method: http.MethodPost,
			path:   "/v1/scenarios/mon-1/mute",
			call: func(c *Client) error {
				until := "2026-09-23T00:00:00.000Z"
				return c.MuteScenario(context.Background(), "mon-1", &until)
			},
		},
		{
			name:   "create maintenance window",
			method: http.MethodPost,
			path:   "/v1/maintenance-windows",
			call: func(c *Client) error {
				_, err := c.CreateMaintenanceWindow(context.Background(), MaintenanceWindowInput{}, "idem")
				return err
			},
		},
		{
			name:   "read maintenance window",
			method: http.MethodGet,
			path:   "/v1/maintenance-windows/mw-1",
			call: func(c *Client) error {
				_, err := c.GetMaintenanceWindow(context.Background(), "mw-1")
				return err
			},
		},
		{
			name:   "delete maintenance window",
			method: http.MethodDelete,
			path:   "/v1/maintenance-windows/mw-1",
			call:   func(c *Client) error { return c.DeleteMaintenanceWindow(context.Background(), "mw-1") },
		},
		{
			name:   "create webhook",
			method: http.MethodPost,
			path:   "/v1/webhooks",
			call: func(c *Client) error {
				_, err := c.CreateWebhook(context.Background(), WebhookInput{URL: "https://x.example.com"}, "idem")
				return err
			},
		},
		{
			name:   "read webhook",
			method: http.MethodGet,
			path:   "/v1/webhooks/h1",
			call: func(c *Client) error {
				_, err := c.GetWebhook(context.Background(), "h1")
				return err
			},
		},
		{
			name:   "delete webhook",
			method: http.MethodDelete,
			path:   "/v1/webhooks/h1",
			call:   func(c *Client) error { return c.DeleteWebhook(context.Background(), "h1") },
		},
		{
			name:   "SLA target",
			method: http.MethodPut,
			path:   "/v1/sla-targets",
			call: func(c *Client) error {
				_, err := c.UpsertSlaTarget(context.Background(), SlaTargetInput{ObjectivePct: 99, WindowDays: 30}, "idem")
				return err
			},
		},
		{
			name:   "read SLA target",
			method: http.MethodGet,
			path:   "/v1/sla-targets/sla-1",
			call: func(c *Client) error {
				_, err := c.GetSlaTarget(context.Background(), "sla-1")
				return err
			},
		},
		{
			name:   "delete SLA target",
			method: http.MethodDelete,
			path:   "/v1/sla-targets/sla-1",
			call:   func(c *Client) error { return c.DeleteSlaTarget(context.Background(), "sla-1") },
		},
		{
			name:   "usage",
			method: http.MethodGet,
			path:   "/v1/usage",
			call:   func(c *Client) error { return c.Ping(context.Background()) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotPath string
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				_, _ = w.Write([]byte(`{"id":"x"}`))
			})
			if err := tc.call(c); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if gotMethod != tc.method || gotPath != tc.path {
				t.Errorf("%s: %s %s, want %s %s", tc.name, gotMethod, gotPath, tc.method, tc.path)
			}
		})
	}
}

func TestIdentifiersAreEscapedInPaths(t *testing.T) {
	var gotPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusNotFound)
	})

	// An identifier coming from the state must not be able to escape its URL
	// segment and aim at another route.
	_, _ = c.GetScenario(context.Background(), "../maintenance-windows/mw-1")
	if gotPath == "/v1/maintenance-windows/mw-1" {
		t.Errorf("path = %q: the identifier must stay escaped", gotPath)
	}
}

func TestOptionsAreApplied(t *testing.T) {
	custom := &http.Client{}
	c := New("https://api.example.com", "sp_x", WithHTTPClient(custom), WithUserAgent("test-ua/9"))
	if c.httpClient != custom {
		t.Error("WithHTTPClient must be honored: a corporate proxy depends on it")
	}
	if c.userAgent != "test-ua/9" {
		t.Errorf("userAgent = %q", c.userAgent)
	}
}

func TestListScenariosStopsOnRunawayPagination(t *testing.T) {
	// A cursor that never advances loops forever: the guard must return an
	// error rather than burn the whole rate limit of the key.
	pages := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++
		_, _ = w.Write([]byte(`{"items":[{"id":"a"}],"nextCursor":"always-the-same"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "sp_x")
	got, err := c.ListScenarios(context.Background())
	if err == nil {
		t.Fatal("pagination must stop with an error, not run forever")
	}
	if got != nil {
		t.Error("a partial result would be taken for the complete fleet")
	}
	if pages > 250 {
		t.Errorf("pages requested = %d", pages)
	}
}
