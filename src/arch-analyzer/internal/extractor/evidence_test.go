package extractor

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jctanner/arch-analyzer/internal/model"
)

func TestCrossReferencesJoinExplicitNetworkAndSecurityFacts(t *testing.T) {
	input := model.Input{
		HTTPEndpoints:  []model.HTTPEndpoint{{Method: "GET", Path: "/readyz", Port: 8080, Source: "server.go:10"}},
		Services:       []model.Service{{Name: "api", Ports: []model.ServicePort{{Port: 8080}}, Source: "service.yaml:4"}},
		Authentication: []model.AuthenticationFact{{Endpoint: "/readyz", Mechanism: "RBAC", EnforcementPoint: "middleware", Source: "auth.go:3"}},
	}
	refs := crossReferences(input)
	if len(refs) != 2 {
		t.Fatalf("cross references = %#v, want endpoint/service and endpoint/auth joins", refs)
	}
	joined := false
	protected := false
	for _, ref := range refs {
		joined = joined || ref.Relationship == "served-by" && ref.To == "api"
		protected = protected || ref.Relationship == "protected-by" && ref.To == "RBAC"
		if len(ref.Sources) != 2 {
			t.Errorf("reference sources = %#v, want both contributing facts", ref.Sources)
		}
	}
	if !joined || !protected {
		t.Errorf("references = %#v, want explicit service and auth joins", refs)
	}
}

func TestCoverageFindingsDoNotClaimAbsenceForPartialCoverage(t *testing.T) {
	input := model.Input{
		DataCoverage: map[string]string{"go_crds": "partial: parse warning", "manifests": "complete"},
		CategoryCoverage: map[string]model.CategoryCoverage{
			"grpc_services": {Status: "partial"}, "http_endpoints": {Status: "complete"},
			"services": {Status: "complete"},
		},
	}
	findings := coverageFindings(input)
	for _, finding := range findings {
		if finding.Status == "confirmed-empty" && finding.Category == "crds" {
			t.Fatal("partial CRD coverage must not produce a confirmed-empty finding")
		}
	}
	statuses := map[string]string{}
	for _, finding := range findings {
		statuses[finding.Category] = finding.Status
	}
	if statuses["crds"] != "not-verified" || statuses["http_endpoints"] != "confirmed-empty" {
		t.Errorf("findings = %#v, want explicit not-verified and confirmed-empty statuses", findings)
	}
	if !strings.Contains(findings[0].Finding, "absence is not proven") {
		t.Errorf("CRD finding = %#v, want explicit limitation", findings[0])
	}
}

func TestSynthesisEvidenceIsBoundedAndSourceLinked(t *testing.T) {
	input := model.Input{
		HTTPEndpoints:  []model.HTTPEndpoint{{Path: "/a", Source: "a.go:1"}},
		Authentication: []model.AuthenticationFact{{Endpoint: "/a", Mechanism: "OAuth", Source: "auth.go:2"}},
	}
	evidence := synthesisEvidence(input)
	if len(evidence["http_endpoints"]) != 1 || len(evidence["authentication"]) != 1 {
		t.Fatalf("evidence = %#v, want endpoint and auth bundles", evidence)
	}
	for category, records := range evidence {
		for _, record := range records {
			if record.Claim == "" || len(record.Sources) == 0 {
				t.Errorf("%s record = %#v, want claim and provenance", category, record)
			}
		}
	}
}

func TestBehavioralEvidenceProjectsOnlyUnresolvedGaps(t *testing.T) {
	input := model.Input{
		RuntimeSecurity: []model.RuntimeSecurityControl{{
			Surface: "controller-runtime metrics", Source: "cmd/main.go:492",
		}},
		ControllerWatches: []model.ControllerWatch{{
			Type: "Watches", GVK: "/v1/Namespace", Controller: "ServiceHandler",
			Source: "internal/controller/auth/controller.go:63",
		}},
		BehavioralEvidence: []model.BehavioralEvidence{
			{
				Kind: "conditional-metrics-enforcement", Status: "observed",
				Identity: "controller-runtime metrics", ServingSurface: "controller-runtime metrics serving surface",
				ConfigurationBranch: "config.MetricsSecure is true", EnforcementProvider: "filters.WithAuthenticationAndAuthorization",
				Source: "cmd/main.go:485-500",
			},
			{
				Kind: "named-watch-predicate", Status: "unresolved",
				Identity: "internal/controller/auth.ServiceHandler", WatchedGVK: "/v1/Namespace",
				Source:      "internal/controller/auth/controller.go:63-69",
				Limitations: []string{"predicate argument is dynamic"},
			},
		},
	}

	gaps := gapEvidenceIndex(input)
	if len(gaps["authentication"]) != 0 {
		t.Fatalf("authentication gaps = %#v, want observed metrics behavior to suppress its generic hint", gaps["authentication"])
	}
	watchGaps := gaps["kubernetes_relationships"]
	if len(watchGaps) != 1 || watchGaps[0].LineRange != "63-69" ||
		!strings.Contains(watchGaps[0].Question, "literal resource names") {
		t.Fatalf("watch gaps = %#v, want precise unresolved behavioral candidate", watchGaps)
	}
}

func TestPreciseBehavioralGapsSurviveCategoryCapAndSameFileDeduplication(t *testing.T) {
	input := model.Input{}
	for index := 0; index < 14; index++ {
		line := 10 + index*5
		input.BehavioralEvidence = append(input.BehavioralEvidence, model.BehavioralEvidence{
			Kind: "named-watch-predicate", Status: "unresolved",
			Identity:    fmt.Sprintf("internal/controller/missing-%02d.Handler", index),
			WatchedGVK:  "/v1/Namespace",
			Source:      fmt.Sprintf("internal/controller/shared.go:%d-%d", line, line+3),
			Limitations: []string{"predicate argument is dynamic"},
		})
	}
	input.ControllerWatches = []model.ControllerWatch{{
		Type: "Watches", GVK: "/v1/Namespace", Controller: "Handler",
		Source: "internal/controller/shared.go:10",
	}}

	gaps := gapEvidenceIndex(input)["kubernetes_relationships"]
	if len(gaps) != 14 {
		t.Fatalf("precise watch gaps = %d, want all 14 beyond generic cap: %#v", len(gaps), gaps)
	}
	for index, gap := range gaps {
		wantIdentity := fmt.Sprintf("internal/controller/missing-%02d.Handler", index)
		if gap.Source != "internal/controller/shared.go" || !strings.Contains(strings.Join(gap.Symbols, "\x00"), wantIdentity) ||
			!strings.Contains(gap.Question, "literal resource names") {
			t.Fatalf("gap[%d] = %#v, want precise identity/range without generic replacement", index, gap)
		}
	}
}

func TestGapEvidenceOrderingUsesCompleteCandidateTieBreakers(t *testing.T) {
	first := model.IntegrationFact{
		Component: "Zulu", InteractionType: "REST", Source: "client.go:7",
	}
	second := model.IntegrationFact{
		Component: "Alpha", InteractionType: "REST", Source: "client.go:7",
	}
	left := gapEvidenceIndex(model.Input{IntegrationPoints: []model.IntegrationFact{first, second}})
	right := gapEvidenceIndex(model.Input{IntegrationPoints: []model.IntegrationFact{second, first}})

	if !reflect.DeepEqual(left, right) {
		t.Fatalf("gap evidence ordering differs: %#v != %#v", left, right)
	}
	if got := left["integration_points"]; len(got) != 2 || !strings.Contains(strings.Join(got[0].Symbols, ","), "Alpha") || !strings.Contains(strings.Join(got[1].Symbols, ","), "Zulu") {
		t.Fatalf("gap evidence = %#v, want complete symbol tie-break order", got)
	}
}

func TestSourceLocationPreservesBoundedBehavioralRange(t *testing.T) {
	path, lineRange := sourceLocation("cmd/main.go:485-500")
	if path != "cmd/main.go" || lineRange != "485-500" {
		t.Fatalf("source location = %q, %q, want bounded range", path, lineRange)
	}
}

func TestCrossCuttingEvidencePreservesRequiredTopicsAndProvenance(t *testing.T) {
	input := model.Input{
		Authentication: []model.AuthenticationFact{{Endpoint: "/api", Mechanism: "RBAC", Source: "auth.go:4"}},
		IngressRouting: []model.Ingress{{Kind: "HTTPRoute", Name: "api", Host: "api.example", TLS: true, Source: "route.yaml:3"}},
		Dockerfiles:    []model.Dockerfile{{Path: "Dockerfile", BaseImage: "ubi9", User: "1001"}},
	}
	evidence := crossCuttingEvidence(input)
	for _, topic := range []string{"security", "ingress", "supply_chain", "disconnected_deployment", "high_availability", "deployment_topology"} {
		records := evidence[topic]
		if len(records) == 0 {
			t.Fatalf("topic %q has no evidence", topic)
		}
		for _, record := range records {
			if record.Claim == "" || record.Status == "" || len(record.Sources) == 0 {
				t.Errorf("topic %q record = %#v, want claim/status/provenance", topic, record)
			}
		}
	}
}

func TestFIPSCoverageIsPartialAndSourceLinked(t *testing.T) {
	input := model.Input{
		SecurityEvidence: []model.SecurityEvidence{{
			Kind: "fips-posture", Target: "FIPS validation", Status: "not-extracted",
			Detail: "FIPS validation is not verified", Source: "coverage:fips_compliance",
		}},
	}
	coverage := fipsComplianceCoverage(input)
	if coverage.Status != "partial" || coverage.FactCount != 1 {
		t.Fatalf("coverage = %#v, want one partial FIPS fact", coverage)
	}
	if len(coverage.Evidence) != 1 || coverage.Evidence[0] != "coverage:fips_compliance" {
		t.Fatalf("coverage evidence = %#v, want source provenance", coverage.Evidence)
	}
}

func TestFIPSCoverageWithoutSignalsIsNotVerified(t *testing.T) {
	coverage := fipsComplianceCoverage(model.Input{})
	if coverage.Status != "partial" || coverage.FactCount != 0 {
		t.Fatalf("coverage = %#v, want zero-fact partial coverage", coverage)
	}
	if len(coverage.Evidence) != 1 || coverage.Evidence[0] != "coverage:fips_compliance" {
		t.Fatalf("coverage evidence = %#v, want explicit coverage provenance", coverage.Evidence)
	}
	if !strings.Contains(coverage.Limitations[0], "not verified") {
		t.Fatalf("coverage limitations = %#v, want not-verified limitation", coverage.Limitations)
	}
}

func TestGapEvidenceIndexCoversHighDemandCategories(t *testing.T) {
	input := model.Input{
		HTTPEndpoints:     []model.HTTPEndpoint{{Source: "web/routes.py:12"}},
		GRPCServices:      []model.GRPCService{{Source: "rust/src/server.rs:8"}},
		Authentication:    []model.AuthenticationFact{{Source: "auth.go:4"}},
		IntegrationPoints: []model.IntegrationFact{{Source: "client.py:7"}},
		Dependencies:      model.Dependencies{Internal: []model.InternalDependency{{Source: "controller.go:9"}}},
		RuntimeClients:    []model.RuntimeClient{{Source: "egress.go:5"}},
		Services:          []model.Service{{Source: "service.yaml:2"}},
		Deployments:       []model.Deployment{{Source: "deployment.yaml:3"}},
		ControllerWatches: []model.ControllerWatch{{Controller: "WidgetReconciler", GVK: "example.io/v1/Widget", Source: "controller.go:11"}},
		RBAC:              model.RBAC{ClusterRoles: []model.Role{{Name: "manager", Source: "rbac.yaml:4"}}},
		SourceDefaults:    []model.SourceDefault{{Path: "--port", Sources: []string{"config.go:6"}}},
		Webhooks:          []model.Webhook{{Name: "widget", Path: "/validate", Sources: []model.WebhookSource{{File: "webhook.go", Line: 12}}}},
	}
	index := gapEvidenceIndex(input)
	for _, category := range []string{"http_endpoints", "grpc_services", "authentication", "integration_points", "internal_dependencies", "egress", "services"} {
		candidates := index[category]
		if len(candidates) == 0 {
			t.Errorf("gap category %q has no candidates", category)
		}
		for _, candidate := range candidates {
			if candidate.Status != "candidate" || candidate.Source == "" || candidate.Question == "" || candidate.ExpectedSignal == "" {
				t.Errorf("category %q candidate = %#v, want bounded source-backed metadata", category, candidate)
			}
		}
	}
	for _, category := range []string{"kubernetes_relationships", "authorization", "configuration_lifecycle", "webhooks"} {
		if len(index[category]) == 0 {
			t.Errorf("extended gap category %q has no candidates", category)
		}
	}
}

func TestGapEvidenceIndexIncludesEntrypointAppNameForAuthentication(t *testing.T) {
	input := model.Input{Entrypoints: []model.Entrypoint{
		{Name: "Dockerfile:CMD", Command: `["python", "-m", "server", "--app-name", "kubernetes-auth"]`, Source: "Dockerfile:10"},
	}}

	candidates := gapEvidenceIndex(input)["authentication"]
	if len(candidates) != 1 {
		t.Fatalf("authentication candidates = %#v, want one deduplicated entrypoint candidate", candidates)
	}
	for _, candidate := range candidates {
		if candidate.Source != "Dockerfile" {
			t.Errorf("candidate source = %q, want Dockerfile", candidate.Source)
		}
		if !strings.Contains(candidate.Question, "app/plugin") {
			t.Errorf("candidate question = %q, want app/plugin guidance", candidate.Question)
		}
		if !strings.Contains(candidate.ExpectedSignal, "authentication") {
			t.Errorf("candidate expected signal = %q, want authentication guidance", candidate.ExpectedSignal)
		}
	}
	if got := literalFlagValue(`python -m server --app-name=basic-auth`, "--app-name"); got != "basic-auth" {
		t.Fatalf("literalFlagValue() = %q, want basic-auth", got)
	}
}

func TestLiteralFlagValueDoesNotInterpretShellExpansion(t *testing.T) {
	if got := literalFlagValue(`python -m server --app-name "$APP_NAME"`, "--app-name"); got != "$APP_NAME" {
		t.Fatalf("literalFlagValue() = %q, want literal shell value", got)
	}
	if got := literalFlagValue(`python -m server --other value`, "--app-name"); got != "" {
		t.Fatalf("literalFlagValue() = %q, want empty value", got)
	}
}
