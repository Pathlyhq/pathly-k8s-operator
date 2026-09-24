// Package client talks to the public Pathly `/v1` API.
//
// Deliberately hand-written rather than generated from the OpenAPI spec: a
// Terraform provider needs three behaviors a generated client does not give,
// and they decide how reliable an `apply` is — idempotent creates, honoring
// `Retry-After`, and telling "missing" apart from "error" so that a resource
// deleted by hand leaves the state instead of failing the plan.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the production API.
	DefaultBaseURL = "https://api.pathlyhq.com"

	// maxAttempts bounds the retries on 429 and on 5xx. Beyond that, a visible
	// failure is better than an `apply` that looks stuck.
	maxAttempts = 4

	// maxRetryWait caps the wait announced by the server: an absurd
	// `Retry-After` must not freeze a pipeline for an hour.
	maxRetryWait = 90 * time.Second
)

// APIError carries the status and the message returned by the API.
//
// The API messages are written to be displayed as they are: rewording them
// would lose the detail that makes the fix possible (the named missing scope,
// the offending field, the exceeded limit).
type APIError struct {
	StatusCode int
	Message    string
	// Path helps locate the error when an apply touches several resources.
	Path string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s (HTTP %d)", e.Path, e.Message, e.StatusCode)
}

// IsNotFound reports a resource missing from this organization.
func (e *APIError) IsNotFound() bool { return e.StatusCode == http.StatusNotFound }

// IsNotFound is true when the error reports a missing resource.
//
// This is what lets Read drop the resource from the state instead of failing
// the plan when somebody deleted it from the console.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.IsNotFound()
}

// Client is ready to use and safe for concurrent use.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	userAgent  string
	// sleep is replaceable in tests so they do not actually wait.
	sleep func(time.Duration)
}

// Option configures the client at construction time.
type Option func(*Client)

// WithHTTPClient forces an HTTP client, for tests or a corporate proxy.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithUserAgent identifies the provider version in the API logs.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithSleep replaces the wait between two attempts.
func WithSleep(f func(time.Duration)) Option {
	return func(c *Client) { c.sleep = f }
}

// New builds a client. The token is never logged.
func New(baseURL, token string, opts ...Option) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      strings.TrimSpace(token),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		userAgent:  "pathly-k8s-operator",
		sleep:      time.Sleep,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type request struct {
	method string
	path   string
	body   any
	// idempotencyKey only makes sense on a write. Reused as it is from one
	// attempt to the next: this is what avoids the duplicate when the first
	// request went through but the response got lost.
	idempotencyKey string
	out            any
}

// do sends the request and retries the transient failures.
//
// Splitting it with attempt is not cosmetic: the last attempt has to return
// its error instead of asking for a retry, and expressing that with a
// parameter avoids a final path that nothing can reach nor test.
func (c *Client) do(ctx context.Context, r request) error {
	var payload []byte
	if r.body != nil {
		encoded, err := json.Marshal(r.body)
		if err != nil {
			return fmt.Errorf("encoding the body of %s: %w", r.path, err)
		}
		payload = encoded
	}

	for attempt := 1; attempt < maxAttempts; attempt++ {
		retryIn, err := c.attempt(ctx, r, payload, attempt, false)
		if retryIn <= 0 {
			return err
		}
		c.sleep(retryIn)
	}
	_, err := c.attempt(ctx, r, payload, maxAttempts, true)
	return err
}

// attempt returns a non-zero wait when the failure is transient and a retry is
// still allowed. `last` cuts the retries off: the error is returned instead.
func (c *Client) attempt(
	ctx context.Context,
	r request,
	payload []byte,
	attempt int,
	last bool,
) (time.Duration, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, r.method, c.baseURL+r.path, reader)
	if err != nil {
		return 0, fmt.Errorf("building request %s %s: %w", r.method, r.path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", r.idempotencyKey)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		// Transport error: the request may have gone through on the server
		// side. The idempotency key guarantees a retry creates no duplicate.
		wrapped := fmt.Errorf("calling %s %s: %w", r.method, r.path, err)
		if last {
			return 0, wrapped
		}
		return backoff(attempt), wrapped
	}

	body, readErr := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if readErr != nil {
		wrapped := fmt.Errorf("reading the response of %s %s: %w", r.method, r.path, readErr)
		if last {
			return 0, wrapped
		}
		return backoff(attempt), wrapped
	}

	if res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500 {
		failure := apiError(res.StatusCode, r.path, body)
		if last {
			return 0, failure
		}
		return waitFor(res, attempt), failure
	}

	// A request refused for its content or its permissions is not retried: it
	// would be refused identically, while eating into the rate limit.
	if res.StatusCode >= 400 {
		return 0, apiError(res.StatusCode, r.path, body)
	}

	if r.out == nil || len(body) == 0 {
		return 0, nil
	}
	if err := json.Unmarshal(body, r.out); err != nil {
		return 0, fmt.Errorf("unreadable response from %s %s: %w", r.method, r.path, err)
	}
	return 0, nil
}

// waitFor favors the wait announced by the server.
//
// Without it, a provider that reads a whole fleet back on every plan retries
// too early and stays locked inside its own rate limit.
func waitFor(res *http.Response, attempt int) time.Duration {
	if raw := res.Header.Get("Retry-After"); raw != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && secs > 0 {
			wait := time.Duration(secs) * time.Second
			if wait > maxRetryWait {
				return maxRetryWait
			}
			return wait
		}
	}
	return backoff(attempt)
}

func backoff(attempt int) time.Duration {
	return time.Duration(attempt) * 500 * time.Millisecond
}

func apiError(status int, path string, body []byte) *APIError {
	var parsed struct {
		Error string `json:"error"`
		// The API details validation refusals field by field. Without that
		// detail, a refused `apply` only shows "Invalid body": enough to spend
		// a long time looking for which line of the file to fix.
		Details []struct {
			Message string `json:"message"`
			// The path mixes field names and array indices —
			// `["events", 0]` points at the first event. Decoding it into
			// strings used to fail, and made the whole message fall back to
			// the raw JSON body.
			Path []any `json:"path"`
		} `json:"details"`
	}
	message := strings.TrimSpace(string(body))
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error != "" {
		message = parsed.Error
		for _, d := range parsed.Details {
			message += fmt.Sprintf(" [%s: %s]", fieldOf(d.Path), d.Message)
		}
	}
	if message == "" {
		message = http.StatusText(status)
	}
	switch status {
	case http.StatusUnauthorized:
		message += " Check PATHLY_API_TOKEN: an expired, revoked or truncated key gives the same response."
	case http.StatusForbidden:
		message += " Widen the scopes of the key, or check that the plan includes the feature."
	}
	return &APIError{StatusCode: status, Message: message, Path: path}
}

// fieldOf renders a validation path readable: `events.0`, or `body` when the
// refusal is about the whole object.
func fieldOf(path []any) string {
	parts := make([]string, 0, len(path))
	for _, p := range path {
		switch v := p.(type) {
		case string:
			parts = append(parts, v)
		case float64:
			parts = append(parts, strconv.Itoa(int(v)))
		}
	}
	if len(parts) == 0 {
		return "body"
	}
	return strings.Join(parts, ".")
}

// Ping checks that the token is accepted.
//
// Called when the provider is configured: failing here gives one clear message
// once, instead of one error per resource during the plan.
//
// A 403 is not a failure: on the contrary it proves the key is valid, and only
// signals that it does not carry `org:read`. Treating that case as an error
// would force every configuration — even one limited to scenarios — to ask for
// a scope on the organization settings, which is exactly the opposite of least
// privilege.
func (c *Client) Ping(ctx context.Context) error {
	var out struct {
		PlanID string `json:"planId"`
	}
	err := c.do(ctx, request{method: http.MethodGet, path: "/v1/usage", out: &out})
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
		return nil
	}
	return err
}
