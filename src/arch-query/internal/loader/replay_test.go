package loader

import (
	"os"
	"reflect"
	"testing"

	"github.com/jctanner/arch-query/internal/types"
)

// TestPhaseOneReplayCorpusParity is an opt-in replay check for a coordinator's
// frozen phase-one corpus. Normal unit tests use the checked-in Praxis fixture;
// reviewers can set both roots to independently replay a larger accepted set.
func TestPhaseOneReplayCorpusParity(t *testing.T) {
	acceptedRoot := os.Getenv("SC18_REPLAY_ACCEPTED_ROOT")
	legacyRoot := os.Getenv("SC18_REPLAY_LEGACY_ROOT")
	version := os.Getenv("SC18_REPLAY_VERSION")
	if acceptedRoot == "" || legacyRoot == "" || version == "" {
		t.Skip("SC18 replay roots and version not provided")
	}
	accepted, err := LoadVersion(os.DirFS(acceptedRoot), nil, version)
	if err != nil {
		t.Fatalf("load accepted replay: %v", err)
	}
	legacy, err := LoadVersion(os.DirFS(legacyRoot), nil, version)
	if err != nil {
		t.Fatalf("load legacy replay: %v", err)
	}
	if len(accepted.Components) != len(legacy.Components) {
		t.Fatalf("component count accepted=%d legacy=%d", len(accepted.Components), len(legacy.Components))
	}
	for name, acceptedDoc := range accepted.Components {
		legacyDoc := legacy.Components[name]
		if legacyDoc == nil {
			t.Errorf("legacy replay missing component %q", name)
			continue
		}
		if got, want := replayLegacyProjection(acceptedDoc), replayLegacyProjection(legacyDoc); !reflect.DeepEqual(got, want) {
			t.Errorf("component %q typed projection differs\naccepted: %#v\nlegacy: %#v", name, got, want)
		}
	}
}

type replayProjection struct {
	Name, Repository, Branch, Version, Languages, DeployType, Purpose, PurposeFull string
	Metadata                                                                       map[string]string
	Components                                                                     []types.ArchComponent
	CRDs                                                                           []types.CRD
	Endpoints                                                                      []types.Endpoint
	GRPCServices                                                                   []types.GRPCService
	ExternalDeps, InternalDeps                                                     []types.Dependency
	Services                                                                       []types.Service
	Ingresses                                                                      []types.Ingress
	Egresses                                                                       []types.Egress
	RBACRoles                                                                      []types.RBACRole
}

func replayLegacyProjection(doc *types.ComponentDoc) replayProjection {
	return replayProjection{
		Name: doc.Name, Repository: doc.Repository, Branch: doc.Branch, Version: doc.Version,
		Languages: doc.Languages, DeployType: doc.DeployType, Purpose: doc.Purpose, PurposeFull: doc.PurposeFull,
		Metadata: doc.Metadata, Components: doc.Components, CRDs: doc.CRDs, Endpoints: doc.Endpoints,
		GRPCServices: doc.GRPCServices, ExternalDeps: doc.ExternalDeps, InternalDeps: doc.InternalDeps,
		Services: doc.Services, Ingresses: doc.Ingresses, Egresses: doc.Egresses, RBACRoles: doc.RBACRoles,
	}
}
