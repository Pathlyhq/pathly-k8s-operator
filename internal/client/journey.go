package client

import "encoding/json"

// ScenarioAst is the journey tree the API accepts and never reads back.
//
// A browser run walks `Steps`. The API stores the tree encrypted, and the
// public projection only returns `scenarioFingerprint`: the steps carry login
// credentials, and leaking them through a GET would put them in every log and
// every Terraform state refresh.
type ScenarioAst struct {
	Steps        []ScenarioStep    `json:"steps,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	ClickDelayMs *int64            `json:"clickDelayMs,omitempty"`
	Viewport     *string           `json:"viewport,omitempty"`
	Locale       *string           `json:"locale,omitempty"`
	Timezone     *string           `json:"timezone,omitempty"`
	BasicAuth    *BasicAuth        `json:"basicAuth,omitempty"`
}

// BasicAuth is HTTP authentication attached to the first request of a journey.
type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ScenarioStep is one action of a browser journey.
//
// Pointers keep unused fields out of the request: the API validates a
// discriminated union on `op`, and a zero that does not belong to that op is a
// 400. `regex` and `equals` change type with the operation, so they go through
// MarshalJSON rather than a single Go type.
type ScenarioStep struct {
	Op          string
	URL         *string
	Selector    *string
	Value       *string
	Text        *string
	Ms          *int64
	TimeoutMs   *int64
	Includes    *string
	File        *string
	NewTab      *bool
	Href        *string
	URLIncludes *string
	IgnoreCase  *bool
	Regex       *bool
	URLRegex    *string
	Path        *string
	IgnoreHash  *bool
	IgnoreQuery *bool
	Key         *string
	Status      *int64
	RetryTimes  *int64
	Currency    *string
	Min         *float64
	Max         *float64
	Equals      *float64
	Name        *string
	Username    *string
	Password    *string
	JSONEquals  *string
}

func (s ScenarioStep) MarshalJSON() ([]byte, error) {
	wire := map[string]any{"op": s.Op}
	put := func(key string, value any) {
		switch v := value.(type) {
		case *string:
			if v != nil {
				wire[key] = *v
			}
		case *int64:
			if v != nil {
				wire[key] = *v
			}
		case *bool:
			if v != nil {
				wire[key] = *v
			}
		case *float64:
			if v != nil {
				wire[key] = *v
			}
		}
	}
	put("url", s.URL)
	put("selector", s.Selector)
	put("value", s.Value)
	put("text", s.Text)
	put("ms", s.Ms)
	put("timeoutMs", s.TimeoutMs)
	put("includes", s.Includes)
	put("file", s.File)
	put("newTab", s.NewTab)
	put("href", s.Href)
	put("urlIncludes", s.URLIncludes)
	put("ignoreCase", s.IgnoreCase)
	put("path", s.Path)
	put("ignoreHash", s.IgnoreHash)
	put("ignoreQuery", s.IgnoreQuery)
	put("key", s.Key)
	put("status", s.Status)
	put("retryTimes", s.RetryTimes)
	put("currency", s.Currency)
	put("min", s.Min)
	put("max", s.Max)
	put("name", s.Name)
	put("username", s.Username)
	put("password", s.Password)
	// `regex` is a boolean on assert_text and a string on assert_url. Sending
	// the wrong type is a 400 that names neither the step nor the field.
	if s.URLRegex != nil {
		wire["regex"] = *s.URLRegex
	} else if s.Regex != nil {
		wire["regex"] = *s.Regex
	}
	if s.JSONEquals != nil {
		wire["equals"] = *s.JSONEquals
	} else if s.Equals != nil {
		wire["equals"] = *s.Equals
	}
	return json.Marshal(wire)
}
