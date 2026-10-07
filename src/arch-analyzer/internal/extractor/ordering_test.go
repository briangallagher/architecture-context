package extractor

import (
	"reflect"
	"testing"

	"github.com/jctanner/arch-analyzer/internal/model"
)

func TestProducerSetOrderingIsCanonicalAndPreservesFacts(t *testing.T) {
	entrypoints := []model.Entrypoint{
		{Name: "worker", Type: "Python console script", Runtime: "Python", Command: "app:worker", Source: "pyproject.toml:9"},
		{Name: "api", Type: "Python console script", Runtime: "Python", Command: "app:api", Source: "pyproject.toml:8"},
	}
	integrations := []model.IntegrationFact{
		{Component: "OpenShift Routes", InteractionType: "CRD Watch", Role: "runtime-integration", Protocol: "HTTPS", Source: "role.yaml:2"},
		{Component: "OpenShift Image Streams", InteractionType: "REST", Role: "runtime-transport", Port: 6443, Protocol: "HTTPS", Source: "role.yaml:2"},
	}
	reversedEntrypoints := []model.Entrypoint{entrypoints[1], entrypoints[0]}
	reversedIntegrations := []model.IntegrationFact{integrations[1], integrations[0]}

	sortEntrypoints(entrypoints)
	sortEntrypoints(reversedEntrypoints)
	sortIntegrationPoints(integrations)
	sortIntegrationPoints(reversedIntegrations)

	if !reflect.DeepEqual(entrypoints, reversedEntrypoints) {
		t.Fatalf("entrypoint ordering differs: %#v != %#v", entrypoints, reversedEntrypoints)
	}
	if !reflect.DeepEqual(integrations, reversedIntegrations) {
		t.Fatalf("integration ordering differs: %#v != %#v", integrations, reversedIntegrations)
	}
	if len(entrypoints) != 2 || len(integrations) != 2 {
		t.Fatal("canonical ordering must preserve every producer fact")
	}
}

func TestSecurityEvidenceOrderingAndMergedSourcesAreCanonical(t *testing.T) {
	records := []model.SecurityEvidence{
		{Kind: "rbac-ref", Target: "z.example/auth", Detail: "RBAC import", Status: "dependency-signal", Source: "z.go"},
		{Kind: "rbac-ref", Target: "a.example/auth", Detail: "RBAC import", Status: "dependency-signal", Source: "a.go"},
		{Kind: "rbac-ref", Target: "z.example/auth", Detail: "RBAC import", Status: "dependency-signal", Source: "a.go"},
	}
	reversed := []model.SecurityEvidence{records[2], records[1], records[0]}

	got := dedupeSecurityEvidence(records)
	want := dedupeSecurityEvidence(reversed)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("security evidence differs: %#v != %#v", got, want)
	}
	if len(got) != 2 || !reflect.DeepEqual(got[1].Sources, []string{"a.go", "z.go"}) {
		t.Fatalf("security evidence = %#v, want two facts and both sorted sources", got)
	}
}
