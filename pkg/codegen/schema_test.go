package codegen

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oapi-codegen/oapi-codegen/v2/pkg/util"
)

func TestProperty_GoTypeDef(t *testing.T) {
	type fields struct {
		GlobalStateDisableRequiredReadOnlyAsPointer bool
		Schema                                      Schema
		Required                                    bool
		Nullable                                    bool
		ReadOnly                                    bool
		WriteOnly                                   bool
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			// When pointer is skipped by setting flag SkipOptionalPointer, the
			// flag will never be pointer irrespective of other flags.
			name: "Set skip optional pointer type for go type",
			fields: fields{
				Schema: Schema{
					SkipOptionalPointer: true,
					RefType:             "",
					GoType:              "int",
				},
			},
			want: "int",
		},

		{
			// if the field is optional, it will always be pointer irrespective of other
			// flags, given that pointer type is not skipped by setting SkipOptionalPointer
			// flag to true
			name: "When the field is optional",
			fields: fields{
				Schema: Schema{
					SkipOptionalPointer: false,
					RefType:             "",
					GoType:              "int",
				},
				Required: false,
			},
			want: "*int",
		},

		{
			// if the field(custom-type) is optional, it will NOT be a pointer if
			// SkipOptionalPointer flag is set to true
			name: "Set skip optional pointer type for ref type",
			fields: fields{
				Schema: Schema{
					SkipOptionalPointer: true,
					RefType:             "CustomType",
					GoType:              "int",
				},
				Required: false,
			},
			want: "CustomType",
		},

		// For the following test cases, SkipOptionalPointer flag is false.
		{
			name: "When field is required and not nullable",
			fields: fields{
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				Required: true,
				Nullable: false,
			},
			want: "int",
		},

		{
			name: "When field is required and nullable",
			fields: fields{
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				Required: true,
				Nullable: true,
			},
			want: "*int",
		},

		{
			name: "When field is optional and not nullable",
			fields: fields{
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				Required: false,
				Nullable: false,
			},
			want: "*int",
		},

		{
			name: "When field is optional and nullable",
			fields: fields{
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				Required: false,
				Nullable: true,
			},
			want: "*int",
		},

		// Following tests cases for non-nullable and required; and skip pointer is not opted
		{
			name: "When field is readOnly it will always be pointer",
			fields: fields{
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				ReadOnly: true,
				Required: true,
			},
			want: "*int",
		},

		{
			name: "When field is readOnly and read only pointer disabled",
			fields: fields{
				GlobalStateDisableRequiredReadOnlyAsPointer: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				ReadOnly: true,
				Required: true,
			},
			want: "int",
		},

		{
			name: "When field is readOnly and optional",
			fields: fields{
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				ReadOnly: true,
				Required: false,
			},
			want: "*int",
		},
		{
			name: "When field is readOnly and optional and read only pointer disabled",
			fields: fields{
				GlobalStateDisableRequiredReadOnlyAsPointer: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				ReadOnly: true,
				Required: false,
			},
			want: "*int",
		},

		// When field is write only, it will always be pointer unless pointer is
		// skipped by setting SkipOptionalPointer flag
		{
			name: "When field is write only and read only pointer disabled",
			fields: fields{
				GlobalStateDisableRequiredReadOnlyAsPointer: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				WriteOnly: true,
			},
			want: "*int",
		},

		{
			name: "When field is write only and read only pointer enabled",
			fields: fields{
				GlobalStateDisableRequiredReadOnlyAsPointer: false,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				WriteOnly: true,
			},
			want: "*int",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalState.options.Compatibility.DisableRequiredReadOnlyAsPointer = tt.fields.GlobalStateDisableRequiredReadOnlyAsPointer
			p := Property{
				Schema:    tt.fields.Schema,
				Required:  tt.fields.Required,
				Nullable:  tt.fields.Nullable,
				ReadOnly:  tt.fields.ReadOnly,
				WriteOnly: tt.fields.WriteOnly,
			}
			assert.Equal(t, tt.want, p.GoTypeDef())
		})
	}
}

func TestProperty_GoTypeDef_nullable(t *testing.T) {
	type fields struct {
		GlobalStateDisableRequiredReadOnlyAsPointer bool
		GlobalStateNullableType                     bool
		Schema                                      Schema
		Required                                    bool
		Nullable                                    bool
		ReadOnly                                    bool
		WriteOnly                                   bool
	}

	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			// Field not nullable.
			// When pointer is skipped by setting flag SkipOptionalPointer, the
			// flag will never be pointer irrespective of other flags.
			name: "Set skip optional pointer type for go type",
			fields: fields{
				GlobalStateNullableType: true,
				Schema: Schema{
					SkipOptionalPointer: true,
					RefType:             "",
					GoType:              "int",
				},
			},
			want: "int",
		},

		{
			// Field not nullable.
			// if the field is optional, it will always be pointer irrespective of other
			// flags, given that pointer type is not skipped by setting SkipOptionalPointer
			// flag to true
			name: "When the field is optional",
			fields: fields{
				GlobalStateNullableType: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					RefType:             "",
					GoType:              "int",
				},
				Required: false,
			},
			want: "*int",
		},

		{
			// Field not nullable.
			// if the field(custom type) is optional, it will NOT be a pointer if
			// SkipOptionalPointer flag is set to true
			name: "Set skip optional pointer type for ref type",
			fields: fields{
				GlobalStateNullableType: true,
				Schema: Schema{
					SkipOptionalPointer: true,
					RefType:             "CustomType",
					GoType:              "int",
				},
				Required: false,
			},
			want: "CustomType",
		},

		// Field not nullable.
		// For the following test case, SkipOptionalPointer flag is false.
		{
			name: "When field is required and not nullable",
			fields: fields{
				GlobalStateNullableType: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				Required: true,
				Nullable: false,
			},
			want: "int",
		},

		{
			name: "When field is required and nullable",
			fields: fields{
				GlobalStateNullableType: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				Required: true,
				Nullable: true,
			},
			want: "nullable.Nullable[int]",
		},

		{
			name: "When field is optional and not nullable",
			fields: fields{
				GlobalStateNullableType: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				Required: false,
				Nullable: false,
			},
			want: "*int",
		},

		{
			name: "When field is optional and nullable",
			fields: fields{
				GlobalStateNullableType: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				Required: false,
				Nullable: true,
			},
			want: "nullable.Nullable[int]",
		},

		{
			name: "When field is readOnly, non-nullable and required and skip pointer is not opted",
			fields: fields{
				GlobalStateNullableType: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				ReadOnly: true,
				Required: true,
			},
			want: "*int",
		},

		{
			name: "When field is readOnly, required, non-nullable and read only pointer disabled",
			fields: fields{
				GlobalStateNullableType:                     true,
				GlobalStateDisableRequiredReadOnlyAsPointer: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				ReadOnly: true,
				Required: true,
			},
			want: "int",
		},

		{
			name: "When field is readOnly, optional and non nullable",
			fields: fields{
				GlobalStateNullableType: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				ReadOnly: true,
				Required: false,
			},
			want: "*int",
		},
		{
			name: "When field is readOnly and optional and read only pointer disabled",
			fields: fields{
				GlobalStateNullableType:                     true,
				GlobalStateDisableRequiredReadOnlyAsPointer: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				ReadOnly: true,
				Required: false,
			},
			want: "*int",
		},

		{
			name: "When field is write only and non nullable",
			fields: fields{
				GlobalStateNullableType:                     true,
				GlobalStateDisableRequiredReadOnlyAsPointer: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				WriteOnly: true,
			},
			want: "*int",
		},

		{
			name: "When field is write only and nullable",
			fields: fields{
				GlobalStateNullableType:                     true,
				GlobalStateDisableRequiredReadOnlyAsPointer: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				WriteOnly: true,
				Nullable:  true,
			},
			want: "nullable.Nullable[int]",
		},

		{
			name: "When field is write only, nullable and read only pointer enabled",
			fields: fields{
				GlobalStateNullableType: true,
				Schema: Schema{
					SkipOptionalPointer: false,
					GoType:              "int",
				},
				WriteOnly: true,
				Nullable:  true,
			},
			want: "nullable.Nullable[int]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalState.options.Compatibility.DisableRequiredReadOnlyAsPointer = tt.fields.GlobalStateDisableRequiredReadOnlyAsPointer
			globalState.options.OutputOptions.NullableType = tt.fields.GlobalStateNullableType
			p := Property{
				Schema:    tt.fields.Schema,
				Required:  tt.fields.Required,
				Nullable:  tt.fields.Nullable,
				ReadOnly:  tt.fields.ReadOnly,
				WriteOnly: tt.fields.WriteOnly,
			}
			assert.Equal(t, tt.want, p.GoTypeDef())
		})
	}
}

func TestProperty_ZeroValueIsNil(t *testing.T) {
	newType := func(typ string) *openapi3.Types {
		return &openapi3.Types{typ}
	}

	tests := []struct {
		name        string
		oapiSchema  *openapi3.Schema
		goType      string
		expectIsNil bool
	}{
		{
			name:        "when an array, returns true",
			oapiSchema:  &openapi3.Schema{Type: newType("array")},
			expectIsNil: true,
		},
		{
			name:        "when an object, returns false",
			oapiSchema:  &openapi3.Schema{Type: newType("object")},
			expectIsNil: false,
		},
		{
			name:        "when an object rendered as a map, returns true",
			oapiSchema:  &openapi3.Schema{Type: newType("object")},
			goType:      "map[string]string",
			expectIsNil: true,
		},
		{
			name:        "when a string, returns false",
			oapiSchema:  &openapi3.Schema{Type: newType("string")},
			expectIsNil: false,
		},
		{
			name:        "when an integer, returns false",
			oapiSchema:  &openapi3.Schema{Type: newType("integer")},
			expectIsNil: false,
		},
		{
			name:        "when a number, returns false",
			oapiSchema:  &openapi3.Schema{Type: newType("number")},
			expectIsNil: false,
		},
		{
			name:        "when OAPISchema is nil, returns false",
			oapiSchema:  nil,
			expectIsNil: false,
		},
		{
			name:        "when OAPISchema is zero value, returns false",
			oapiSchema:  &openapi3.Schema{},
			expectIsNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prop := Property{
				Schema: Schema{
					OAPISchema: tt.oapiSchema,
					GoType:     tt.goType,
				},
			}
			if tt.expectIsNil {
				require.True(t, prop.ZeroValueIsNil())
			} else {
				require.False(t, prop.ZeroValueIsNil())
			}
		})
	}
}

func TestParamToGoTypeResolvesNestedSchemaRefs(t *testing.T) {
	specPath := "test_specs/nested-parameter-refs/spec.yaml"
	swagger, err := util.LoadSwagger(specPath)
	require.NoError(t, err)

	oldInputSpec := globalState.options.InputSpec
	t.Cleanup(func() {
		globalState.options.InputSpec = oldInputSpec
	})
	globalState.options.InputSpec = specPath

	params := swagger.Paths.Value("/items/{id}").Get.Parameters
	require.Len(t, params, 4)

	typeByName := map[string]string{}
	for _, p := range params {
		schema, err := paramToGoType(p.Value, []string{"GetItem", p.Value.Name})
		require.NoError(t, err)
		typeByName[p.Value.Name] = schema.GoType
	}

	assert.Equal(t, "int64", typeByName["id"])
	assert.Equal(t, "int64", typeByName["q"])
	assert.Equal(t, "string", typeByName["X-Trace"])
	assert.Equal(t, "interface{}", typeByName["passthrough"])
}

func TestResolveNestedParameterSchemaRef(t *testing.T) {
	specPath := "test_specs/recursive-parameter-refs/spec.yaml"
	swagger, err := util.LoadSwagger(specPath)
	require.NoError(t, err)

	oldInputSpec := globalState.options.InputSpec
	t.Cleanup(func() {
		globalState.options.InputSpec = oldInputSpec
	})
	globalState.options.InputSpec = specPath

	paramByPathAndName := func(path, name string) *openapi3.Parameter {
		t.Helper()
		for _, p := range swagger.Paths.Value(path).Get.Parameters {
			if p.Value != nil && p.Value.Name == name {
				return p.Value
			}
		}
		t.Fatalf("parameter %q not found for path %q", name, path)
		return nil
	}

	tests := []struct {
		name         string
		param        *openapi3.Parameter
		wantResolved bool
		wantGoType   string
		assertion    func(t *testing.T, sref *openapi3.SchemaRef)
	}{
		{
			name:         "two level local chain",
			param:        paramByPathAndName("/two-level/{id}", "id"),
			wantResolved: true,
			wantGoType:   "int64",
			assertion: func(t *testing.T, sref *openapi3.SchemaRef) {
				require.NotNil(t, sref.Value)
				require.NotNil(t, sref.Value.Type)
				assert.Equal(t, openapi3.TypeInteger, sref.Value.Type.Slice()[0])
				assert.Equal(t, "int64", sref.Value.Format)
			},
		},
		{
			name:         "three level local chain",
			param:        paramByPathAndName("/three-level", "q"),
			wantResolved: true,
			wantGoType:   "string",
			assertion: func(t *testing.T, sref *openapi3.SchemaRef) {
				require.NotNil(t, sref.Value)
				require.NotNil(t, sref.Value.Type)
				assert.Equal(t, openapi3.TypeString, sref.Value.Type.Slice()[0])
			},
		},
		{
			name:         "mixed chain preserves constraints",
			param:        paramByPathAndName("/mixed", "code"),
			wantResolved: true,
			wantGoType:   "string",
			assertion: func(t *testing.T, sref *openapi3.SchemaRef) {
				require.NotNil(t, sref.Value)
				require.NotNil(t, sref.Value.Type)
				assert.Equal(t, openapi3.TypeString, sref.Value.Type.Slice()[0])
				assert.Equal(t, uint64(3), sref.Value.MinLength)
				require.NotNil(t, sref.Value.MaxLength)
				assert.Equal(t, uint64(8), *sref.Value.MaxLength)
				assert.Equal(t, "^[a-z]+$", sref.Value.Pattern)
				assert.Equal(t, []any{"foo", "bar"}, sref.Value.Enum)
			},
		},
		{
			name:         "cycle falls back",
			param:        paramByPathAndName("/cycle", "loop"),
			wantResolved: false,
			wantGoType:   "interface{}",
		},
		{
			name:         "external multi level chain",
			param:        paramByPathAndName("/external/{externalId}", "externalId"),
			wantResolved: true,
			wantGoType:   "string",
			assertion: func(t *testing.T, sref *openapi3.SchemaRef) {
				require.NotNil(t, sref.Value)
				require.NotNil(t, sref.Value.Type)
				assert.Equal(t, openapi3.TypeString, sref.Value.Type.Slice()[0])
			},
		},
		{
			name:         "single level regression",
			param:        paramByPathAndName("/single-level", "trace"),
			wantResolved: true,
			wantGoType:   "string",
			assertion: func(t *testing.T, sref *openapi3.SchemaRef) {
				require.NotNil(t, sref.Value)
				require.NotNil(t, sref.Value.Type)
				assert.Equal(t, openapi3.TypeString, sref.Value.Type.Slice()[0])
			},
		},
		{
			name:         "untyped regression remains interface",
			param:        paramByPathAndName("/unknown", "passthrough"),
			wantResolved: false,
			wantGoType:   "interface{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, ok := resolveNestedParameterSchemaRef(tt.param.Schema)
			assert.Equal(t, tt.wantResolved, ok)
			if tt.assertion != nil {
				require.True(t, ok)
				tt.assertion(t, resolved)
			}

			got, err := paramToGoType(tt.param, []string{"Params", tt.param.Name})
			require.NoError(t, err)
			assert.Equal(t, tt.wantGoType, got.GoType)
		})
	}
}
