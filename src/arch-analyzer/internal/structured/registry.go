package structured

import "github.com/jctanner/arch-analyzer/internal/model"

type factShape int

const (
	shapeSingleton factShape = iota
	shapeArray
	shapeNestedSingleton
	shapeNestedArray
	shapeMap
	shapeMapArray
)

type factDescriptor struct {
	Type     string
	Top      string
	Child    string
	Shape    factShape
	Rendered bool
	Sections []string
	NewValue func() any
}

func ptr[T any]() func() any { return func() any { return new(T) } }

var factRegistry = []factDescriptor{
	{"summary", "summary", "", shapeSingleton, true, []string{"purpose"}, ptr[string]()},
	{"source_component", "source_components", "", shapeArray, true, []string{"architecture-components"}, ptr[model.SourceComponent]()},
	{"crd", "crds", "", shapeArray, true, []string{"apis-exposed"}, ptr[model.CRD]()},
	{"serving_runtime_definition", "serving_runtime_definitions", "", shapeArray, true, []string{"apis-exposed"}, ptr[model.ServingRuntimeDefinition]()},
	{"api_reference_contract", "api_reference_contracts", "", shapeArray, false, []string{"apis-exposed"}, ptr[model.APIReferenceContract]()},
	{"field_projection", "field_projections", "", shapeArray, false, []string{"apis-exposed"}, ptr[model.FieldProjection]()},
	{"managed_component_contract", "managed_component_contracts", "", shapeArray, false, []string{"architecture-components"}, ptr[model.ManagedComponentContract]()},
	{"service", "services", "", shapeArray, true, []string{"network-architecture"}, ptr[model.Service]()},
	{"deployment", "deployments", "", shapeArray, true, []string{"architecture-components"}, ptr[model.Deployment]()},
	{"rbac_cluster_role", "rbac", "cluster_roles", shapeNestedArray, true, []string{"security"}, ptr[model.Role]()},
	{"rbac_role", "rbac", "roles", shapeNestedArray, true, []string{"security"}, ptr[model.Role]()},
	{"rbac_cluster_role_binding", "rbac", "cluster_role_bindings", shapeNestedArray, true, []string{"security"}, ptr[model.Binding]()},
	{"rbac_role_binding", "rbac", "role_bindings", shapeNestedArray, true, []string{"security"}, ptr[model.Binding]()},
	{"secret_reference", "secrets_referenced", "", shapeArray, true, []string{"security"}, ptr[model.Secret]()},
	{"http_endpoint", "http_endpoints", "", shapeArray, true, []string{"apis-exposed"}, ptr[model.HTTPEndpoint]()},
	{"grpc_service", "grpc_services", "", shapeArray, true, []string{"apis-exposed"}, ptr[model.GRPCService]()},
	{"dependency_go_version", "dependencies", "go_version", shapeNestedSingleton, false, []string{"metadata"}, ptr[string]()},
	{"dependency_go_module", "dependencies", "go_modules", shapeNestedArray, true, []string{"dependencies"}, ptr[model.GoModule]()},
	{"dependency_package", "dependencies", "packages", shapeNestedArray, true, []string{"dependencies"}, ptr[model.LanguagePackage]()},
	{"dependency_internal", "dependencies", "internal_odh", shapeNestedArray, true, []string{"dependencies", "integration-points"}, ptr[model.InternalDependency]()},
	{"controller_watch", "controller_watches", "", shapeArray, true, []string{"integration-points"}, ptr[model.ControllerWatch]()},
	{"webhook", "webhooks", "", shapeArray, true, []string{"admission-webhooks", "apis-exposed"}, ptr[model.Webhook]()},
	{"external_webhook", "external_webhooks", "", shapeArray, true, []string{"integration-points"}, ptr[model.ExternalWebhook]()},
	{"ingress_route", "ingress_routing", "", shapeArray, true, []string{"network-architecture"}, ptr[model.Ingress]()},
	{"external_connection", "external_connections", "", shapeArray, true, []string{"network-architecture", "integration-points"}, ptr[model.ExternalConnection]()},
	{"authentication", "authentication", "", shapeArray, true, []string{"security"}, ptr[model.AuthenticationFact]()},
	{"integration_point", "integration_points", "", shapeArray, true, []string{"integration-points"}, ptr[model.IntegrationFact]()},
	{"recent_change", "recent_changes", "", shapeArray, true, []string{"recent-changes"}, ptr[model.RecentChange]()},
	{"component_ref", "component_refs", "", shapeArray, true, []string{"integration-points"}, ptr[model.ComponentRef]()},
	{"entrypoint", "entrypoints", "", shapeArray, true, []string{"architecture-components"}, ptr[model.Entrypoint]()},
	{"security_evidence", "security_evidence", "", shapeArray, true, []string{"security"}, ptr[model.SecurityEvidence]()},
	{"dockerfile", "dockerfiles", "", shapeArray, true, []string{"architecture-components", "metadata"}, ptr[model.Dockerfile]()},
	{"source_default", "source_defaults", "", shapeArray, false, []string{"network-architecture"}, ptr[model.SourceDefault]()},
	{"runtime_client", "runtime_clients", "", shapeArray, false, []string{"integration-points"}, ptr[model.RuntimeClient]()},
	{"runtime_module_use", "runtime_module_uses", "", shapeArray, false, []string{"dependencies"}, ptr[model.RuntimeModuleUse]()},
	{"runtime_managed_component", "runtime_managed_components", "", shapeArray, false, []string{"architecture-components"}, ptr[model.RuntimeManagedComponent]()},
	{"runtime_server", "runtime_servers", "", shapeArray, false, []string{"apis-exposed"}, ptr[model.RuntimeServer]()},
	{"runtime_security_control", "runtime_security_controls", "", shapeArray, false, []string{"security"}, ptr[model.RuntimeSecurityControl]()},
	{"runtime_proxy_control", "runtime_proxy_controls", "", shapeArray, false, []string{"security", "network-architecture"}, ptr[model.RuntimeProxyControl]()},
	{"runtime_webhook_server", "runtime_webhook_servers", "", shapeArray, false, []string{"admission-webhooks"}, ptr[model.RuntimeWebhookServer]()},
	{"behavioral_evidence", "behavioral_evidence", "", shapeArray, true, []string{"security"}, ptr[model.BehavioralEvidence]()},
	{"access_policy", "access_policies", "", shapeArray, false, []string{"security"}, ptr[model.AccessPolicy]()},
	{"infrastructure_resource", "infrastructure_resources", "", shapeArray, false, []string{"architecture-components"}, ptr[model.InfrastructureResource]()},
	{"data_coverage", "data_coverage", "", shapeMap, false, []string{"architectural-analysis"}, ptr[string]()},
	{"category_coverage", "category_coverage", "", shapeMap, false, []string{"architectural-analysis"}, ptr[model.CategoryCoverage]()},
	{"cross_reference", "cross_references", "", shapeArray, false, []string{"integration-points"}, ptr[model.CrossReference]()},
	{"coverage_finding", "coverage_findings", "", shapeArray, false, []string{"architectural-analysis"}, ptr[model.CoverageFinding]()},
	{"synthesis_evidence", "synthesis_evidence", "", shapeMapArray, false, []string{"architectural-analysis"}, ptr[model.EvidenceRecord]()},
	{"cross_cutting_evidence", "cross_cutting_evidence", "", shapeMapArray, false, []string{"security"}, ptr[model.CrossCuttingEvidence]()},
	{"gap_evidence_candidate", "gap_evidence_index", "", shapeMapArray, false, []string{"architectural-analysis"}, ptr[model.GapEvidenceCandidate]()},
	{"context_contract", "context_contract", "", shapeSingleton, true, []string{"context-contract"}, ptr[model.ContextContract]()},
}

var metadataFields = map[string]bool{
	"component": true, "repo": true, "commit_sha": true, "extracted_at": true,
	"analyzer_version": true, "schema_version": true,
	"scan_statistics": true,
}

func descriptorByType(factType string) (factDescriptor, bool) {
	for _, descriptor := range factRegistry {
		if descriptor.Type == factType {
			return descriptor, true
		}
	}
	return factDescriptor{}, false
}

// FactTypes is the complete phase-one analyzer fact inventory.
func FactTypes() []string {
	result := make([]string, 0, len(factRegistry))
	for _, descriptor := range factRegistry {
		result = append(result, descriptor.Type)
	}
	return result
}
