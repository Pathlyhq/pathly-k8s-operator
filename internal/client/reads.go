package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Numbers that arrive as strings
// ---------------------------------------------------------------------------

// Number decodes a JSON number that the API sometimes sends quoted.
//
// The API reads several of these values straight out of PostgreSQL `NUMERIC`
// columns, and the driver renders that type as a string to keep the exact
// decimal. A plain float64 field would make the whole response unreadable and
// break a resource over a formatting detail, so the two forms are accepted.
type Number float64

func (n *Number) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("unreadable number %s: %w", raw, err)
	}
	*n = Number(value)
	return nil
}

// Float returns the value, or nil when the API left the field empty.
func (n *Number) Float() *float64 {
	if n == nil {
		return nil
	}
	f := float64(*n)
	return &f
}

// ---------------------------------------------------------------------------
// Listing helpers
// ---------------------------------------------------------------------------

// maxPages bounds a walk through a paginated listing. A cursor that never
// advances would otherwise loop forever, and an `apply` that hangs is harder to
// diagnose than one that stops.
const maxPages = 200

// pageSize asks for the largest page the API accepts, to keep the number of
// round trips low when a plan reads a whole estate back.
const pageSize = "200"

// listPaged walks every page of a `{items, nextCursor}` listing.
//
// A data source that returned only the first page would produce silently
// incomplete `for_each` loops.
func listPaged[T any](ctx context.Context, c *Client, base string, extra url.Values) ([]T, error) {
	var all []T
	cursor := ""
	for page := 0; page < maxPages; page++ {
		var out struct {
			Items      []T     `json:"items"`
			NextCursor *string `json:"nextCursor"`
		}
		query := url.Values{"limit": []string{pageSize}}
		for key, values := range extra {
			query[key] = values
		}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		path := base + "?" + query.Encode()
		if err := c.do(ctx, request{method: http.MethodGet, path: path, out: &out}); err != nil {
			return nil, err
		}
		all = append(all, out.Items...)
		if out.NextCursor == nil || *out.NextCursor == "" {
			return all, nil
		}
		cursor = *out.NextCursor
	}
	return nil, fmt.Errorf("%s pagination: too many pages, stopping on purpose", base)
}

// listItems reads a listing the API returns whole, with no cursor.
func listItems[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var out struct {
		Items []T `json:"items"`
	}
	if err := c.do(ctx, request{method: http.MethodGet, path: path, out: &out}); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// ---------------------------------------------------------------------------
// Organization settings
// ---------------------------------------------------------------------------

// Settings is the organization-wide configuration.
//
// The `Has*` fields report that an integration is wired without ever returning
// its URL or its key: the API only exposes their presence, which is what lets
// a Terraform state describe the alerting setup without holding a secret.
type Settings struct {
	Name                 *string `json:"name"`
	PlanID               *string `json:"planId"`
	AlertEmail           *string `json:"alertEmail"`
	AlertOnRecovery      *bool   `json:"alertOnRecovery"`
	EscalationAfterFails *int64  `json:"escalationAfterFails"`
	EscalationEmail      *string `json:"escalationEmail"`
	WeeklyDigestEmail    *string `json:"weeklyDigestEmail"`
	WeeklyDigestEnabled  *bool   `json:"weeklyDigestEnabled"`
	StatusSlug           *string `json:"statusSlug"`
	StatusPublic         *bool   `json:"statusPublic"`
	Timezone             *string `json:"timezone"`
	SSLWarnDays          []int64 `json:"sslWarnDays"`
	DomainWarnDays       []int64 `json:"domainWarnDays"`
	TriageEnabled        *bool   `json:"triageEnabled"`
	TriageConfirmEnabled *bool   `json:"triageConfirmEnabled"`
	TriageLatencyFactor  *Number `json:"triageLatencyFactor"`
	HasSlackWebhook      *bool   `json:"hasSlackWebhook"`
	HasTeamsWebhook      *bool   `json:"hasTeamsWebhook"`
	HasDiscordWebhook    *bool   `json:"hasDiscordWebhook"`
	HasPagerduty         *bool   `json:"hasPagerduty"`
	HasOpsgenie          *bool   `json:"hasOpsgenie"`
	HasDatadog           *bool   `json:"hasDatadog"`
	HasSentry            *bool   `json:"hasSentry"`
}

// SettingsInput only carries what the API accepts in writing. The plan, the
// organization name and the integration flags are read-only, so a configuration
// cannot claim to set them.
type SettingsInput struct {
	AlertEmail           *string  `json:"alertEmail,omitempty"`
	AlertOnRecovery      *bool    `json:"alertOnRecovery,omitempty"`
	EscalationAfterFails *int64   `json:"escalationAfterFails,omitempty"`
	EscalationEmail      *string  `json:"escalationEmail,omitempty"`
	WeeklyDigestEmail    *string  `json:"weeklyDigestEmail,omitempty"`
	WeeklyDigestEnabled  *bool    `json:"weeklyDigestEnabled,omitempty"`
	StatusSlug           *string  `json:"statusSlug,omitempty"`
	StatusPublic         *bool    `json:"statusPublic,omitempty"`
	Timezone             *string  `json:"timezone,omitempty"`
	SSLWarnDays          []int64  `json:"sslWarnDays,omitempty"`
	DomainWarnDays       []int64  `json:"domainWarnDays,omitempty"`
	TriageEnabled        *bool    `json:"triageEnabled,omitempty"`
	TriageConfirmEnabled *bool    `json:"triageConfirmEnabled,omitempty"`
	TriageLatencyFactor  *float64 `json:"triageLatencyFactor,omitempty"`
}

// settingsEnvelope is how the API wraps both the read and the write.
type settingsEnvelope struct {
	Settings Settings `json:"settings"`
}

func (c *Client) GetSettings(ctx context.Context) (*Settings, error) {
	out := &settingsEnvelope{}
	if err := c.do(ctx, request{method: http.MethodGet, path: "/v1/settings", out: out}); err != nil {
		return nil, err
	}
	return &out.Settings, nil
}

// UpdateSettings sends only the fields present in the input. The settings are a
// single organization-wide object: there is nothing to create and nothing to
// delete, only fields to move.
func (c *Client) UpdateSettings(ctx context.Context, in SettingsInput, idempotencyKey string) (*Settings, error) {
	out := &settingsEnvelope{}
	err := c.do(ctx, request{
		method:         http.MethodPatch,
		path:           "/v1/settings",
		body:           in,
		idempotencyKey: idempotencyKey,
		out:            out,
	})
	if err != nil {
		return nil, err
	}
	return &out.Settings, nil
}

// ---------------------------------------------------------------------------
// Consumption
// ---------------------------------------------------------------------------

// Usage is the consumption of the running period. Every count goes through
// Number: the API derives them from decimal columns, so a quoted value is a
// normal answer rather than an anomaly.
type Usage struct {
	PlanID                 string  `json:"planId"`
	BrowserRunsUsed        *Number `json:"browserRunsUsed"`
	PackRunsUsedThisPeriod *Number `json:"packRunsUsedThisPeriod"`
	PackRunsRemaining      *Number `json:"packRunsRemaining"`
}

func (c *Client) GetUsage(ctx context.Context) (*Usage, error) {
	out := &Usage{}
	if err := c.do(ctx, request{method: http.MethodGet, path: "/v1/usage", out: out}); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Incidents
// ---------------------------------------------------------------------------

// Incident is read-only here on purpose: an incident is opened by a failing
// run, not by a configuration. The provider reports it, resolving it and
// writing a postmortem stay actions taken in the console or over the API.
//
// The field names are the ones the API returns, which are snake_case for this
// endpoint. They are kept as they are rather than renamed: a projection that
// invents its own names hides which endpoint the value came from.
type Incident struct {
	ID               string  `json:"id"`
	MonitorID        *string `json:"monitor_id"`
	MonitorName      *string `json:"monitor_name"`
	Status           *string `json:"status"`
	Title            *string `json:"title"`
	OpenedAt         *string `json:"opened_at"`
	ResolvedAt       *string `json:"resolved_at"`
	Postmortem       *string `json:"postmortem"`
	PublicPostmortem *bool   `json:"public_postmortem"`
	OpenedRunID      *string `json:"opened_run_id"`
	ResolvedRunID    *string `json:"resolved_run_id"`
	AssigneeEmail    *string `json:"assignee_email"`
	AssigneeName     *string `json:"assignee_name"`
	WorkItemID       *int64  `json:"ado_work_item_id"`
	WorkItemURL      *string `json:"ado_work_item_url"`
}

func (c *Client) ListIncidents(ctx context.Context) ([]Incident, error) {
	return listPaged[Incident](ctx, c, "/v1/incidents", nil)
}

// ---------------------------------------------------------------------------
// Members
// ---------------------------------------------------------------------------

// Member is deliberately read-only. Inviting a member from a Terraform file
// would turn a repository write into an access grant, which is an escalation
// through a detour.
type Member struct {
	ID    string  `json:"id"`
	Email *string `json:"email"`
	Name  *string `json:"name"`
	Role  *string `json:"role"`
}

func (c *Client) ListMembers(ctx context.Context) ([]Member, error) {
	return listItems[Member](ctx, c, "/v1/members")
}

// ---------------------------------------------------------------------------
// Runs
// ---------------------------------------------------------------------------

// Run is one execution of a scenario.
//
// The `stepResults` and `triageSignals` payloads are left out: they are
// free-form JSON whose shape follows the scenario, and a Terraform attribute
// cannot describe that without becoming a string nobody can plan against.
type Run struct {
	ID              string  `json:"id"`
	MonitorID       *string `json:"monitor_id"`
	MonitorName     *string `json:"monitor_name"`
	MonitorType     *string `json:"monitor_type"`
	Status          *string `json:"status"`
	LatencyMs       *int64  `json:"latency_ms"`
	Message         *string `json:"message"`
	ProbeRegion     *string `json:"probe_region"`
	ComparedToRunID *string `json:"compared_to_run_id"`
	ConfirmsRunID   *string `json:"confirms_run_id"`
	TriageVerdict   *string `json:"triage_verdict"`
	TriageReason    *string `json:"triage_reason"`
	InMaintenance   *bool   `json:"in_maintenance"`
	BrowserSeconds  *Number `json:"browser_seconds"`
	ScreenshotPath  *string `json:"screenshot_path"`
	VideoPath       *string `json:"video_path"`
	CreatedAt       *string `json:"created_at"`
}

// ListRuns walks the executions, newest first. An empty scenarioID reads the
// whole organization.
func (c *Client) ListRuns(ctx context.Context, scenarioID string) ([]Run, error) {
	var extra url.Values
	if strings.TrimSpace(scenarioID) != "" {
		extra = url.Values{"scenarioId": []string{scenarioID}}
	}
	return listPaged[Run](ctx, c, "/v1/runs", extra)
}

// GetRun reads a single execution. The API answers a narrower projection here
// than in the listing, so the absent fields come back empty.
func (c *Client) GetRun(ctx context.Context, id string) (*Run, error) {
	out := &Run{}
	err := c.do(ctx, request{
		method: http.MethodGet,
		path:   "/v1/runs/" + url.PathEscape(id),
		out:    out,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// SLA measurement
// ---------------------------------------------------------------------------

// SlaStatus is the measurement of a target over its window.
type SlaStatus struct {
	UptimePct                *Number `json:"uptimePct"`
	ObjectivePct             *Number `json:"objectivePct"`
	WindowDays               *int64  `json:"windowDays"`
	EligibleRuns             *int64  `json:"eligibleRuns"`
	OkRuns                   *int64  `json:"okRuns"`
	FailedRuns               *int64  `json:"failedRuns"`
	ExcludedRuns             *int64  `json:"excludedRuns"`
	ErrorBudgetRuns          *int64  `json:"errorBudgetRuns"`
	ErrorBudgetUsedRatio     *Number `json:"errorBudgetUsedRatio"`
	EstimatedDowntimeMinutes *Number `json:"estimatedDowntimeMinutes"`
	State                    *string `json:"state"`
}

// SlaOverview is a target together with where it currently stands. This is the
// measurement, not the definition: the definition is `pathly_sla_target`.
type SlaOverview struct {
	ID                 string     `json:"id"`
	MonitorID          *string    `json:"monitorId"`
	MonitorName        *string    `json:"monitorName"`
	Name               *string    `json:"name"`
	ObjectivePct       *Number    `json:"objectivePct"`
	WindowDays         *int64     `json:"windowDays"`
	ExcludeMaintenance *bool      `json:"excludeMaintenance"`
	WarnAtBudgetRatio  *Number    `json:"warnAtBudgetRatio"`
	Enabled            *bool      `json:"enabled"`
	Status             *SlaStatus `json:"status"`
}

func (c *Client) ListSlaOverview(ctx context.Context) ([]SlaOverview, error) {
	return listItems[SlaOverview](ctx, c, "/v1/sla")
}

// ---------------------------------------------------------------------------
// Collections already covered as single resources
// ---------------------------------------------------------------------------

func (c *Client) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	return listItems[Webhook](ctx, c, "/v1/webhooks")
}

func (c *Client) ListMaintenanceWindows(ctx context.Context) ([]MaintenanceWindow, error) {
	return listItems[MaintenanceWindow](ctx, c, "/v1/maintenance-windows")
}

func (c *Client) ListSlaTargets(ctx context.Context) ([]SlaTarget, error) {
	return listItems[SlaTarget](ctx, c, "/v1/sla-targets")
}

// ---------------------------------------------------------------------------
// Public status page
// ---------------------------------------------------------------------------

// StatusComponent is one line of the public status page.
type StatusComponent struct {
	Name      string  `json:"name"`
	Uptime30d *Number `json:"uptime30d"`
	Uptime90d *Number `json:"uptime90d"`
}

// StatusPage is what the public page displays. The API answers 404 when the
// organization has no page or keeps it private, which the provider reports as
// such instead of showing an empty page.
type StatusPage struct {
	Components []StatusComponent `json:"components"`
	Uptime30d  *Number           `json:"uptime30d"`
}

func (c *Client) GetStatusPage(ctx context.Context) (*StatusPage, error) {
	out := &StatusPage{}
	err := c.do(ctx, request{
		method: http.MethodGet,
		path:   "/v1/status/components",
		out:    out,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// compile-time proof that Number satisfies the decoder the responses need.
var _ json.Unmarshaler = (*Number)(nil)
