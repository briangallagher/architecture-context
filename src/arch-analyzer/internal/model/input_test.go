package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeInputAcceptsCompatibilityJSONAndUnknownFields(t *testing.T) {
	input, err := DecodeInput(strings.NewReader(`{
  "component": "example",
  "repo": "example/example",
  "services": [{
    "name": "api",
    "ports": [
      {"port": 8443, "targetPort": "https", "protocol": "TCP"},
      {"port": "metrics", "targetPort": 8080, "protocol": "TCP"}
    ]
  }],
  "field_from_a_newer_schema": {"ignored": true}
}`))
	if err != nil {
		t.Fatalf("DecodeInput() error = %v", err)
	}
	if input.Component != "example" {
		t.Fatalf("Component = %q, want example", input.Component)
	}
	if got := input.Services[0].Ports[0].Port; got != float64(8443) {
		t.Fatalf("numeric port = %#v, want 8443", got)
	}
	if got := input.Services[0].Ports[1].Port; got != "metrics" {
		t.Fatalf("string port = %#v, want metrics", got)
	}
}

func TestDecodeInputRequiresComponent(t *testing.T) {
	_, err := DecodeInput(strings.NewReader(`{"repo":"example/example"}`))
	if err == nil || !strings.Contains(err.Error(), "missing component") {
		t.Fatalf("DecodeInput() error = %v, want missing component", err)
	}
}

func TestCategoryCoverageRoundTrips(t *testing.T) {
	input := Input{
		Component: "example",
		ScanStatistics: []ScanStatistic{{
			Category: "authentication", Metric: "files_scanned", Value: 12,
			Unit: "files", Scope: "Python source files",
		}},
		CategoryCoverage: map[string]CategoryCoverage{
			"authentication": {
				Status: "complete", FactCount: 0,
				DiscoveryContract: "authentication/v1",
				CompletedChecks:   []string{"runtime-inventory"},
				Limitations:       []string{},
				Evidence:          []string{"summary:no inbound surfaces"},
			},
		},
	}
	var encoded strings.Builder
	if err := EncodeInput(&encoded, input); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(encoded.String()), &raw); err != nil {
		t.Fatal(err)
	}
	record := raw["category_coverage"].(map[string]any)["authentication"].(map[string]any)
	for _, field := range []string{"status", "fact_count", "discovery_contract", "completed_checks", "limitations", "evidence"} {
		if _, exists := record[field]; !exists {
			t.Errorf("encoded category coverage missing %q: %#v", field, record)
		}
	}
	decoded, err := DecodeInput(strings.NewReader(encoded.String()))
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.CategoryCoverage["authentication"]
	if got.Status != "complete" || got.DiscoveryContract != "authentication/v1" || len(got.CompletedChecks) != 1 {
		t.Fatalf("category coverage = %#v", got)
	}
	if len(decoded.ScanStatistics) != 1 || decoded.ScanStatistics[0].Value != 12 {
		t.Fatalf("scan statistics = %#v", decoded.ScanStatistics)
	}
}

func TestUnknownCategoryCoverageRoundTrips(t *testing.T) {
	input := Input{
		Component: "legacy-adapter",
		CategoryCoverage: map[string]CategoryCoverage{
			"authentication": {
				Status: "unknown", FactCount: 0,
				DiscoveryContract: "authentication/v1",
				CompletedChecks:   []string{},
				Limitations:       []string{"coverage was not collected"},
				Evidence:          []string{},
			},
		},
	}
	var encoded strings.Builder
	if err := EncodeInput(&encoded, input); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeInput(strings.NewReader(encoded.String()))
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.CategoryCoverage["authentication"].Status; got != "unknown" {
		t.Fatalf("status = %q, want unknown", got)
	}
}

func TestBehavioralEvidenceRoundTripsWithoutChangingLegacyInput(t *testing.T) {
	input := Input{
		Component: "operator",
		BehavioralEvidence: []BehavioralEvidence{{
			Kind: "named-watch-predicate", Status: "observed",
			Identity: "internal/controller/auth.ServiceHandler", WatchedGVK: "/v1/Namespace",
			LiteralValues: []string{"models-as-a-service"},
			EventTarget:   "services.platform.opendatahub.io/v1alpha1/Auth/auth",
			Source:        "internal/controller/auth/controller.go:63-69",
		}},
	}
	var encoded strings.Builder
	if err := EncodeInput(&encoded, input); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeInput(strings.NewReader(encoded.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.BehavioralEvidence) != 1 || decoded.BehavioralEvidence[0].LiteralValues[0] != "models-as-a-service" {
		t.Fatalf("behavioral evidence = %#v, want lossless compatibility round trip", decoded.BehavioralEvidence)
	}
}

func TestEncodeInputIncludesEmptyBehavioralEvidence(t *testing.T) {
	input := Input{Component: "behavior-free"}
	var encoded strings.Builder
	if err := EncodeInput(&encoded, input); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(encoded.String()), &raw); err != nil {
		t.Fatal(err)
	}
	evidence, exists := raw["behavioral_evidence"]
	if !exists {
		t.Fatal("encoded input omitted behavioral_evidence")
	}
	items, ok := evidence.([]any)
	if !ok || len(items) != 0 {
		t.Fatalf("behavioral_evidence = %#v, want empty array", evidence)
	}
}

func TestDecodeInputAcceptsMissingLegacyBehavioralEvidence(t *testing.T) {
	input, err := DecodeInput(strings.NewReader(`{"component":"legacy"}`))
	if err != nil {
		t.Fatal(err)
	}
	if input.BehavioralEvidence != nil {
		t.Fatalf("behavioral evidence = %#v, want absent legacy value", input.BehavioralEvidence)
	}
}
