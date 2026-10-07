package markdown

import "testing"

func TestParseRBACRolesSupportsCurrentAndLegacyColumns(t *testing.T) {
	current := parseRBACRoles([]string{
		"### RBAC - Cluster Roles",
		"",
		"| Role Name | API Group | Resources | Non-Resource URLs | Verbs |",
		"|---|---|---|---|---|",
		"| metrics-reader | | | /metrics | get |",
	})
	if len(current) != 1 || current[0].NonResourceURLs != "/metrics" || current[0].Verbs != "get" {
		t.Fatalf("current RBAC row = %#v", current)
	}

	legacy := parseRBACRoles([]string{
		"### RBAC - Cluster Roles",
		"",
		"| Role Name | API Group | Resources | Verbs |",
		"|---|---|---|---|",
		"| metrics-reader | | | get |",
	})
	if len(legacy) != 1 || legacy[0].NonResourceURLs != "" || legacy[0].Verbs != "get" {
		t.Fatalf("legacy RBAC row = %#v", legacy)
	}
}
