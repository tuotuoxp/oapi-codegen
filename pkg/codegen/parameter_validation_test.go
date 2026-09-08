package codegen

import (
	"bytes"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	params, err = DescribeParameters(openapi3.Parameters{cycleParam}, []string{"GetCycleParams"}, []string{"paths", "/cycle", "get", "parameters"})
	require.NoError(t, err)
	require.Len(t, params, 1)
	assert.False(t, params[0].Validation.HasValidation())
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
	assert.Contains(t, code, `validateParamInteger("id", id, "2", true, true, "6", true, false, []string{"3", "5"}, true)`)
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

func TestResolveParameterValidationIgnoresSequenceIndexesAfterIncludeExpansion(t *testing.T) {
	specPath := "test_specs/parameter-validation-include/spec.yaml"
	swagger, err := util.LoadSwagger(specPath)
	require.NoError(t, err)

	resolver, err := newParameterValidationResolver(specPath)
	require.NoError(t, err)
	require.NotNil(t, resolver)

	paramRef := findParameterRefByInAndName(t, swagger.Paths.Value("/included-header").Get.Parameters, openapi3.ParameterInHeader, "X-Header")
	plan, err := resolver.resolvePlan(paramRef, []string{"paths", "/included-header", "get", "parameters"}, 99)
	require.NoError(t, err)
	assert.True(t, plan.HasValidation())
	assert.Equal(t, openapi3.TypeString, plan.Kind)
	assert.Equal(t, uint64(3), plan.MinLength)
	assert.Equal(t, uint64(8), plan.MaxLength)
	assert.Equal(t, "^[A-Z]+$", plan.Pattern)
	assert.Equal(t, []string{"FOO", "BAR"}, plan.StringEnum)
}

func TestResolveParameterValidationFallsBackWhenSourceLookupMismatches(t *testing.T) {
	specPath := "test_specs/parameter-validation/spec.yaml"
	swagger, err := util.LoadSwagger(specPath)
	require.NoError(t, err)

	resolver, err := newParameterValidationResolver(specPath)
	require.NoError(t, err)
	require.NotNil(t, resolver)

	paramRef := swagger.Paths.Value("/different").Get.Parameters[0]
	stderr := captureStderr(t, func() {
		plan, err := resolver.resolvePlan(paramRef, []string{"paths", "/missing", "get", "parameters"}, 0)
		require.NoError(t, err)
		assert.True(t, plan.HasValidation())
		assert.Equal(t, uint64(1), plan.MinLength)
		assert.False(t, plan.HasMaxLength)
		assert.Equal(t, "^[a-z]+$", plan.Pattern)
	})
	assert.Contains(t, stderr, `Warning: failed to resolve original parameter validation source for "q" in "query"`)
}

func TestResolveParameterValidationUsesOriginalSourceWhenLocalPathExists(t *testing.T) {
	specPath := "test_specs/parameter-validation/spec.yaml"
	swagger, err := util.LoadSwagger(specPath)
	require.NoError(t, err)

	resolver, err := newParameterValidationResolver(specPath)
	require.NoError(t, err)
	require.NotNil(t, resolver)

	paramRef := swagger.Paths.Value("/different").Get.Parameters[0]
	stderr := captureStderr(t, func() {
		plan, err := resolver.resolvePlan(paramRef, []string{"paths", "/different", "get", "parameters"}, 0)
		require.NoError(t, err)
		assert.True(t, plan.HasValidation())
		assert.Equal(t, uint64(1), plan.MinLength)
		assert.True(t, plan.HasMaxLength)
		assert.Equal(t, uint64(5), plan.MaxLength)
		assert.Equal(t, "^[a-z]+$", plan.Pattern)
	})
	assert.NotContains(t, stderr, `Warning: failed to resolve original parameter validation source`)
}

func TestResolveParameterValidationDoesNotWarnForExternalParameterRef(t *testing.T) {
	specPath := "test_specs/parameter-validation/external-ref-main.yaml"
	swagger, err := util.LoadSwagger(specPath)
	require.NoError(t, err)

	resolver, err := newParameterValidationResolver(specPath)
	require.NoError(t, err)
	require.NotNil(t, resolver)

	paramRef := swagger.Paths.Value("/external-ref").Get.Parameters[0]
	stderr := captureStderr(t, func() {
		plan, err := resolver.resolvePlan(paramRef, []string{"paths", "/external-ref", "get", "parameters"}, 0)
		require.NoError(t, err)
		assert.True(t, plan.HasValidation())
		assert.Equal(t, uint64(2), plan.MinLength)
		assert.Equal(t, "^[a-z]+$", plan.Pattern)
	})
	assert.NotContains(t, stderr, `Warning: failed to resolve original parameter validation source`)
}

func TestResolveParameterValidationDoesNotWarnForExternalPathSourceFallback(t *testing.T) {
	specPath := "test_specs/parameter-validation/external-ref-main.yaml"
	swagger, err := util.LoadSwagger(specPath)
	require.NoError(t, err)

	resolver, err := newParameterValidationResolver(specPath)
	require.NoError(t, err)
	require.NotNil(t, resolver)

	paramRef := swagger.Paths.Value("/external-inline").Get.Parameters[0]
	stderr := captureStderr(t, func() {
		plan, err := resolver.resolvePlan(paramRef, []string{"paths", "/external-inline", "get", "parameters"}, 0)
		require.NoError(t, err)
		assert.True(t, plan.HasValidation())
		assert.Equal(t, uint64(3), plan.MinLength)
		assert.Equal(t, "^[A-Z]+$", plan.Pattern)
	})
	assert.NotContains(t, stderr, `Warning: failed to resolve original parameter validation source`)
}

func TestGenerateServerParameterValidationCodeWithIncludeArrayHeaderParameter(t *testing.T) {
	specPath := "test_specs/parameter-validation-include/spec.yaml"
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

	assert.Contains(t, code, `validateParamString("X-Header", string(XHeader), 3, true, 8, true, "^[A-Z]+$", true, []string{"FOO", "BAR"}, true)`)
}

func TestMixedFormatParameterRefsResolveTypeAndValidation(t *testing.T) {
	tests := []struct {
		name           string
		specPath       string
		operationPath  string
		paramName      string
		expectedGoType string
		assertion      func(t *testing.T, got ParameterDefinition)
	}{
		{
			name:           "yaml path to json chain string",
			specPath:       "test_specs/parameter-validation-mixed/openapi.yaml",
			operationPath:  "/cross-format-string",
			paramName:      "code",
			expectedGoType: "string",
			assertion: func(t *testing.T, got ParameterDefinition) {
				assert.Equal(t, openapi3.TypeString, got.Validation.Kind)
				assert.Equal(t, uint64(3), got.Validation.MinLength)
				assert.Equal(t, uint64(8), got.Validation.MaxLength)
				assert.Equal(t, "^[a-z]+$", got.Validation.Pattern)
			},
		},
		{
			name:           "yaml path to json chain integer",
			specPath:       "test_specs/parameter-validation-mixed/openapi.yaml",
			operationPath:  "/cross-format-int",
			paramName:      "count",
			expectedGoType: "int64",
			assertion: func(t *testing.T, got ParameterDefinition) {
				assert.Equal(t, openapi3.TypeInteger, got.Validation.Kind)
				assert.Equal(t, "2", got.Validation.IntegerMinimum)
				assert.Equal(t, "6", got.Validation.IntegerMaximum)
				assert.True(t, got.Validation.ExclusiveIntegerMinimum)
			},
		},
		{
			name:           "yaml yaml json matrix",
			specPath:       "test_specs/parameter-validation-mixed/openapi.yaml",
			operationPath:  "/yaml-yaml-json",
			paramName:      "nested",
			expectedGoType: "string",
			assertion: func(t *testing.T, got ParameterDefinition) {
				assert.Equal(t, openapi3.TypeString, got.Validation.Kind)
				assert.Equal(t, uint64(2), got.Validation.MinLength)
				assert.Equal(t, uint64(10), got.Validation.MaxLength)
				assert.Equal(t, "^[a-z0-9]+$", got.Validation.Pattern)
			},
		},
		{
			name:           "yaml json yaml matrix",
			specPath:       "test_specs/parameter-validation-mixed/openapi.yaml",
			operationPath:  "/yaml-json-yaml",
			paramName:      "mixed",
			expectedGoType: "string",
			assertion: func(t *testing.T, got ParameterDefinition) {
				assert.Equal(t, openapi3.TypeString, got.Validation.Kind)
				assert.Equal(t, uint64(4), got.Validation.MinLength)
				assert.Equal(t, uint64(9), got.Validation.MaxLength)
				assert.Equal(t, "^[A-Z]+$", got.Validation.Pattern)
			},
		},
		{
			name:           "json yaml json matrix",
			specPath:       "test_specs/parameter-validation-mixed/openapi.json",
			operationPath:  "/json-yaml-json",
			paramName:      "amount",
			expectedGoType: "int64",
			assertion: func(t *testing.T, got ParameterDefinition) {
				assert.Equal(t, openapi3.TypeInteger, got.Validation.Kind)
				assert.Equal(t, "7", got.Validation.IntegerMinimum)
				assert.Equal(t, "15", got.Validation.IntegerMaximum)
			},
		},
		{
			name:           "relative path segments",
			specPath:       "test_specs/parameter-validation-mixed/openapi.yaml",
			operationPath:  "/relative",
			paramName:      "rel",
			expectedGoType: "string",
			assertion: func(t *testing.T, got ParameterDefinition) {
				assert.Equal(t, openapi3.TypeString, got.Validation.Kind)
				assert.Equal(t, uint64(4), got.Validation.MinLength)
				assert.Equal(t, "^[0-9]+$", got.Validation.Pattern)
			},
		},
		{
			name:           "fragment pointer in external doc",
			specPath:       "test_specs/parameter-validation-mixed/openapi.yaml",
			operationPath:  "/fragment",
			paramName:      "token",
			expectedGoType: "string",
			assertion: func(t *testing.T, got ParameterDefinition) {
				assert.Equal(t, openapi3.TypeString, got.Validation.Kind)
				assert.Equal(t, uint64(5), got.Validation.MinLength)
				assert.Equal(t, uint64(12), got.Validation.MaxLength)
				assert.Equal(t, "^[A-Z]+$", got.Validation.Pattern)
			},
		},
		{
			name:           "single hop json regression",
			specPath:       "test_specs/parameter-validation-mixed/openapi.yaml",
			operationPath:  "/single-hop-json",
			paramName:      "directJson",
			expectedGoType: "string",
			assertion: func(t *testing.T, got ParameterDefinition) {
				assert.Equal(t, openapi3.TypeString, got.Validation.Kind)
				assert.Equal(t, uint64(3), got.Validation.MinLength)
				assert.Equal(t, uint64(8), got.Validation.MaxLength)
			},
		},
		{
			name:           "single hop yaml regression",
			specPath:       "test_specs/parameter-validation-mixed/openapi.yaml",
			operationPath:  "/single-hop-yaml",
			paramName:      "directYaml",
			expectedGoType: "string",
			assertion: func(t *testing.T, got ParameterDefinition) {
				assert.Equal(t, openapi3.TypeString, got.Validation.Kind)
				assert.Equal(t, uint64(1), got.Validation.MinLength)
				assert.Equal(t, "^[a-z]+$", got.Validation.Pattern)
			},
		},
		{
			name:           "cycle falls back gracefully",
			specPath:       "test_specs/parameter-validation-mixed/openapi.yaml",
			operationPath:  "/cycle",
			paramName:      "loop",
			expectedGoType: "interface{}",
			assertion: func(t *testing.T, got ParameterDefinition) {
				assert.False(t, got.Validation.HasValidation())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			swagger, err := util.LoadSwagger(tt.specPath)
			require.NoError(t, err)

			opts := Configuration{
				PackageName: "api",
				Generate: GenerateOptions{
					ChiServer: true,
					Models:    true,
				},
				InputSpec: tt.specPath,
			}
			globalState.options = opts
			globalState.spec = swagger

			op := swagger.Paths.Value(tt.operationPath).Get
			require.NotNil(t, op)
			params, err := DescribeParameters(op.Parameters, []string{SchemaNameToTypeName(op.OperationID) + "Params"}, []string{"paths", tt.operationPath, "get", "parameters"})
			require.NoError(t, err)

			var got *ParameterDefinition
			for i := range params {
				if params[i].ParamName == tt.paramName {
					got = &params[i]
					break
				}
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.expectedGoType, got.Schema.GoType)
			tt.assertion(t, *got)
		})
	}
}

func TestGenerateServerParameterValidationCodeAcrossMixedFormats(t *testing.T) {
	specPath := "test_specs/parameter-validation-mixed/openapi.yaml"
	swagger, err := util.LoadSwagger(specPath)
	require.NoError(t, err)

	opts := Configuration{
		PackageName: "api",
		Generate: GenerateOptions{
			ChiServer: true,
			Models:    true,
		},
		InputSpec: specPath,
	}

	code, err := Generate(swagger, opts)
	require.NoError(t, err)
	require.NotEmpty(t, code)

	_, err = format.Source([]byte(code))
	require.NoError(t, err)

	assert.Contains(t, code, "type GetCrossFormatStringParams struct {")
	assert.Regexp(t, regexp.MustCompile(`(?m)^\\s*Code\\s+string\\b`), code)
	assert.NotContains(t, code, "Code string `json:")

	assert.Contains(t, code, "type GetCrossFormatIntParams struct {")
	assert.Regexp(t, regexp.MustCompile(`(?m)^\\s*Count\\s+int64\\b`), code)
	assert.NotContains(t, code, "Count int64 `json:")

	assert.Contains(t, code, `validateParamString("code", string(params.Code), 3, true, 8, true, "^[a-z]+$", true, nil, false)`)
	assert.Contains(t, code, `validateParamInteger("count", int64(params.Count), "2", true, true, "6", true, false, nil, false)`)
	assert.Contains(t, code, `validateParamString("token", string(params.Token), 5, true, 12, true, "^[A-Z]+$", true, nil, false)`)
	assert.Contains(t, code, `validateParamString("rel", string(params.Rel), 4, true, 0, false, "^[0-9]+$", true, nil, false)`)

	assert.Contains(t, code, "type GetCycleParams struct {")
	assert.Regexp(t, regexp.MustCompile(`(?m)^\\s*Loop\\s+interface\\{\\}`), code)

	assert.Contains(t, code, "type CreateThingJSONBody struct {")
	assert.Contains(t, code, "Name string `json:\"name\"`")
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

func findParameterRefByInAndName(t *testing.T, params openapi3.Parameters, in string, name string) *openapi3.ParameterRef {
	t.Helper()

	for _, paramRef := range params {
		if paramRef != nil && paramRef.Value != nil && paramRef.Value.In == in && paramRef.Value.Name == name {
			return paramRef
		}
	}

	t.Fatalf("parameter %s in %s not found", name, in)
	return nil
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	originalStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stderr = w
	defer func() {
		os.Stderr = originalStderr
		_ = w.Close()
	}()

	done := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		_ = r.Close()
		done <- b.String()
	}()

	fn()

	_ = w.Close()
	os.Stderr = originalStderr

	return <-done
}
