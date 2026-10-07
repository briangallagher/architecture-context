package jsondata

import (
	"testing"
	"testing/fstest"
)

func TestParseComponentJSONMapsLegacyAnalyzerFields(t *testing.T) {
	fsys := fstest.MapFS{"component.json": {Data: []byte(`{
  "component": "example",
  "repo": "https://github.com/example/repo.git",
  "commit_sha": "abc123",
  "analyzer_version": "test/v1",
  "rbac": {
    "cluster_roles": [{"name":"reader","rules":[
      {"apiGroups":["apps"],"resources":["deployments"],"verbs":["get"]},
      {"nonResourceURLs":["/metrics"],"verbs":["get"]},
      {"apiGroups":["apps"],"resources":["deployments/status"],"nonResourceURLs":["/healthz"],"verbs":["get"]},
      {"verbs":["get"]}
    ]}],
    "kubebuilder_markers": [{"file":"controller.go","parsed":{"groups":["batch"],"resources":["jobs"],"nonResourceURLs":["/readyz"],"verbs":["list"]}}]
  },
  "services": [{"name":"api","type":"ClusterIP","ports":[{"port":8080,"targetPort":"http","protocol":"TCP"}]}],
  "controller_watches": [{"type":"Owns","gvk":"apps/v1.Deployment","controller":"Example","source":"controller.go:10"}],
  "webhooks": [{"name":"vexample","type":"validating","service_ref":"hook","path":"/validate","port":9443,"failure_policy":"Fail","side_effects":"None","rules":[{"apiGroups":["example.io"],"apiVersions":["v1"],"resources":["examples"],"operations":["CREATE"]}],"source":"webhook.yaml","purpose":"validate"}],
  "platform_webhooks": [{"component":"platform","webhook":"mutate"}],
  "external_webhooks": [{"component":"external","webhook":"check"}],
  "network_policies": [{"name":"default-deny","source":"netpol.yaml","pod_selector":{"app":"api"},"policy_types":["Ingress"]}],
  "http_endpoints": [{"method":"GET","path":"/healthz","source":"server.go"}],
  "dockerfiles": [{"path":"Dockerfile","base_image":"base:1","stages":2,"user":"1001","exposed_ports":[8080],"issues":["none"]}],
  "cross_cutting_evidence": {"fips":[{"claim":"system crypto","status":"literal","sources":["go.mod:2"]}]},
  "dependencies": {"internal_odh":[{"component":"gateway","interaction":"HTTP"}]}
}`)}}

	doc, err := ParseComponentJSON(fsys, "component.json")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "example" || doc.Repository != "https://github.com/example/repo.git" || doc.CommitSHA != "abc123" || doc.AnalyzerVersion != "test/v1" {
		t.Fatalf("identity fields = %#v", doc)
	}
	if len(doc.RBACRoles) != 5 {
		t.Fatalf("RBAC rules = %#v", doc.RBACRoles)
	}
	if got := doc.RBACRoles[3]; got.Resources != "deployments/status" || got.NonResourceURLs != "/healthz" {
		t.Fatalf("mixed RBAC rule = %#v", got)
	}
	if got := doc.RBACRoles[4]; got.Resources != "" || got.NonResourceURLs != "" || got.Verbs != "get" {
		t.Fatalf("legacy RBAC rule = %#v", got)
	}
	if got := doc.RBACRoles[0]; got.APIGroup != "batch" || got.NonResourceURLs != "/readyz" {
		t.Fatalf("kubebuilder RBAC rule = %#v", got)
	}
	if len(doc.Services) != 1 || doc.Services[0].TargetPort != "http" || len(doc.ControllerWatches) != 1 || len(doc.Webhooks) != 1 {
		t.Fatalf("service/watch/webhook mapping lost: %#v", doc)
	}
	if len(doc.PlatformWebhooks) != 1 || len(doc.ExternalWebhooks) != 1 || len(doc.NetworkPolicies) != 1 {
		t.Fatalf("reference/network mapping lost: %#v", doc)
	}
	if len(doc.Endpoints) != 1 || len(doc.Dockerfiles) != 1 || len(doc.InternalDeps) != 1 || len(doc.CrossCuttingEvidence["fips"]) != 1 {
		t.Fatalf("typed mapper fields lost: %#v", doc)
	}
}

func TestParseComponentJSONRejectsInvalidOrIdentitylessJSON(t *testing.T) {
	for name, raw := range map[string]string{"malformed": "{", "identityless": `{}`} {
		t.Run(name, func(t *testing.T) {
			fsys := fstest.MapFS{"component.json": {Data: []byte(raw)}}
			if _, err := ParseComponentJSON(fsys, "component.json"); err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}
