package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPreprocessSwaggerIncludes(t *testing.T) {
	t.Run("include replaces object", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml": "some_obj: !include ./obj.yaml\n",
			"obj.yaml":  "type: object\nname: example\n",
		}, "spec.yaml")

		var got map[string]any
		require.NoError(t, preprocessToValue(entryPath, &got))
		require.Equal(t, map[string]any{
			"some_obj": map[string]any{
				"type": "object",
				"name": "example",
			},
		}, got)
	})

	t.Run("include replaces array", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml": "some_array: !include ./arr.yaml\n",
			"arr.yaml":  "- 111\n- 222\n",
		}, "spec.yaml")

		var got map[string]any
		require.NoError(t, preprocessToValue(entryPath, &got))
		require.Equal(t, map[string]any{
			"some_array": []any{111, 222},
		}, got)
	})

	t.Run("object merge with single include", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml": "some_obj:\n  prop_a: local\n  <<: !include ./base.yaml\n  prop_b: yyy\n",
			"base.yaml": "prop_a: from_include\nprop_c: c\n",
		}, "spec.yaml")

		var got map[string]any
		require.NoError(t, preprocessToValue(entryPath, &got))
		require.Equal(t, map[string]any{
			"some_obj": map[string]any{
				"prop_a": "local",
				"prop_b": "yyy",
				"prop_c": "c",
			},
		}, got)
	})

	t.Run("object merge include precedence", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml": "some_obj:\n  local: local_value\n  <<: !include ./aaa.yaml\n  <<: !include ./bbb.yaml\n  z: local_z\n",
			"aaa.yaml":  "a: 1\nshared: from_aaa\nz: from_aaa\n",
			"bbb.yaml":  "b: 2\nshared: from_bbb\n",
		}, "spec.yaml")

		var got map[string]any
		require.NoError(t, preprocessToValue(entryPath, &got))
		require.Equal(t, map[string]any{
			"some_obj": map[string]any{
				"a":      1,
				"b":      2,
				"shared": "from_bbb",
				"local":  "local_value",
				"z":      "local_z",
			},
		}, got)
	})

	t.Run("array include_array flattens", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml":  "some_array:\n  - val1\n  - !include_array ./aaa.yaml\n  - val2\n",
			"aaa.yaml":   "- 111\n- 222\n",
			"unused.yml": "noop: true\n",
		}, "spec.yaml")

		var got map[string]any
		require.NoError(t, preprocessToValue(entryPath, &got))
		require.Equal(t, map[string]any{
			"some_array": []any{"val1", 111, 222, "val2"},
		}, got)
	})

	t.Run("array include does not flatten", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml": "some_array:\n  - val1\n  - !include ./aaa.yaml\n  - val2\n",
			"aaa.yaml":  "- 111\n- 222\n",
		}, "spec.yaml")

		var got map[string]any
		require.NoError(t, preprocessToValue(entryPath, &got))
		require.Equal(t, map[string]any{
			"some_array": []any{"val1", []any{111, 222}, "val2"},
		}, got)
	})

	t.Run("include_array type mismatch", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml": "some_array:\n  - !include_array ./aaa.yaml\n",
			"aaa.yaml":  "not_an_array: true\n",
		}, "spec.yaml")

		_, err := preprocessSwaggerIncludes(entryPath)
		require.Error(t, err)
		require.ErrorContains(t, err, "!include_array")
		require.ErrorContains(t, err, "must be an array")
	})

	t.Run("merge include type mismatch", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml": "some_obj:\n  <<: !include ./aaa.yaml\n",
			"aaa.yaml":  "- 111\n- 222\n",
		}, "spec.yaml")

		_, err := preprocessSwaggerIncludes(entryPath)
		require.Error(t, err)
		require.ErrorContains(t, err, "merge include")
		require.ErrorContains(t, err, "object/map")
	})

	t.Run("include cycle detection", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml": "a: !include ./a.yaml\n",
			"a.yaml":    "b: !include ./b.yaml\n",
			"b.yaml":    "a: !include ./a.yaml\n",
		}, "spec.yaml")

		_, err := preprocessSwaggerIncludes(entryPath)
		require.Error(t, err)
		require.ErrorContains(t, err, "include cycle detected")
		require.ErrorContains(t, err, "a.yaml")
		require.ErrorContains(t, err, "b.yaml")
	})

	t.Run("nested relative path resolution", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml":          "root: !include ./sub/first.yaml\n",
			"sub/first.yaml":     "value: !include ../common/second.yaml\n",
			"common/second.yaml": "name: nested\n",
		}, "spec.yaml")

		var got map[string]any
		require.NoError(t, preprocessToValue(entryPath, &got))
		require.Equal(t, map[string]any{
			"root": map[string]any{
				"value": map[string]any{
					"name": "nested",
				},
			},
		}, got)
	})

	t.Run("missing include file returns context", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml": "root: !include ./does-not-exist.yaml\n",
		}, "spec.yaml")

		_, err := preprocessSwaggerIncludes(entryPath)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to include")
		require.ErrorContains(t, err, "does-not-exist.yaml")
	})

	t.Run("include path traversal outside root is blocked", func(t *testing.T) {
		fixtureRoot := t.TempDir()
		entryPath := writeFixtureFilesInDir(t, fixtureRoot, map[string]string{
			"spec.yaml": "root: !include ../outside.yaml\n",
		}, "spec.yaml")
		writeFixtureFilesInDir(t, filepath.Dir(fixtureRoot), map[string]string{
			"outside.yaml": "secret: true\n",
		}, "outside.yaml")

		_, err := preprocessSwaggerIncludes(entryPath)
		require.Error(t, err)
		require.ErrorContains(t, err, "outside specification root")
	})
}

func TestLoadSwaggerWithIncludeAndRef(t *testing.T) {
	t.Run("included-file-relative-ref-resolves-from-included-file", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml":                    "openapi: 3.0.0\ninfo:\n  title: include spec\n  version: 1.0.0\npaths: !include ./some_dir/paths.yaml\n",
			"some_dir/paths.yaml":          "/pets:\n  get:\n    operationId: listPets\n    responses:\n      '200':\n        description: ok\n        content:\n          application/json:\n            schema:\n              $ref: ./components/pet.yaml\n",
			"some_dir/components/pet.yaml": "type: object\nrequired:\n  - fromIncluded\nproperties:\n  fromIncluded:\n    type: string\n",
		}, "spec.yaml")

		swagger, err := LoadSwagger(entryPath)
		require.NoError(t, err)

		petSchema := requireResponseSchema(t, swagger, "/pets")
		require.Contains(t, petSchema.Value.Properties, "fromIncluded")
	})

	t.Run("guard against entry-file-based misresolution", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml":                    "openapi: 3.0.0\ninfo:\n  title: include spec\n  version: 1.0.0\npaths: !include ./some_dir/paths.yaml\n",
			"components/pet.yaml":          "type: object\nproperties:\n  fromEntry:\n    type: string\n",
			"some_dir/paths.yaml":          "/pets:\n  get:\n    operationId: listPets\n    responses:\n      '200':\n        description: ok\n        content:\n          application/json:\n            schema:\n              $ref: ./components/pet.yaml\n",
			"some_dir/components/pet.yaml": "type: object\nproperties:\n  fromIncluded:\n    type: string\n",
		}, "spec.yaml")

		swagger, err := LoadSwagger(entryPath)
		require.NoError(t, err)

		petSchema := requireResponseSchema(t, swagger, "/pets")
		require.Contains(t, petSchema.Value.Properties, "fromIncluded")
		require.NotContains(t, petSchema.Value.Properties, "fromEntry")
	})

	t.Run("nested include + ref", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml":                         "openapi: 3.0.0\ninfo:\n  title: include spec\n  version: 1.0.0\npaths: !include ./some_dir/paths.yaml\n",
			"some_dir/paths.yaml":               "/pets: !include ./operations/get-pets.yaml\n",
			"some_dir/operations/get-pets.yaml": "get:\n  operationId: listPets\n  responses:\n    '200':\n      description: ok\n      content:\n        application/json:\n          schema:\n            $ref: ../components/pet.yaml\n",
			"some_dir/components/pet.yaml":      "type: object\nproperties:\n  fromNestedInclude:\n    type: string\n",
		}, "spec.yaml")

		swagger, err := LoadSwagger(entryPath)
		require.NoError(t, err)

		petSchema := requireResponseSchema(t, swagger, "/pets")
		require.Contains(t, petSchema.Value.Properties, "fromNestedInclude")
	})

	t.Run("included paths still resolve local component refs", func(t *testing.T) {
		entryPath := writeFixtureFiles(t, map[string]string{
			"spec.yaml":    "openapi: 3.0.0\ninfo:\n  title: include spec\n  version: 1.0.0\npaths: !include ./paths.yaml\ncomponents:\n  schemas: !include ./schemas.yaml\n",
			"paths.yaml":   "/pets:\n  get:\n    operationId: listPets\n    responses:\n      '200':\n        description: ok\n        content:\n          application/json:\n            schema:\n              $ref: '#/components/schemas/Pet'\n",
			"schemas.yaml": "Pet:\n  type: object\n  required:\n    - id\n  properties:\n    id:\n      type: string\n",
		}, "spec.yaml")

		swagger, err := LoadSwagger(entryPath)
		require.NoError(t, err)

		require.Contains(t, swagger.Components.Schemas, "Pet")
		petSchema := swagger.Components.Schemas["Pet"]
		require.NotNil(t, petSchema.Value)
		require.Equal(t, []string{"object"}, petSchema.Value.Type.Slice())

		responseSchema := requireResponseSchema(t, swagger, "/pets")
		require.Equal(t, "#/components/schemas/Pet", responseSchema.Ref)
	})
}

func requireResponseSchema(t *testing.T, swagger *openapi3.T, path string) *openapi3.SchemaRef {
	t.Helper()

	item := swagger.Paths.Find(path)
	require.NotNil(t, item)
	require.NotNil(t, item.Get)
	response := item.Get.Responses.Map()["200"]
	require.NotNil(t, response)
	content := response.Value.Content.Get("application/json")
	require.NotNil(t, content)
	require.NotNil(t, content.Schema)
	require.NotNil(t, content.Schema.Value)
	return content.Schema
}

func preprocessToValue(entryPath string, out any) error {
	specData, err := preprocessSwaggerIncludes(entryPath)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(specData, out)
}

func writeFixtureFiles(t *testing.T, files map[string]string, entryFile string) string {
	t.Helper()
	return writeFixtureFilesInDir(t, t.TempDir(), files, entryFile)
}

func writeFixtureFilesInDir(t *testing.T, dir string, files map[string]string, entryFile string) string {
	t.Helper()

	for relativePath, content := range files {
		absolutePath := filepath.Join(dir, relativePath)
		require.NoError(t, os.MkdirAll(filepath.Dir(absolutePath), 0o755))
		require.NoError(t, os.WriteFile(absolutePath, []byte(content), 0o644))
	}

	return filepath.Join(dir, entryFile)
}
