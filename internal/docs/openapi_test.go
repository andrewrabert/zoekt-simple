package docs

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateOpenAPIValidYAML(t *testing.T) {
	spec := DefaultSpec()
	data := GenerateOpenAPI(spec)

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated spec is not valid YAML: %v\n\n%s", err, string(data))
	}

	if parsed["openapi"] != "3.1.0" {
		t.Errorf("expected openapi version 3.1.0, got %v", parsed["openapi"])
	}

	info, ok := parsed["info"].(map[string]any)
	if !ok {
		t.Fatal("missing info section")
	}
	if info["title"] != "zoekt-server API" {
		t.Errorf("unexpected title: %v", info["title"])
	}
}

func TestGenerateOpenAPIContainsAllEndpoints(t *testing.T) {
	spec := DefaultSpec()
	data := GenerateOpenAPI(spec)

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated spec is not valid YAML: %v", err)
	}

	paths, ok := parsed["paths"].(map[string]any)
	if !ok {
		t.Fatal("missing paths section")
	}

	expectedPaths := []string{
		"/healthz",
		"/api/search",
		"/api/list",
		"/api/file",
		"/api/reindex",
		"/api/reindex/{taskID}",
		"/mcp",
	}
	for _, p := range expectedPaths {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path: %s", p)
		}
	}
}

func TestGenerateOpenAPIContainsComponents(t *testing.T) {
	spec := DefaultSpec()
	data := GenerateOpenAPI(spec)

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generated spec is not valid YAML: %v", err)
	}

	components, ok := parsed["components"].(map[string]any)
	if !ok {
		t.Fatal("missing components section")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("missing components/schemas section")
	}

	expectedSchemas := []string{
		"Error",
		"SearchOptions",
		"SearchResult",
		"Stats",
		"FileMatch",
		"LineMatch",
		"LineFragment",
		"SymbolInfo",
		"RepoList",
		"RepoListEntry",
		"Repository",
		"RepoStats",
		"ReindexTask",
	}
	for _, s := range expectedSchemas {
		if _, ok := schemas[s]; !ok {
			t.Errorf("missing component schema: %s", s)
		}
	}
}

func TestGenerateOpenAPIContainsTags(t *testing.T) {
	spec := DefaultSpec()
	data := GenerateOpenAPI(spec)

	content := string(data)
	expectedTags := []string{"Search", "Files", "Reindex", "MCP", "System"}
	for _, tag := range expectedTags {
		if !strings.Contains(content, "name: "+tag) {
			t.Errorf("missing tag: %s", tag)
		}
	}
}

func TestGeneratedSpecIsServedByHandler(t *testing.T) {
	// Verify that generatedSpec() returns non-empty bytes and is valid YAML.
	data := generatedSpec()
	if len(data) == 0 {
		t.Fatal("generatedSpec() returned empty bytes")
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("generatedSpec() is not valid YAML: %v", err)
	}
}
