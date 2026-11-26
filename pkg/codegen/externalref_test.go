package codegen

import "testing"

func TestEnsureExternalRefsInSchemaQualifiesNamedTypes(t *testing.T) {
	previousImportMapping := globalState.importMapping
	globalState.importMapping = importMap{
		"external.yaml": {Name: "external"},
	}
	t.Cleanup(func() {
		globalState.importMapping = previousImportMapping
	})

	tests := []struct {
		name        string
		goType      string
		wantRefType string
		wantUnion   string
	}{
		{
			name:        "named type",
			goType:      "ExternalType",
			wantRefType: "external.ExternalType",
			wantUnion:   "external.ExternalType",
		},
		{
			name:      "built-in type",
			goType:    "string",
			wantUnion: "string",
		},
		{
			name:      "container type",
			goType:    "[]string",
			wantUnion: "[]string",
		},
		{
			name:      "already-qualified type",
			goType:    "sdkopenapi.ExternalType",
			wantUnion: "sdkopenapi.ExternalType",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := Schema{
				GoType:        tt.goType,
				UnionElements: []UnionElement{UnionElement(tt.goType)},
			}

			ensureExternalRefsInSchema(&schema, "external.yaml#/components/responses/External")

			if schema.RefType != tt.wantRefType {
				t.Errorf("RefType = %q, want %q", schema.RefType, tt.wantRefType)
			}
			if got := string(schema.UnionElements[0]); got != tt.wantUnion {
				t.Errorf("UnionElements[0] = %q, want %q", got, tt.wantUnion)
			}
		})
	}
}
