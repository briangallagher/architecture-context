package model

type Document struct {
	Component              string                            `json:"component"`
	Metadata               Metadata                          `json:"metadata"`
	RepoLineage            []RepoLineageRow                  `json:"repo_lineage"`
	Purpose                string                            `json:"purpose"`
	ArchitectureComponents []ArchitectureComponent           `json:"architecture_components"`
	CRDs                   []CRDRow                          `json:"crds"`
	ServingRuntimes        []ServingRuntimeRow               `json:"serving_runtimes"`
	HTTPEndpoints          []HTTPEndpointRow                 `json:"http_endpoints"`
	GRPCServices           []GRPCServiceRow                  `json:"grpc_services"`
	ExternalDependencies   []ExternalDependencyRow           `json:"external_dependencies"`
	InternalDependencies   []InternalDependencyRow           `json:"internal_dependencies"`
	Services               []ServiceRow                      `json:"services"`
	Ingress                []IngressRow                      `json:"ingress"`
	Egress                 []EgressRow                       `json:"egress"`
	ClusterRoles           []ClusterRoleRow                  `json:"cluster_roles"`
	RoleBindings           []RoleBindingRow                  `json:"role_bindings"`
	Secrets                []SecretRow                       `json:"secrets"`
	Authentication         []AuthenticationRow               `json:"authentication"`
	SecurityEvidence       []SecurityEvidence                `json:"security_evidence"`
	BehavioralEvidence     []BehavioralEvidence              `json:"behavioral_evidence"`
	Webhooks               []WebhookRow                      `json:"webhooks"`
	IntegrationPoints      []IntegrationPointRow             `json:"integration_points"`
	RecentChanges          []RecentChange                    `json:"recent_changes"`
	Sources                []SourceRow                       `json:"sources"`
	DataCoverage           map[string]string                 `json:"data_coverage"`
	CategoryCoverage       map[string]CategoryCoverage       `json:"category_coverage"`
	CrossReferences        []CrossReference                  `json:"cross_references"`
	CoverageFindings       []CoverageFinding                 `json:"coverage_findings"`
	SynthesisEvidence      map[string][]EvidenceRecord       `json:"synthesis_evidence"`
	CrossCuttingEvidence   map[string][]CrossCuttingEvidence `json:"cross_cutting_evidence"`
	Contract               *ContextContract                  `json:"context_contract,omitempty"`
	StructuredSections     []StructuredSection               `json:"-"`
}

type Metadata struct {
	Repository     string `json:"repository"`
	Version        string `json:"version"`
	Distribution   string `json:"distribution"`
	Languages      string `json:"languages"`
	DeploymentType string `json:"deployment_type"`
	GeneratedBy    string `json:"generated_by"`
}

type ArchitectureComponent struct {
	Component string `json:"component"`
	Type      string `json:"type"`
	Purpose   string `json:"purpose"`
}

type CRDRow struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	Scope   string `json:"scope"`
	APIRole string `json:"api_role"`
	Purpose string `json:"purpose"`
}

type ServingRuntimeRow struct {
	Name                  string `json:"name"`
	Kind                  string `json:"kind"`
	APIGroup              string `json:"api_group"`
	Version               string `json:"version"`
	Scope                 string `json:"scope"`
	SupportedModelFormats string `json:"supported_model_formats"`
	ContainerImages       string `json:"container_images"`
	BuiltInAdapter        string `json:"built_in_adapter"`
	Source                string `json:"source"`
}

type HTTPEndpointRow struct {
	Path       string `json:"path"`
	Method     string `json:"method"`
	Port       string `json:"port"`
	Protocol   string `json:"protocol"`
	Transport  string `json:"transport"`
	Encryption string `json:"encryption"`
	Auth       string `json:"auth"`
	Owner      string `json:"owner"`
	Purpose    string `json:"purpose"`
}

type GRPCServiceRow struct {
	Service    string `json:"service"`
	Port       string `json:"port"`
	Protocol   string `json:"protocol"`
	Transport  string `json:"transport"`
	Encryption string `json:"encryption"`
	Auth       string `json:"auth"`
	Owner      string `json:"owner"`
	Purpose    string `json:"purpose"`
}

type ExternalDependencyRow struct {
	Component string `json:"component"`
	Version   string `json:"version"`
	Required  string `json:"required"`
	Role      string `json:"role"`
	Purpose   string `json:"purpose"`
}

type InternalDependencyRow struct {
	Component       string `json:"component"`
	InteractionType string `json:"interaction_type"`
	Role            string `json:"role"`
	Purpose         string `json:"purpose"`
}

type ServiceRow struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Port       string `json:"port"`
	TargetPort string `json:"target_port"`
	Protocol   string `json:"protocol"`
	Encryption string `json:"encryption"`
	Auth       string `json:"auth"`
	Exposure   string `json:"exposure"`
}

type IngressRow struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Hosts      string `json:"hosts"`
	Port       string `json:"port"`
	Protocol   string `json:"protocol"`
	Encryption string `json:"encryption"`
	TLSMode    string `json:"tls_mode"`
	Exposure   string `json:"exposure"`
}

type EgressRow struct {
	Destination string `json:"destination"`
	Port        string `json:"port"`
	Protocol    string `json:"protocol"`
	Encryption  string `json:"encryption"`
	Auth        string `json:"auth"`
	Purpose     string `json:"purpose"`
}

type ClusterRoleRow struct {
	Name            string `json:"name"`
	APIGroup        string `json:"api_group"`
	Resources       string `json:"resources"`
	NonResourceURLs string `json:"non_resource_urls"`
	Verbs           string `json:"verbs"`
}

type RoleBindingRow struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Role           string `json:"role"`
	ServiceAccount string `json:"service_account"`
}

type SecretRow struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Purpose       string `json:"purpose"`
	ProvisionedBy string `json:"provisioned_by"`
	AutoRotate    string `json:"auto_rotate"`
}

type AuthenticationRow struct {
	Endpoint         string `json:"endpoint"`
	Methods          string `json:"methods"`
	Mechanism        string `json:"mechanism"`
	EnforcementPoint string `json:"enforcement_point"`
	Policy           string `json:"policy"`
}

type WebhookRow struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Path          string `json:"path"`
	Port          string `json:"port"`
	FailurePolicy string `json:"failure_policy"`
	Resources     string `json:"resources"`
	Operations    string `json:"operations"`
	Purpose       string `json:"purpose"`
}

type IntegrationPointRow struct {
	Component       string `json:"component"`
	InteractionType string `json:"interaction_type"`
	Role            string `json:"role"`
	Port            string `json:"port"`
	Protocol        string `json:"protocol"`
	Encryption      string `json:"encryption"`
	Purpose         string `json:"purpose"`
}

type RepoLineageRow struct {
	Role            string `json:"role"`
	Repository      string `json:"repository"`
	SyncMechanism   string `json:"sync_mechanism"`
	SyncBranch      string `json:"sync_branch"`
	SyncWorkflows   string `json:"sync_workflows"`
	DetectionMethod string `json:"detection_method"`
}

type SourceRow struct {
	File     string `json:"file"`
	Lines    string `json:"lines"`
	Sections string `json:"sections"`
}

// StructuredSection is schema-constrained content whose heading and placement
// are owned by the renderer's section registry. It intentionally cannot carry a
// user-supplied heading or raw Markdown block.
type StructuredSection struct {
	ID          string                   `json:"id"`
	Status      string                   `json:"status"`
	Authority   StructuredAuthority      `json:"authority"`
	Blocks      []StructuredContentBlock `json:"blocks"`
	Evidence    []StructuredEvidenceRef  `json:"evidence"`
	Uncertainty string                   `json:"uncertainty,omitempty"`
}

type StructuredContentBlock struct {
	Type    string               `json:"type"`
	Text    string               `json:"text,omitempty"`
	Items   []string             `json:"items,omitempty"`
	TableID string               `json:"table_id,omitempty"`
	Rows    []StructuredTableRow `json:"rows,omitempty"`
}

type StructuredTableRow struct {
	Cells    []string                `json:"cells"`
	Evidence []StructuredEvidenceRef `json:"evidence"`
}

type StructuredAuthority struct {
	Kind       string `json:"kind"`
	Origin     string `json:"origin"`
	ClaimClass string `json:"claim_class"`
}

type StructuredEvidenceRef struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Revision  string `json:"revision"`
}
