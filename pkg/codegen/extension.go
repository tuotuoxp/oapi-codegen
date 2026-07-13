package codegen

import (
	"fmt"
	"strings"
)

const (
	// extPropGoType overrides the generated type definition. When
	// resolve-type-name-collisions is enabled, the collision resolver
	// controls the final Go type name; this extension controls what
	// that name aliases or refers to.
	extPropGoType = "x-go-type"
	// extPropGoTypeSkipOptionalPointer specifies that optional fields should
	// be the type itself instead of a pointer to the type.
	extPropGoTypeSkipOptionalPointer = "x-go-type-skip-optional-pointer"
	// extPropGoImport specifies the module to import which provides above type
	extPropGoImport = "x-go-type-import"
	// extPropGoRef qualifies x-go-type with package import information, using
	// the same shape as go-jsonschema: {"path": "...", "alias": "..."}.
	// When present alongside x-go-type, x-go-ref takes precedence over
	// x-go-type-import for determining the import and type qualifier.
	extPropGoRef = "x-go-ref"
	// extGoName is used to override a field name
	extGoName = "x-go-name"
	// extGoTypeName overrides a generated typename. When
	// resolve-type-name-collisions is enabled, the collision resolver
	// controls the top-level Go type name; this extension controls
	// the name of the underlying type definition that gets aliased.
	extGoTypeName        = "x-go-type-name"
	extPropGoJsonIgnore  = "x-go-json-ignore"
	extPropOmitEmpty     = "x-omitempty"
	extPropOmitZero      = "x-omitzero"
	extPropExtraTags     = "x-oapi-codegen-extra-tags"
	extEnumVarNames      = "x-enum-varnames"
	extEnumNames         = "x-enumNames"
	extDeprecationReason = "x-deprecated-reason"
	extOrder             = "x-order"
	// extOapiCodegenOnlyHonourGoName is to be used to explicitly enforce the generation of a field as the `x-go-name` extension has describe it.
	// This is intended to be used alongside the `allow-unexported-struct-field-names` Compatibility option
	extOapiCodegenOnlyHonourGoName = "x-oapi-codegen-only-honour-go-name"
)

// goRef holds the parsed contents of an x-go-ref extension.
type goRef struct {
	Path  string // import path, e.g. "github.com/yourorg/pack"
	Alias string // optional package alias/name, e.g. "mypack"
}

func extString(extPropValue any) (string, error) {
	str, ok := extPropValue.(string)
	if !ok {
		return "", fmt.Errorf("failed to convert type: %T", extPropValue)
	}
	return str, nil
}

func extTypeName(extPropValue any) (string, error) {
	return extString(extPropValue)
}

func extParsePropGoTypeSkipOptionalPointer(extPropValue any) (bool, error) {
	goTypeSkipOptionalPointer, ok := extPropValue.(bool)
	if !ok {
		return false, fmt.Errorf("failed to convert type: %T", extPropValue)
	}
	return goTypeSkipOptionalPointer, nil
}

func extParseGoFieldName(extPropValue any) (string, error) {
	return extString(extPropValue)
}

func extParseOmitEmpty(extPropValue any) (bool, error) {
	omitEmpty, ok := extPropValue.(bool)
	if !ok {
		return false, fmt.Errorf("failed to convert type: %T", extPropValue)
	}
	return omitEmpty, nil
}

func extParseOmitZero(extPropValue any) (bool, error) {
	omitZero, ok := extPropValue.(bool)
	if !ok {
		return false, fmt.Errorf("failed to convert type: %T", extPropValue)
	}
	return omitZero, nil
}

func extExtraTags(extPropValue any) (map[string]string, error) {
	tagsI, ok := extPropValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("failed to convert type: %T", extPropValue)
	}
	tags := make(map[string]string, len(tagsI))
	for k, v := range tagsI {
		vs, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("failed to convert type: %T", v)
		}
		tags[k] = vs
	}
	return tags, nil
}

func extParseGoJsonIgnore(extPropValue any) (bool, error) {
	goJsonIgnore, ok := extPropValue.(bool)
	if !ok {
		return false, fmt.Errorf("failed to convert type: %T", extPropValue)
	}
	return goJsonIgnore, nil
}

func extParseEnumVarNames(extPropValue any) ([]string, error) {
	namesI, ok := extPropValue.([]any)
	if !ok {
		return nil, fmt.Errorf("failed to convert type: %T", extPropValue)
	}
	names := make([]string, len(namesI))
	for i, v := range namesI {
		vs, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("failed to convert type: %T", v)
		}
		names[i] = vs
	}
	return names, nil
}

func extParseDeprecationReason(extPropValue any) (string, error) {
	return extString(extPropValue)
}

func extParseOapiCodegenOnlyHonourGoName(extPropValue any) (bool, error) {
	onlyHonourGoName, ok := extPropValue.(bool)
	if !ok {
		return false, fmt.Errorf("failed to convert type: %T", extPropValue)
	}
	return onlyHonourGoName, nil
}

// extParseGoRef parses the raw value of an x-go-ref extension into a goRef.
// The extension is expected to be a map with "path" (required) and optional "alias" keys.
func extParseGoRef(extPropValue any) (*goRef, error) {
	m, ok := extPropValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("failed to convert type: %T", extPropValue)
	}
	ref := &goRef{}
	for k, v := range m {
		if strings.EqualFold(k, "path") {
			vs, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("failed to convert type: %T", v)
			}
			ref.Path = vs
		} else if strings.EqualFold(k, "alias") {
			vs, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("failed to convert type: %T", v)
			}
			ref.Alias = vs
		}
	}
	return ref, nil
}
