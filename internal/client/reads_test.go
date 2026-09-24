package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

/*
The read side of the API.

Two things are checked here, and they fail for different reasons. First the
routes and the envelopes: the API wraps its answers in three different ways —
`{settings}`, `{items}` and a bare object — and reading one with the wrong
envelope gives an empty result rather than an error, which is the worst possible
outcome for a data source. Then the numbers: several fields come out of decimal
columns and arrive quoted, so a naive decoder breaks a resource over a
formatting detail.
*/

// ---------------------------------------------------------------------------
// Numbers arriving as strings
// ---------------------------------------------------------------------------

func TestNumberAcceptsBothFormsTheAPIUses(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want float64
	}{
		{name: "bare number", raw: `1.5`, want: 1.5},
		{name: "quoted number", raw: `"1.5"`, want: 1.5},
		{name: "integer", raw: `99`, want: 99},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got Number
			if err := json.Unmarshal([]byte(tc.raw), &got); err != nil {
				t.Fatalf("%s: %v", tc.raw, err)
			}
			if float64(got) != tc.want {
				t.Errorf("%s decoded to %v, want %v", tc.raw, float64(got), tc.want)
			}
		})
	}
}

func TestNumberTreatsEmptyAsAbsentAndRefusesGarbage(t *testing.T) {
	// `null` and `""` are how the API says "not measured yet". Turning them into
	// zero would read as "no downtime" when nothing has been measured.
	for _, raw := range []string{`null`, `""`} {
		var got Number
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("%s must decode without error: %v", raw, err)
		}
		if float64(got) != 0 {
			t.Errorf("%s decoded to %v", raw, float64(got))
		}
	}

	var got Number
	if err := json.Unmarshal([]byte(`"not-a-number"`), &got); err == nil {
		t.Error("an unreadable value must be reported, not silently zeroed")
	}
}

func TestNumberFloatDistinguishesAbsentFromZero(t *testing.T) {
	var absent *Number
	if absent.Float() != nil {
		t.Error("an absent number must stay absent")
	}
	value := Number(0)
	if got := value.Float(); got == nil || *got != 0 {
		t.Errorf("a zero that was measured must stay a zero: %v", got)
	}
}

// ---------------------------------------------------------------------------
// Routes and envelopes
// ---------------------------------------------------------------------------

// TestEveryReadTargetsTheRightRoute pins method and path for the read side.
//
// The same reasoning as for the write operations: a misspelled path only shows
// up against the real API, and costs a debugging round trip.
func TestEveryReadTargetsTheRightRoute(t *testing.T) {
	cases := []struct {
		name   string
		call   func(c *Client) error
		method string
		path   string
	}{
		{
			name: "read settings", method: http.MethodGet, path: "/v1/settings",
			call: func(c *Client) error { _, err := c.GetSettings(context.Background()); return err },
		},
		{
			name: "update settings", method: http.MethodPatch, path: "/v1/settings",
			call: func(c *Client) error {
				_, err := c.UpdateSettings(context.Background(), SettingsInput{}, "idem")
				return err
			},
		},
		{
			name: "usage", method: http.MethodGet, path: "/v1/usage",
			call: func(c *Client) error { _, err := c.GetUsage(context.Background()); return err },
		},
		{
			name: "incidents", method: http.MethodGet, path: "/v1/incidents",
			call: func(c *Client) error { _, err := c.ListIncidents(context.Background()); return err },
		},
		{
			name: "members", method: http.MethodGet, path: "/v1/members",
			call: func(c *Client) error { _, err := c.ListMembers(context.Background()); return err },
		},
		{
			name: "runs", method: http.MethodGet, path: "/v1/runs",
			call: func(c *Client) error { _, err := c.ListRuns(context.Background(), ""); return err },
		},
		{
			name: "single run", method: http.MethodGet, path: "/v1/runs/run-1",
			call: func(c *Client) error { _, err := c.GetRun(context.Background(), "run-1"); return err },
		},
		{
			name: "sla measurement", method: http.MethodGet, path: "/v1/sla",
			call: func(c *Client) error { _, err := c.ListSlaOverview(context.Background()); return err },
		},
		{
			name: "sla targets", method: http.MethodGet, path: "/v1/sla-targets",
			call: func(c *Client) error { _, err := c.ListSlaTargets(context.Background()); return err },
		},
		{
			name: "webhooks", method: http.MethodGet, path: "/v1/webhooks",
			call: func(c *Client) error { _, err := c.ListWebhooks(context.Background()); return err },
		},
		{
			name: "maintenance windows", method: http.MethodGet, path: "/v1/maintenance-windows",
			call: func(c *Client) error { _, err := c.ListMaintenanceWindows(context.Background()); return err },
		},
		{
			name: "status page", method: http.MethodGet, path: "/v1/status/components",
			call: func(c *Client) error { _, err := c.GetStatusPage(context.Background()); return err },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotPath string
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				_, _ = w.Write([]byte(`{"settings":{},"items":[],"components":[]}`))
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

// TestEveryReadPropagatesAPIErrors makes sure no read swallows a refusal.
//
// A data source that returned an empty list on a missing scope would make a
// `for_each` shrink to nothing, and Terraform would then destroy everything it
// no longer sees.
func TestEveryReadPropagatesAPIErrors(t *testing.T) {
	calls := map[string]func(c *Client) error{
		"settings": func(c *Client) error { _, err := c.GetSettings(context.Background()); return err },
		"update settings": func(c *Client) error {
			_, err := c.UpdateSettings(context.Background(), SettingsInput{}, "")
			return err
		},
		"usage":               func(c *Client) error { _, err := c.GetUsage(context.Background()); return err },
		"incidents":           func(c *Client) error { _, err := c.ListIncidents(context.Background()); return err },
		"members":             func(c *Client) error { _, err := c.ListMembers(context.Background()); return err },
		"runs":                func(c *Client) error { _, err := c.ListRuns(context.Background(), "mon-1"); return err },
		"single run":          func(c *Client) error { _, err := c.GetRun(context.Background(), "run-1"); return err },
		"sla":                 func(c *Client) error { _, err := c.ListSlaOverview(context.Background()); return err },
		"sla targets":         func(c *Client) error { _, err := c.ListSlaTargets(context.Background()); return err },
		"webhooks":            func(c *Client) error { _, err := c.ListWebhooks(context.Background()); return err },
		"maintenance windows": func(c *Client) error { _, err := c.ListMaintenanceWindows(context.Background()); return err },
		"status page":         func(c *Client) error { _, err := c.GetStatusPage(context.Background()); return err },
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"This key does not carry the required scope."}`))
			})
			if err := call(c); err == nil {
				t.Errorf("%s: a refusal must surface", name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Decoding what the API actually answers
// ---------------------------------------------------------------------------

func TestSettingsUnwrapTheEnvelopeAndToleratePostgresNumbers(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		// triageLatencyFactor quoted: that is how a NUMERIC column comes back.
		_, _ = w.Write([]byte(`{"settings":{
		  "name":"Acme","planId":"pro","alertEmail":"ops@example.com","alertOnRecovery":true,
		  "escalationAfterFails":3,"statusSlug":"acme","statusPublic":false,
		  "timezone":"Europe/Paris","sslWarnDays":[30,14,7],"domainWarnDays":[],
		  "triageEnabled":true,"triageLatencyFactor":"2.5","hasSlackWebhook":true
		}}`))
	})

	got, err := c.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if got.Name == nil || *got.Name != "Acme" {
		t.Errorf("name = %v", got.Name)
	}
	if got.TriageLatencyFactor == nil || float64(*got.TriageLatencyFactor) != 2.5 {
		t.Errorf("triage latency factor = %v", got.TriageLatencyFactor)
	}
	if len(got.SSLWarnDays) != 3 || got.SSLWarnDays[0] != 30 {
		t.Errorf("ssl warn days = %v", got.SSLWarnDays)
	}
	// `[]` and absence are different: the empty list means "no threshold set",
	// and turning it into nil would make a difference show up on every plan.
	if got.DomainWarnDays == nil || len(got.DomainWarnDays) != 0 {
		t.Errorf("domain warn days = %v, want an empty non-nil slice", got.DomainWarnDays)
	}
	if got.HasSlackWebhook == nil || !*got.HasSlackWebhook {
		t.Errorf("has slack webhook = %v", got.HasSlackWebhook)
	}
}

func TestUpdateSettingsSendsOnlyWhatWasAsked(t *testing.T) {
	var body map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"settings":{"timezone":"Europe/Paris"}}`))
	})

	timezone := "Europe/Paris"
	got, err := c.UpdateSettings(context.Background(), SettingsInput{Timezone: &timezone}, "idem")
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if got.Timezone == nil || *got.Timezone != timezone {
		t.Errorf("timezone read back = %v", got.Timezone)
	}
	// A field the configuration does not mention must not travel: sending a
	// zero would erase a setting made from the console.
	if len(body) != 1 {
		t.Errorf("body = %v, only the timezone was asked for", body)
	}
	if _, present := body["alertEmail"]; present {
		t.Error("alertEmail must stay out of the request")
	}
}

func TestUsageReadsQuotedCounters(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"planId":"pro","browserRunsUsed":"12.5","packRunsUsedThisPeriod":40,"packRunsRemaining":960}`))
	})
	got, err := c.GetUsage(context.Background())
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if got.PlanID != "pro" {
		t.Errorf("plan = %q", got.PlanID)
	}
	if got.BrowserRunsUsed == nil || float64(*got.BrowserRunsUsed) != 12.5 {
		t.Errorf("browser runs used = %v", got.BrowserRunsUsed)
	}
	if got.PackRunsRemaining == nil || float64(*got.PackRunsRemaining) != 960 {
		t.Errorf("pack runs remaining = %v", got.PackRunsRemaining)
	}
}

// Incidents and runs are paginated. The walk has to follow the cursor, because
// a data source stopping at the first page produces silently incomplete loops.
func TestIncidentsFollowThePages(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			if r.URL.Query().Get("cursor") != "" {
				t.Error("the first page must be asked for without a cursor")
			}
			_, _ = w.Write([]byte(`{"items":[
			  {"id":"inc-1","monitor_id":"mon-1","monitor_name":"Checkout","status":"open",
			   "title":"Checkout down","opened_at":"2026-09-23T08:00:00.000Z","public_postmortem":false,
			   "ado_work_item_id":4242}
			],"nextCursor":"c1"}`))
			return
		}
		if got := r.URL.Query().Get("cursor"); got != "c1" {
			t.Errorf("cursor of the second page = %q", got)
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"inc-2","status":"resolved"}],"nextCursor":null}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "sp_x").ListIncidents(context.Background())
	if err != nil {
		t.Fatalf("ListIncidents: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("incidents = %+v", got)
	}
	if got[0].WorkItemID == nil || *got[0].WorkItemID != 4242 {
		t.Errorf("work item = %v", got[0].WorkItemID)
	}
	if got[0].MonitorName == nil || *got[0].MonitorName != "Checkout" {
		t.Errorf("scenario name = %v", got[0].MonitorName)
	}
}

func TestRunsFilterIsPassedToTheAPI(t *testing.T) {
	var query string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"items":[
		  {"id":"run-1","monitor_id":"mon-1","monitor_name":"Checkout","monitor_type":"browser",
		   "status":"fail","latency_ms":1800,"message":"assert_text failed","probe_region":"eu-west",
		   "triage_verdict":"real","in_maintenance":false,"browser_seconds":"3.250",
		   "created_at":"2026-09-23T08:00:00.000Z"}
		],"nextCursor":null}`))
	})

	got, err := c.ListRuns(context.Background(), "mon-1")
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	// Filtering server-side is what keeps the number of pages down on a large
	// organization.
	if !contains(query, "scenarioId=mon-1") {
		t.Errorf("query = %q, the scenario filter must reach the API", query)
	}
	if len(got) != 1 || got[0].BrowserSeconds == nil || float64(*got[0].BrowserSeconds) != 3.25 {
		t.Errorf("run = %+v", got)
	}
}

func TestRunsWithoutFilterAskForTheWholeOrganization(t *testing.T) {
	var query string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"items":[],"nextCursor":null}`))
	})
	// A blank filter must not become `scenarioId=`: the API would read it as a
	// scenario whose identifier is empty and answer nothing.
	if _, err := c.ListRuns(context.Background(), "   "); err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if contains(query, "scenarioId") {
		t.Errorf("query = %q, no filter was asked for", query)
	}
}

func TestSingleRunIsReadWhole(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"run-1","monitor_id":"mon-1","monitor_name":"Checkout",
		  "status":"ok","latency_ms":320,"probe_region":"eu-west","in_maintenance":true,
		  "created_at":"2026-09-23T08:00:00.000Z"}`))
	})
	got, err := c.GetRun(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.LatencyMs == nil || *got.LatencyMs != 320 {
		t.Errorf("latency = %v", got.LatencyMs)
	}
	if got.InMaintenance == nil || !*got.InMaintenance {
		t.Errorf("in maintenance = %v", got.InMaintenance)
	}
	// The single read returns a narrower projection: the absent fields must come
	// back empty rather than zeroed.
	if got.MonitorType != nil {
		t.Errorf("scenario type = %v, absent from this projection", got.MonitorType)
	}
}

func TestMembersAndInventoriesUnwrapItems(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/members":
			_, _ = w.Write([]byte(`{"items":[{"id":"u-1","email":"ops@example.com","name":"Ops","role":"admin"}]}`))
		case "/v1/webhooks":
			_, _ = w.Write([]byte(`{"items":[{"id":"h-1","events":["run.failed"],"enabled":true,"hasSecret":true,"urlFingerprint":"sha256:ab"}]}`))
		case "/v1/maintenance-windows":
			_, _ = w.Write([]byte(`{"items":[{"id":"mw-1","weekday":6,"startMinute":120,"durationMin":90}]}`))
		default:
			_, _ = w.Write([]byte(`{"items":[{"id":"sla-1","objectivePct":99.9,"windowDays":30,"enabled":true}]}`))
		}
	})
	ctx := context.Background()

	members, err := c.ListMembers(ctx)
	if err != nil || len(members) != 1 || members[0].Role == nil || *members[0].Role != "admin" {
		t.Fatalf("members = %+v, err = %v", members, err)
	}
	hooks, err := c.ListWebhooks(ctx)
	if err != nil || len(hooks) != 1 || hooks[0].URLFingerprint == nil {
		t.Fatalf("webhooks = %+v, err = %v", hooks, err)
	}
	// The listing must not leak the destination, only its fingerprint.
	if hooks[0].Secret != nil {
		t.Error("the listing must not return a secret")
	}
	windows, err := c.ListMaintenanceWindows(ctx)
	if err != nil || len(windows) != 1 || windows[0].DurationMin == nil || *windows[0].DurationMin != 90 {
		t.Fatalf("windows = %+v, err = %v", windows, err)
	}
	targets, err := c.ListSlaTargets(ctx)
	if err != nil || len(targets) != 1 || targets[0].ObjectivePct == nil || *targets[0].ObjectivePct != 99.9 {
		t.Fatalf("targets = %+v, err = %v", targets, err)
	}
}

func TestSlaOverviewCarriesTheMeasurement(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"items":[
		  {"id":"sla-1","monitorId":"mon-1","monitorName":"Checkout","name":"Checkout 99.9",
		   "objectivePct":99.9,"windowDays":30,"excludeMaintenance":true,"warnAtBudgetRatio":0.8,
		   "enabled":true,
		   "status":{"uptimePct":"99.95","objectivePct":99.9,"windowDays":30,"eligibleRuns":8640,
		             "okRuns":8636,"failedRuns":4,"excludedRuns":12,"errorBudgetRuns":9,
		             "errorBudgetUsedRatio":"0.444","estimatedDowntimeMinutes":20,"state":"ok"}},
		  {"id":"sla-2","name":"Fresh target","status":null}
		]}`))
	})
	got, err := c.ListSlaOverview(context.Background())
	if err != nil {
		t.Fatalf("ListSlaOverview: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("targets = %+v", got)
	}
	if got[0].Status == nil || got[0].Status.State == nil || *got[0].Status.State != "ok" {
		t.Fatalf("measurement = %+v", got[0].Status)
	}
	if u := got[0].Status.UptimePct; u == nil || float64(*u) != 99.95 {
		t.Errorf("uptime = %v", u)
	}
	// A target never measured has no status: the absence has to survive the
	// decoding, otherwise it reads as a perfect score.
	if got[1].Status != nil {
		t.Errorf("unmeasured target = %+v", got[1].Status)
	}
}

func TestStatusPageReadsComponentsAndReportsAbsence(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"components":[
		  {"name":"Checkout","uptime30d":99.98,"uptime90d":"99.9"},
		  {"name":"API","uptime30d":100,"uptime90d":100}
		],"uptime30d":99.99}`))
	})
	got, err := c.GetStatusPage(context.Background())
	if err != nil {
		t.Fatalf("GetStatusPage: %v", err)
	}
	if len(got.Components) != 2 || got.Components[1].Name != "API" {
		t.Fatalf("components = %+v", got.Components)
	}
	if got.Uptime30d == nil || float64(*got.Uptime30d) != 99.99 {
		t.Errorf("overall uptime = %v", got.Uptime30d)
	}

	// No page, or a private one: the API answers 404 and that must stay
	// distinguishable from an empty page.
	missing, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Pas de status page"}`))
	})
	_, err = missing.GetStatusPage(context.Background())
	if !IsNotFound(err) {
		t.Errorf("a missing status page must be a not-found, got %v", err)
	}
}

func TestPaginatedReadStopsOnRunawayCursor(t *testing.T) {
	// Same guard as for the scenarios, checked on another endpoint: the bound
	// lives in the shared walk, so it has to protect every listing that uses it.
	pages := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++
		_, _ = w.Write([]byte(`{"items":[{"id":"run-1"}],"nextCursor":"stuck"}`))
	}))
	defer srv.Close()

	got, err := New(srv.URL, "sp_x").ListRuns(context.Background(), "")
	if err == nil {
		t.Fatal("a cursor that never advances must stop with an error")
	}
	if got != nil {
		t.Error("a partial result would be taken for the whole history")
	}
	if pages > maxPages+1 {
		t.Errorf("pages requested = %d", pages)
	}
}

// contains avoids pulling strings in for a single check, and says what the test
// means: the query has to carry the filter.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
