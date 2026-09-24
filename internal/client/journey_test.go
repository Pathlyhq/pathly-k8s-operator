package client

import (
	"encoding/json"
	"testing"
)

func TestStepMarshalOmitsEmptyFieldsAndPicksTheRightRegex(t *testing.T) {
	gotoStep := ScenarioStep{Op: "goto", URL: ptr("https://shop.example.com")}
	raw, err := json.Marshal(gotoStep)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["op"] != "goto" || got["url"] != "https://shop.example.com" {
		t.Errorf("goto = %v", got)
	}
	if _, present := got["selector"]; present {
		t.Errorf("unused fields must stay out: %v", got)
	}

	flag := true
	assertText := ScenarioStep{Op: "assert_text", Text: ptr("Cart"), Regex: &flag}
	raw, _ = json.Marshal(assertText)
	_ = json.Unmarshal(raw, &got)
	if got["regex"] != true {
		t.Errorf("assert_text regex must stay a boolean: %v", got)
	}

	pattern := "^/checkout"
	assertURL := ScenarioStep{Op: "assert_url", URLRegex: &pattern}
	raw, _ = json.Marshal(assertURL)
	_ = json.Unmarshal(raw, &got)
	if got["regex"] != "^/checkout" {
		t.Errorf("assert_url regex must stay a string: %v", got)
	}

	amount := 19.9
	assertAmount := ScenarioStep{Op: "assert_amount", Equals: &amount}
	raw, _ = json.Marshal(assertAmount)
	_ = json.Unmarshal(raw, &got)
	if got["equals"] != 19.9 {
		t.Errorf("amount equals = %v", got)
	}

	jsonEquals := "ok"
	assertJSON := ScenarioStep{Op: "assert_json_path", Path: ptr("$.status"), JSONEquals: &jsonEquals}
	raw, _ = json.Marshal(assertJSON)
	_ = json.Unmarshal(raw, &got)
	if got["equals"] != "ok" || got["path"] != "$.status" {
		t.Errorf("json path = %v", got)
	}

	ms := int64(200)
	off := false
	min := 1.5
	full := ScenarioStep{
		Op: "click", Selector: ptr("#go"), Text: ptr("Pay"), File: ptr("a.pdf"),
		Href: ptr("/pay"), URLIncludes: ptr("/api"), Path: ptr("/cart"),
		Key: ptr("Enter"), Currency: ptr("EUR"), Name: ptr("x"),
		Username: ptr("u"), Password: ptr("p"), Includes: ptr("ok"),
		Ms: &ms, TimeoutMs: &ms, Status: &ms, RetryTimes: &ms,
		NewTab: &off, IgnoreCase: &off, IgnoreHash: &off, IgnoreQuery: &off,
		Min: &min, Max: &min,
	}
	raw, err = json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["newTab"] != false || got["min"] != 1.5 || got["ms"] != float64(200) {
		t.Errorf("full step = %v", got)
	}
}

func ptr(v string) *string { return &v }
