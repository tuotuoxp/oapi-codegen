package codegen

import (
	"go/format"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oapi-codegen/oapi-codegen/v2/pkg/util"
)

func TestResolveParameterValidationSchemaMergesRefs(t *testing.T) {
	specPath := "test_specs/parameter-validation/spec.yaml"
	resolver, err := newParameterValidationResolver(specPath)
	require.NoError(t, err)
	require.NotNil(t, resolver)

	tests := []struct {
		name      string
		pathParts []string
		assertion func(t *testing.T, got parameterValidationSchema)
	}{
		{
			name:      "different constraints are merged",
			pathParts: []string{"paths", "/different", "get", "parameters", "0", "schema"},
			assertion: func(t *testing.T, got parameterValidationSchema) {
				require.NotNil(t, got.MinLength)
				require.NotNil(t, got.MaxLength)
				require.NotNil(t, got.Pattern)
				assert.Equal(t, uint64(1), *got.MinLength)
				assert.Equal(t, uint64(5), *got.MaxLength)
				assert.Equal(t, "^[a-z]+$", *got.Pattern)
			},
		},
		{
			name:      "local override wins same key",
			pathParts: []string{"paths", "/override", "get", "parameters", "0", "schema"},
			assertion: func(t *testing.T, got parameterValidationSchema) {
				require.NotNil(t, got.MinLength)
				require.NotNil(t, got.Pattern)
				assert.Equal(t, uint64(3), *got.MinLength)
				assert.Equal(t, "^[a-z]+$", *got.Pattern)
			},
		},
		{
			name:      "nearest schema wins in chain",
			pathParts: []string{"paths", "/chain", "get", "parameters", "0", "schema"},
			assertion: func(t *testing.T, got parameterValidationSchema) {
				require.NotNil(t, got.MinLength)
				require.NotNil(t, got.MaxLength)
				require.NotNil(t, got.Pattern)
				assert.Equal(t, uint64(3), *got.MinLength)
				assert.Equal(t, uint64(8), *got.MaxLength)
				assert.Equal(t, "^[a-z]+$", *got.Pattern)
				assert.True(t, got.HasEnum)
				assert.Equal(t, []any{"foo", "bar"}, got.Enum)
			},
		},
		{
			name:      "external integer schema resolves",
			pathParts: []string{"paths", "/numeric/{id}", "get", "parameters", "0", "schema"},
			assertion: func(t *testing.T, got parameterValidationSchema) {
				require.NotNil(t, got.Type)
				require.NotNil(t, got.Minimum)
				require.NotNil(t, got.Maximum)
				require.NotNil(t, got.ExclusiveMinimum)
				assert.Equal(t, openapi3.TypeInteger, *got.Type)
				assert.Equal(t, 2.0, *got.Minimum)
				assert.Equal(t, 6.0, *got.Maximum)
				assert.True(t, *got.ExclusiveMinimum)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := resolver.loadDocument(resolver.rootPath)
			require.NoError(t, err)
			node, err := followYAMLPath(doc, tt.pathParts...)
			require.NoError(t, err)
			got, err := resolver.resolveSchema(resolver.rootPath, node, map[string]bool{})
			require.NoError(t, err)
			tt.assertion(t, got)
		})
	}
}

func TestResolveParameterValidationPlanForParameterRefAndCycle(t *testing.T) {
	specPath := "test_specs/parameter-validation/spec.yaml"
	swagger, err := util.LoadSwagger(specPath)
	require.NoError(t, err)

	opts := Configuration{
		PackageName: "api",
		Generate: GenerateOptions{
			ChiServer: true,
		},
		InputSpec: specPath,
	}

	globalState.options = opts
	globalState.spec = swagger

	refParam := swagger.Paths.Value("/ref-param").Get.Parameters[0]
	params, err := DescribeParameters(openapi3.Parameters{refParam}, []string{"GetRefParamParams"}, nil)
	require.NoError(t, err)
	require.Len(t, params, 1)
	assert.True(t, params[0].Validation.HasValidation())
	assert.Equal(t, "^[A-Z]+$", params[0].Validation.Pattern)

	cycleSpecPath := "test_specs/parameter-validation/cycle-spec.yaml"
	cycleSwagger, err := util.LoadSwagger(cycleSpecPath)
	require.NoError(t, err)
	globalState.options.InputSpec = cycleSpecPath
	globalState.spec = cycleSwagger
	cycleParam := cycleSwagger.Paths.Value("/cycle").Get.Parameters[0]
	_, err = DescribeParameters(openapi3.Parameters{cycleParam}, []string{"GetCycleParams"}, []string{"paths", "/cycle", "get", "parameters"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reference cycle")
}

func TestGenerateServerParameterValidationCode(t *testing.T) {
	specPath := "test_specs/parameter-validation/spec.yaml"
	swagger, err := util.LoadSwagger(specPath)
	require.NoError(t, err)

	opts := Configuration{
		PackageName: "api",
		Generate: GenerateOptions{
			ChiServer: true,
		},
		InputSpec: specPath,
	}

	code, err := Generate(swagger, opts)
	require.NoError(t, err)
	require.NotEmpty(t, code)

	_, err = format.Source([]byte(code))
	require.NoError(t, err)

	assert.Contains(t, code, `validateParamString("q", string(params.Q), 1, true, 5, true, "^[a-z]+$", true, nil, false)`)
	assert.Contains(t, code, `validateParamString("q", string(params.Q), 3, true, 8, true, "^[a-z]+$", true, []string{"foo", "bar"}, true)`)
	assert.Contains(t, code, `validateParamNumber("id", float64(id), 2.000000, true, true, 6.000000, true, false, []float64{3, 5}, true)`)
	assert.Contains(t, code, `validateParamCustomType("code", r.URL.Query().Get("code"), &params.Code)`)
	assert.NotContains(t, code, `validateParamString("code"`)
	assert.NotContains(t, code, `validateParamString("array_param"`)
	assert.NotContains(t, code, `validateParamString("one_of_param"`)
}

func TestGenerateServerParameterValidationWithoutInputSpecFallsBack(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	specFile := filepath.Join("test_specs", "parameter-validation", "spec.yaml")
	swagger, err := loader.LoadFromFile(specFile)
	require.NoError(t, err)

	opts := Configuration{
		PackageName: "api",
		Generate: GenerateOptions{
			ChiServer: true,
		},
	}

	code, err := Generate(swagger, opts)
	require.NoError(t, err)
	assert.True(t, strings.Contains(code, "validateParamString") || strings.Contains(code, "validateParamNumber"))
}

func TestBuildParameterValidationPlanSkipsUnsupportedSchemas(t *testing.T) {
	tests := []parameterValidationSchema{
		{HasAllOf: true},
		{HasAnyOf: true},
		{HasOneOf: true},
		{HasNot: true},
		{Type: ptrString(openapi3.TypeArray)},
		{Type: ptrString(openapi3.TypeObject)},
		{Type: ptrString(openapi3.TypeString), XGoType: ptrString("CustomCode")},
	}

	for _, schema := range tests {
		plan, err := buildParameterValidationPlan(schema)
		require.NoError(t, err)
		assert.False(t, plan.HasValidation())
	}
}

func ptrString(v string) *string {
	return &v
}
