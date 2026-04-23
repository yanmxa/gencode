package tool

import "testing"

func TestBuiltInToolSchemasAreOpenAICompatibleObjects(t *testing.T) {
	for _, schema := range GetToolSchemas() {
		params, ok := schema.Parameters.(map[string]any)
		if !ok {
			t.Fatalf("%s parameters must be a JSON schema object map", schema.Name)
		}
		if typ, _ := params["type"].(string); typ != "object" {
			t.Fatalf("%s parameters must declare top-level type object, got %v", schema.Name, params["type"])
		}
		for _, keyword := range []string{"oneOf", "anyOf", "allOf", "enum", "not"} {
			if _, exists := params[keyword]; exists {
				t.Fatalf("%s parameters must not use top-level %q", schema.Name, keyword)
			}
		}
	}
}
