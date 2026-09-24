package client

import (
	"context"
	"net/http"
	"net/url"
)

// ---------------------------------------------------------------------------
// Scenarios
// ---------------------------------------------------------------------------

// Scenario mirrors the public projection of a scenario, field for field.
//
// The pointers tell "missing" apart from "empty": the API returns `null` for
// an attribute that is not set, and writing 0 or "" instead would make the
// plan drift on every read.
type Scenario struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	URL                 *string  `json:"url"`
	Enabled             *bool    `json:"enabled"`
	IntervalSec         *int64   `json:"intervalSec"`
	Method              *string  `json:"method"`
	ExpectedStatus      *int64   `json:"expectedStatus"`
	MaxLatencyMs        *int64   `json:"maxLatencyMs"`
	ExpectText          *string  `json:"expectText"`
	Runbook             *string  `json:"runbook"`
	Cron                *string  `json:"cron"`
	LastStatus          *string  `json:"lastStatus"`
	Regions             []string `json:"regions"`
	Tags                []string `json:"tags"`
	Folder              *string  `json:"folder"`
	Severity            *string  `json:"severity"`
	MutedUntil          *string  `json:"mutedUntil"`
	ScenarioFingerprint *string  `json:"scenarioFingerprint"`
	CreatedAt           *string  `json:"createdAt"`
}

// ScenarioInput serves both the create and the update.
//
// `omitempty` on pointers: a field absent from the plan is not sent, and the
// API keeps the existing value. Sending a zero would erase a setting made from
// the console.
type ScenarioInput struct {
	Name           *string      `json:"name,omitempty"`
	Type           *string      `json:"type,omitempty"`
	URL            *string      `json:"url,omitempty"`
	IntervalSec    *int64       `json:"intervalSec,omitempty"`
	Method         *string      `json:"method,omitempty"`
	ExpectedStatus *int64       `json:"expectedStatus,omitempty"`
	MaxLatencyMs   *int64       `json:"maxLatencyMs,omitempty"`
	ExpectText     *string      `json:"expectText,omitempty"`
	Regions        []string     `json:"regions,omitempty"`
	Tags           []string     `json:"tags,omitempty"`
	Folder         *string      `json:"folder,omitempty"`
	Severity       *string      `json:"severity,omitempty"`
	Runbook        *string      `json:"runbook,omitempty"`
	Cron           *string      `json:"cron,omitempty"`
	Enabled        *bool        `json:"enabled,omitempty"`
	Scenario       *ScenarioAst `json:"scenario,omitempty"`
}

func (c *Client) CreateScenario(ctx context.Context, in ScenarioInput, idempotencyKey string) (*Scenario, error) {
	out := &Scenario{}
	err := c.do(ctx, request{
		method:         http.MethodPost,
		path:           "/v1/scenarios",
		body:           in,
		idempotencyKey: idempotencyKey,
		out:            out,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetScenario(ctx context.Context, id string) (*Scenario, error) {
	out := &Scenario{}
	err := c.do(ctx, request{
		method: http.MethodGet,
		path:   "/v1/scenarios/" + url.PathEscape(id),
		out:    out,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) UpdateScenario(ctx context.Context, id string, in ScenarioInput) (*Scenario, error) {
	out := &Scenario{}
	err := c.do(ctx, request{
		method: http.MethodPatch,
		path:   "/v1/scenarios/" + url.PathEscape(id),
		body:   in,
		out:    out,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) DeleteScenario(ctx context.Context, id string) error {
	return c.do(ctx, request{
		method: http.MethodDelete,
		path:   "/v1/scenarios/" + url.PathEscape(id),
	})
}

// ListScenarios walks through every page.
//
// A data source that returned only the first page would produce silently
// incomplete `for_each` loops.
func (c *Client) ListScenarios(ctx context.Context) ([]Scenario, error) {
	return listPaged[Scenario](ctx, c, "/v1/scenarios", nil)
}

// MuteScenario mutes or unmutes. A nil `until` wakes the scenario up.
func (c *Client) MuteScenario(ctx context.Context, id string, until *string) error {
	body := map[string]any{"mutedUntil": until}
	return c.do(ctx, request{
		method: http.MethodPost,
		path:   "/v1/scenarios/" + url.PathEscape(id) + "/mute",
		body:   body,
	})
}

// ---------------------------------------------------------------------------
// Maintenance windows
// ---------------------------------------------------------------------------

type MaintenanceWindow struct {
	ID          string  `json:"id"`
	MonitorID   *string `json:"monitorId"`
	StartsAt    *string `json:"startsAt"`
	EndsAt      *string `json:"endsAt"`
	Reason      *string `json:"reason"`
	Weekday     *int64  `json:"weekday"`
	StartMinute *int64  `json:"startMinute"`
	DurationMin *int64  `json:"durationMin"`
}

type MaintenanceWindowInput struct {
	MonitorID   *string `json:"monitorId,omitempty"`
	StartsAt    *string `json:"startsAt,omitempty"`
	EndsAt      *string `json:"endsAt,omitempty"`
	Reason      *string `json:"reason,omitempty"`
	Weekday     *int64  `json:"weekday,omitempty"`
	StartMinute *int64  `json:"startMinute,omitempty"`
	DurationMin *int64  `json:"durationMin,omitempty"`
}

func (c *Client) CreateMaintenanceWindow(ctx context.Context, in MaintenanceWindowInput, idempotencyKey string) (*MaintenanceWindow, error) {
	out := &MaintenanceWindow{}
	err := c.do(ctx, request{
		method:         http.MethodPost,
		path:           "/v1/maintenance-windows",
		body:           in,
		idempotencyKey: idempotencyKey,
		out:            out,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetMaintenanceWindow(ctx context.Context, id string) (*MaintenanceWindow, error) {
	out := &MaintenanceWindow{}
	err := c.do(ctx, request{
		method: http.MethodGet,
		path:   "/v1/maintenance-windows/" + url.PathEscape(id),
		out:    out,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) DeleteMaintenanceWindow(ctx context.Context, id string) error {
	return c.do(ctx, request{
		method: http.MethodDelete,
		path:   "/v1/maintenance-windows/" + url.PathEscape(id),
	})
}

// ---------------------------------------------------------------------------
// Outbound webhooks
// ---------------------------------------------------------------------------

// Webhook does not carry the URL: the API never returns it on read, only its
// fingerprint. The provider therefore compares fingerprints to detect that a
// destination changed outside of Terraform.
type Webhook struct {
	ID             string   `json:"id"`
	Events         []string `json:"events"`
	Enabled        *bool    `json:"enabled"`
	HasSecret      *bool    `json:"hasSecret"`
	URLFingerprint *string  `json:"urlFingerprint"`
	CreatedAt      *string  `json:"createdAt"`
	// Secret is only filled in on create, once.
	Secret *string `json:"secret"`
}

type WebhookInput struct {
	URL    string   `json:"url"`
	Events []string `json:"events,omitempty"`
}

func (c *Client) CreateWebhook(ctx context.Context, in WebhookInput, idempotencyKey string) (*Webhook, error) {
	out := &Webhook{}
	err := c.do(ctx, request{
		method:         http.MethodPost,
		path:           "/v1/webhooks",
		body:           in,
		idempotencyKey: idempotencyKey,
		out:            out,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetWebhook(ctx context.Context, id string) (*Webhook, error) {
	out := &Webhook{}
	err := c.do(ctx, request{
		method: http.MethodGet,
		path:   "/v1/webhooks/" + url.PathEscape(id),
		out:    out,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	return c.do(ctx, request{
		method: http.MethodDelete,
		path:   "/v1/webhooks/" + url.PathEscape(id),
	})
}

// ---------------------------------------------------------------------------
// SLA targets
// ---------------------------------------------------------------------------

type SlaTarget struct {
	ID                 string   `json:"id"`
	MonitorID          *string  `json:"monitorId"`
	Name               *string  `json:"name"`
	ObjectivePct       *float64 `json:"objectivePct"`
	WindowDays         *int64   `json:"windowDays"`
	ExcludeMaintenance *bool    `json:"excludeMaintenance"`
	WarnAtBudgetRatio  *float64 `json:"warnAtBudgetRatio"`
	Enabled            *bool    `json:"enabled"`
}

type SlaTargetInput struct {
	MonitorID          *string  `json:"monitorId,omitempty"`
	Name               *string  `json:"name,omitempty"`
	ObjectivePct       float64  `json:"objectivePct"`
	WindowDays         int64    `json:"windowDays"`
	ExcludeMaintenance *bool    `json:"excludeMaintenance,omitempty"`
	WarnAtBudgetRatio  *float64 `json:"warnAtBudgetRatio,omitempty"`
	Enabled            *bool    `json:"enabled,omitempty"`
}

// UpsertSlaTarget creates or replaces. The API exposes a PUT: a target is
// identified by its scenario, not by an identifier one would have to guess.
func (c *Client) UpsertSlaTarget(ctx context.Context, in SlaTargetInput, idempotencyKey string) (*SlaTarget, error) {
	out := &SlaTarget{}
	err := c.do(ctx, request{
		method:         http.MethodPut,
		path:           "/v1/sla-targets",
		body:           in,
		idempotencyKey: idempotencyKey,
		out:            out,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetSlaTarget(ctx context.Context, id string) (*SlaTarget, error) {
	out := &SlaTarget{}
	err := c.do(ctx, request{
		method: http.MethodGet,
		path:   "/v1/sla-targets/" + url.PathEscape(id),
		out:    out,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) DeleteSlaTarget(ctx context.Context, id string) error {
	return c.do(ctx, request{
		method: http.MethodDelete,
		path:   "/v1/sla-targets/" + url.PathEscape(id),
	})
}
