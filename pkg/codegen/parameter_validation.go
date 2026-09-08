package codegen

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/oapi-codegen/oapi-codegen/v2/pkg/util"
	"gopkg.in/yaml.v3"
)

type ParameterValidationPlan struct {
	UseCustomType bool
	SkipReason    string
	Kind          string

	HasMinLength  bool
	MinLength     uint64
	HasMaxLength  bool
	MaxLength     uint64
	HasPattern    bool
	Pattern       string
	HasStringEnum bool
	StringEnum    []string

	HasIntegerMinimum       bool
	IntegerMinimum          string
	ExclusiveIntegerMinimum bool
	HasIntegerMaximum       bool
	IntegerMaximum          string
	ExclusiveIntegerMaximum bool
	HasIntegerEnum          bool
	IntegerEnum             []string

	HasMinimum       bool
	Minimum          float64
	ExclusiveMinimum bool
	HasMaximum       bool
	Maximum          float64
	ExclusiveMaximum bool
	HasNumberEnum    bool
	NumberEnum       []float64
}

func (p ParameterValidationPlan) HasValidation() bool {
	if p.UseCustomType || p.SkipReason != "" {
		return false
	}
	switch p.Kind {
	case openapi3.TypeString:
		return p.HasMinLength || p.HasMaxLength || p.HasPattern || p.HasStringEnum
	case openapi3.TypeInteger:
		return p.HasIntegerMinimum || p.HasIntegerMaximum || p.HasIntegerEnum
	case openapi3.TypeNumber:
		return p.HasMinimum || p.HasMaximum || p.HasNumberEnum
	default:
		return false
	}
}

func (pd ParameterDefinition) HasValidation() bool {
	return pd.Validation.HasValidation()
}

func (pd ParameterDefinition) UsesCustomTypeValidation() bool {
	return pd.Validation.UseCustomType
}

func (pd ParameterDefinition) ValidationCall(valueExpr string) string {
	p := pd.Validation
	switch p.Kind {
	case openapi3.TypeString:
		return fmt.Sprintf(
			`validateParamString(%q, string(%s), %d, %t, %d, %t, %q, %t, %s, %t)`,
			pd.ParamName,
			valueExpr,
			p.MinLength,
			p.HasMinLength,
			p.MaxLength,
			p.HasMaxLength,
			p.Pattern,
			p.HasPattern,
			goStringSliceLiteral(p.StringEnum),
			p.HasStringEnum,
		)
	case openapi3.TypeInteger:
		return fmt.Sprintf(
			`validateParamInteger(%q, %s, %q, %t, %t, %q, %t, %t, %s, %t)`,
			pd.ParamName,
			valueExpr,
			p.IntegerMinimum,
			p.HasIntegerMinimum,
			p.ExclusiveIntegerMinimum,
			p.IntegerMaximum,
			p.HasIntegerMaximum,
			p.ExclusiveIntegerMaximum,
			goStringSliceLiteral(p.IntegerEnum),
			p.HasIntegerEnum,
		)
	case openapi3.TypeNumber:
		return fmt.Sprintf(
			`validateParamNumber(%q, float64(%s), %f, %t, %t, %f, %t, %t, %s, %t)`,
			pd.ParamName,
			valueExpr,
			p.Minimum,
			p.HasMinimum,
			p.ExclusiveMinimum,
			p.Maximum,
			p.HasMaximum,
			p.ExclusiveMaximum,
			goFloat64SliceLiteral(p.NumberEnum),
			p.HasNumberEnum,
		)
	default:
		return ""
	}
}

func (pd ParameterDefinition) CustomTypeValidationCall(rawValueExpr, targetExpr string) string {
	return fmt.Sprintf(`validateParamCustomType(%q, %s, %s)`, pd.ParamName, rawValueExpr, targetExpr)
}

type parameterValidationSchema struct {
	Ref string

	Type   *string
	Format *string

	MinLength *uint64
	MaxLength *uint64
	Pattern   *string
	Enum      []any
	HasEnum   bool

	Minimum          *float64
	Maximum          *float64
	MinimumText      *string
	MaximumText      *string
	ExclusiveMinimum *bool
	ExclusiveMaximum *bool

	XGoType       *string
	XGoTypeImport any
	XGoRef        any

	HasAllOf bool
	HasAnyOf bool
	HasOneOf bool
	HasNot   bool
}

func (s parameterValidationSchema) mergedOver(base parameterValidationSchema) parameterValidationSchema {
	out := base
	if s.Type != nil {
		out.Type = s.Type
	}
	if s.Format != nil {
		out.Format = s.Format
	}
	if s.MinLength != nil {
		out.MinLength = s.MinLength
	}
	if s.MaxLength != nil {
		out.MaxLength = s.MaxLength
	}
	if s.Pattern != nil {
		out.Pattern = s.Pattern
	}
	if s.HasEnum {
		out.Enum = append([]any(nil), s.Enum...)
		out.HasEnum = true
	}
	if s.Minimum != nil {
		out.Minimum = s.Minimum
	}
	if s.Maximum != nil {
		out.Maximum = s.Maximum
	}
	if s.MinimumText != nil {
		out.MinimumText = s.MinimumText
	}
	if s.MaximumText != nil {
		out.MaximumText = s.MaximumText
	}
	if s.ExclusiveMinimum != nil {
		out.ExclusiveMinimum = s.ExclusiveMinimum
	}
	if s.ExclusiveMaximum != nil {
		out.ExclusiveMaximum = s.ExclusiveMaximum
	}
	if s.XGoType != nil {
		out.XGoType = s.XGoType
	}
	if s.XGoTypeImport != nil {
		out.XGoTypeImport = s.XGoTypeImport
	}
	if s.XGoRef != nil {
		out.XGoRef = s.XGoRef
	}
	out.HasAllOf = out.HasAllOf || s.HasAllOf
	out.HasAnyOf = out.HasAnyOf || s.HasAnyOf
	out.HasOneOf = out.HasOneOf || s.HasOneOf
	out.HasNot = out.HasNot || s.HasNot
	return out
}

type parameterValidationResolver struct {
	rootPath string
	docs     map[string]*yaml.Node
}

type parameterValidationLookupError struct {
	err error
}

type parameterValidationCycleError struct {
	ref string
}

func (e *parameterValidationCycleError) Error() string {
	return fmt.Sprintf("parameter schema reference cycle detected at %s", e.ref)
}

func (e *parameterValidationLookupError) Error() string {
	return e.err.Error()
}

func (e *parameterValidationLookupError) Unwrap() error {
	return e.err
}

func newParameterValidationResolver(specPath string) (*parameterValidationResolver, error) {
	if specPath == "" {
		return nil, nil
	}
	u, err := url.Parse(specPath)
	if err == nil && u.Scheme != "" && u.Host != "" {
		return nil, nil
	}
	abs, err := filepath.Abs(specPath)
	if err != nil {
		return nil, err
	}
	return &parameterValidationResolver{
		rootPath: abs,
		docs:     map[string]*yaml.Node{},
	}, nil
}

func (r *parameterValidationResolver) resolvePlan(paramRef *openapi3.ParameterRef, basePath []string, index int) (ParameterValidationPlan, error) {
	if paramRef == nil || paramRef.Value == nil || paramRef.Value.Schema == nil {
		return ParameterValidationPlan{}, nil
	}
	if r == nil {
		return buildParameterValidationPlanFromLoadedSchema(paramRef.Value.Schema)
	}
	schemaNode, schemaFile, err := r.parameterSchemaNode(paramRef, basePath, index)
	if err != nil {
		var lookupErr *parameterValidationLookupError
		if errors.As(err, &lookupErr) {
			fmt.Fprintf(os.Stderr, "Warning: failed to resolve original parameter validation source for %q in %q: %v; falling back to resolved schema\n", paramRef.Value.Name, paramRef.Value.In, err)
			return buildParameterValidationPlanFromLoadedSchema(paramRef.Value.Schema)
		}
		return ParameterValidationPlan{}, err
	}
	if schemaNode == nil {
		return buildParameterValidationPlanFromLoadedSchema(paramRef.Value.Schema)
	}
	effective, err := r.resolveSchema(schemaFile, schemaNode, map[string]bool{})
	if err != nil {
		var cycleErr *parameterValidationCycleError
		if errors.As(err, &cycleErr) {
			return buildParameterValidationPlanFromLoadedSchema(paramRef.Value.Schema)
		}
		return ParameterValidationPlan{}, err
	}
	return buildParameterValidationPlan(effective)
}

func (r *parameterValidationResolver) parameterSchemaNode(paramRef *openapi3.ParameterRef, basePath []string, _ int) (*yaml.Node, string, error) {
	var (
		paramNode *yaml.Node
		filePath  string
		err       error
	)
	switch {
	case paramRef.Ref != "":
		paramNode, filePath, err = r.resolveRefNode(r.rootPath, paramRef.Ref, map[string]bool{})
		if err != nil {
			return nil, "", err
		}
	case len(basePath) != 0:
		externalSource, err := r.hasExternalRefSource(basePath)
		if err != nil {
			return nil, "", &parameterValidationLookupError{err: err}
		}
		if externalSource {
			return nil, "", nil
		}
		doc, err := r.loadDocument(r.rootPath)
		if err != nil {
			return nil, "", &parameterValidationLookupError{err: err}
		}
		paramsNode, err := followYAMLPath(doc, basePath...)
		if err != nil {
			return nil, "", &parameterValidationLookupError{err: err}
		}
		paramNode, filePath, err = r.findParameterNode(paramsNode, r.rootPath, paramRef)
		if err != nil {
			return nil, "", err
		}
	default:
		return nil, "", nil
	}
	paramNode, filePath, err = r.resolveParameterNode(filePath, paramNode, map[string]bool{})
	if err != nil {
		return nil, "", err
	}
	return yamlMapValue(paramNode, "schema"), filePath, nil
}

func (r *parameterValidationResolver) parameterTypeSchemaNode(paramRef *openapi3.ParameterRef, basePath []string, index int) (*yaml.Node, string, error) {
	if paramRef == nil || paramRef.Value == nil {
		return nil, "", nil
	}
	if paramRef.Ref != "" || len(basePath) == 0 || basePath[0] != "components" {
		return r.parameterSchemaNode(paramRef, basePath, index)
	}

	doc, err := r.loadDocument(r.rootPath)
	if err != nil {
		return nil, "", &parameterValidationLookupError{err: err}
	}
	paramNode, err := followYAMLPath(doc, basePath...)
	if err != nil {
		return nil, "", &parameterValidationLookupError{err: err}
	}
	paramNode, filePath, err := r.resolveParameterNode(r.rootPath, paramNode, map[string]bool{})
	if err != nil {
		return nil, "", err
	}
	return yamlMapValue(paramNode, "schema"), filePath, nil
}

func (r *parameterValidationResolver) findParameterNode(paramsNode *yaml.Node, currentFile string, paramRef *openapi3.ParameterRef) (*yaml.Node, string, error) {
	if paramsNode == nil || paramsNode.Kind != yaml.SequenceNode {
		return nil, "", &parameterValidationLookupError{err: fmt.Errorf("parameter source path did not resolve to a sequence")}
	}

	var (
		matchCount int
		matchNode  *yaml.Node
	)
	for _, candidate := range paramsNode.Content {
		matched, err := r.parameterNodeMatches(candidate, currentFile, paramRef)
		if err != nil {
			return nil, "", err
		}
		if matched {
			matchCount++
			matchNode = candidate
		}
	}

	switch matchCount {
	case 1:
		return matchNode, currentFile, nil
	case 0:
		return nil, "", &parameterValidationLookupError{err: fmt.Errorf("parameter %q in %q not found in source sequence", paramRef.Value.Name, paramRef.Value.In)}
	default:
		return nil, "", &parameterValidationLookupError{err: fmt.Errorf("parameter %q in %q matched multiple source entries", paramRef.Value.Name, paramRef.Value.In)}
	}
}

func (r *parameterValidationResolver) parameterNodeMatches(node *yaml.Node, currentFile string, paramRef *openapi3.ParameterRef) (bool, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return false, nil
	}
	if paramRef.Ref != "" {
		refNode := yamlMapValue(node, "$ref")
		return refNode != nil && refNode.Kind == yaml.ScalarNode && refNode.Value == paramRef.Ref, nil
	}

	refNode := yamlMapValue(node, "$ref")
	if refNode == nil || refNode.Kind != yaml.ScalarNode {
		name := yamlMapString(node, "name")
		in := yamlMapString(node, "in")
		if name == nil || in == nil {
			return false, nil
		}
		return *name == paramRef.Value.Name && *in == paramRef.Value.In, nil
	}

	resolvedNode, _, err := r.resolveParameterNode(currentFile, node, map[string]bool{})
	if err != nil {
		return false, err
	}

	name := yamlMapString(resolvedNode, "name")
	in := yamlMapString(resolvedNode, "in")
	if name == nil || in == nil {
		return false, nil
	}
	return *name == paramRef.Value.Name && *in == paramRef.Value.In, nil
}

func (r *parameterValidationResolver) resolveParameterNode(currentFile string, node *yaml.Node, seen map[string]bool) (*yaml.Node, string, error) {
	refNode := yamlMapValue(node, "$ref")
	if refNode == nil || refNode.Kind != yaml.ScalarNode {
		return node, currentFile, nil
	}
	return r.resolveRefNode(currentFile, refNode.Value, seen)
}

func (r *parameterValidationResolver) hasExternalRefSource(basePath []string) (bool, error) {
	if len(basePath) == 0 {
		return false, nil
	}
	node, err := r.loadDocument(r.rootPath)
	if err != nil {
		return false, err
	}

	currentFile := r.rootPath
	seen := map[string]bool{}
	for _, part := range basePath {
		node, currentFile, err = r.resolveParameterNode(currentFile, node, seen)
		if err != nil {
			return false, err
		}
		if currentFile != r.rootPath {
			return true, nil
		}

		switch node.Kind {
		case yaml.MappingNode:
			next := yamlMapValue(node, part)
			if next == nil {
				return false, nil
			}
			node = next
		case yaml.SequenceNode:
			idx, convErr := strconv.Atoi(part)
			if convErr != nil || idx < 0 || idx >= len(node.Content) {
				return false, nil
			}
			node = node.Content[idx]
		default:
			return false, nil
		}
	}

	return false, nil
}

func (r *parameterValidationResolver) resolveSchema(currentFile string, node *yaml.Node, seen map[string]bool) (parameterValidationSchema, error) {
	local, err := parseParameterValidationSchema(node)
	if err != nil {
		return parameterValidationSchema{}, err
	}
	if local.Ref == "" {
		return local, nil
	}
	refNode, refFile, err := r.resolveRefNode(currentFile, local.Ref, seen)
	if err != nil {
		return parameterValidationSchema{}, err
	}
	referenced, err := r.resolveSchema(refFile, refNode, seen)
	if err != nil {
		return parameterValidationSchema{}, err
	}
	local.Ref = ""
	return local.mergedOver(referenced), nil
}

func (r *parameterValidationResolver) resolveSchemaRef(currentFile string, sref *openapi3.SchemaRef, seen map[string]bool) (parameterValidationSchema, error) {
	if sref == nil {
		return parameterValidationSchema{}, nil
	}
	node, err := parameterValidationSchemaNodeFromSchemaRef(sref)
	if err != nil {
		return parameterValidationSchema{}, err
	}
	return r.resolveSchema(currentFile, node, seen)
}

func parameterValidationSchemaNodeFromSchemaRef(sref *openapi3.SchemaRef) (*yaml.Node, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addEncoded := func(key string, value any) error {
		valueNode := &yaml.Node{}
		if err := valueNode.Encode(value); err != nil {
			return err
		}
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			valueNode,
		)
		return nil
	}
	addNode := func(key string, valueNode *yaml.Node) {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			valueNode,
		)
	}

	if sref.Ref != "" {
		if err := addEncoded("$ref", sref.Ref); err != nil {
			return nil, err
		}
	}
	if sref.Value == nil {
		return node, nil
	}

	schema := sref.Value
	if schema.Type != nil {
		if types := schema.Type.Slice(); len(types) > 0 {
			if err := addEncoded("type", types[0]); err != nil {
				return nil, err
			}
		}
	}
	if schema.Format != "" {
		if err := addEncoded("format", schema.Format); err != nil {
			return nil, err
		}
	}
	if schema.MinLength != 0 {
		if err := addEncoded("minLength", schema.MinLength); err != nil {
			return nil, err
		}
	}
	if schema.MaxLength != nil {
		if err := addEncoded("maxLength", *schema.MaxLength); err != nil {
			return nil, err
		}
	}
	if schema.Pattern != "" {
		if err := addEncoded("pattern", schema.Pattern); err != nil {
			return nil, err
		}
	}
	if len(schema.Enum) > 0 {
		if err := addEncoded("enum", schema.Enum); err != nil {
			return nil, err
		}
	}
	if schema.Min != nil {
		if err := addEncoded("minimum", *schema.Min); err != nil {
			return nil, err
		}
		if schema.ExclusiveMin {
			if err := addEncoded("exclusiveMinimum", schema.ExclusiveMin); err != nil {
				return nil, err
			}
		}
	}
	if schema.Max != nil {
		if err := addEncoded("maximum", *schema.Max); err != nil {
			return nil, err
		}
		if schema.ExclusiveMax {
			if err := addEncoded("exclusiveMaximum", schema.ExclusiveMax); err != nil {
				return nil, err
			}
		}
	}

	extensions := combinedSchemaExtensions(sref)
	if v, ok := extensions[extPropGoType]; ok && v != nil {
		if err := addEncoded(extPropGoType, v); err != nil {
			return nil, err
		}
	}
	if v, ok := extensions[extPropGoImport]; ok && v != nil {
		if err := addEncoded(extPropGoImport, v); err != nil {
			return nil, err
		}
	}
	if v, ok := extensions[extPropGoRef]; ok && v != nil {
		if err := addEncoded(extPropGoRef, v); err != nil {
			return nil, err
		}
	}

	if len(schema.AllOf) > 0 {
		addNode("allOf", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"})
	}
	if len(schema.AnyOf) > 0 {
		addNode("anyOf", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"})
	}
	if len(schema.OneOf) > 0 {
		addNode("oneOf", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"})
	}
	if schema.Not != nil {
		addNode("not", &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
	}

	return node, nil
}

func (r *parameterValidationResolver) resolveRefNode(currentFile string, ref string, seen map[string]bool) (*yaml.Node, string, error) {
	targetFile, fragment, err := resolveYAMLReference(currentFile, ref)
	if err != nil {
		return nil, "", err
	}
	visitKey := targetFile + "#" + fragment
	if seen[visitKey] {
		return nil, "", &parameterValidationCycleError{ref: ref}
	}
	seen[visitKey] = true

	doc, err := r.loadDocument(targetFile)
	if err != nil {
		return nil, "", err
	}
	node, err := followJSONPointer(doc, fragment)
	if err != nil {
		return nil, "", err
	}
	return node, targetFile, nil
}

func (r *parameterValidationResolver) loadDocument(filePath string) (*yaml.Node, error) {
	if doc, ok := r.docs[filePath]; ok {
		return doc, nil
	}
	data, err := util.PreprocessSwaggerIncludes(filePath)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("yaml document %q is empty", filePath)
	}
	root := doc.Content[0]
	r.docs[filePath] = root
	return root, nil
}

func buildParameterValidationPlanFromLoadedSchema(sref *openapi3.SchemaRef) (ParameterValidationPlan, error) {
	if sref == nil || sref.Value == nil {
		return ParameterValidationPlan{}, nil
	}
	effective := parameterValidationSchema{}
	if sref.Value.Type != nil {
		if types := sref.Value.Type.Slice(); len(types) > 0 {
			effective.Type = &types[0]
		}
	}
	if sref.Value.Format != "" {
		format := sref.Value.Format
		effective.Format = &format
	}
	if sref.Value.MinLength != 0 {
		v := sref.Value.MinLength
		effective.MinLength = &v
	}
	if sref.Value.MaxLength != nil {
		v := *sref.Value.MaxLength
		effective.MaxLength = &v
	}
	if sref.Value.Pattern != "" {
		v := sref.Value.Pattern
		effective.Pattern = &v
	}
	if len(sref.Value.Enum) > 0 {
		effective.Enum = append([]any(nil), sref.Value.Enum...)
		effective.HasEnum = true
	}
	effective.Minimum = sref.Value.Min
	effective.Maximum = sref.Value.Max
	if sref.Value.Min != nil {
		v := strconv.FormatFloat(*sref.Value.Min, 'f', -1, 64)
		effective.MinimumText = &v
	}
	if sref.Value.Max != nil {
		v := strconv.FormatFloat(*sref.Value.Max, 'f', -1, 64)
		effective.MaximumText = &v
	}
	if sref.Value.Min != nil {
		v := sref.Value.ExclusiveMin
		effective.ExclusiveMinimum = &v
	}
	if sref.Value.Max != nil {
		v := sref.Value.ExclusiveMax
		effective.ExclusiveMaximum = &v
	}
	extensions := combinedSchemaExtensions(sref)
	if v, ok := extensions[extPropGoType].(string); ok && v != "" {
		effective.XGoType = &v
	}
	if v, ok := extensions[extPropGoImport]; ok {
		effective.XGoTypeImport = v
	}
	if v, ok := extensions[extPropGoRef]; ok {
		effective.XGoRef = v
	}
	effective.HasAllOf = len(sref.Value.AllOf) > 0
	effective.HasAnyOf = len(sref.Value.AnyOf) > 0
	effective.HasOneOf = len(sref.Value.OneOf) > 0
	effective.HasNot = sref.Value.Not != nil
	return buildParameterValidationPlan(effective)
}

func buildParameterValidationPlan(schema parameterValidationSchema) (ParameterValidationPlan, error) {
	if schema.XGoType != nil {
		return ParameterValidationPlan{UseCustomType: true}, nil
	}
	if schema.HasAllOf || schema.HasAnyOf || schema.HasOneOf || schema.HasNot {
		return ParameterValidationPlan{SkipReason: "unsupported composed schema"}, nil
	}
	if schema.Type == nil {
		return ParameterValidationPlan{}, nil
	}

	switch *schema.Type {
	case openapi3.TypeString:
		plan := ParameterValidationPlan{Kind: openapi3.TypeString}
		if schema.MinLength != nil {
			plan.HasMinLength = true
			plan.MinLength = *schema.MinLength
		}
		if schema.MaxLength != nil {
			plan.HasMaxLength = true
			plan.MaxLength = *schema.MaxLength
		}
		if schema.Pattern != nil {
			plan.HasPattern = true
			plan.Pattern = *schema.Pattern
		}
		if schema.HasEnum {
			values := make([]string, 0, len(schema.Enum))
			for _, raw := range schema.Enum {
				s, ok := raw.(string)
				if !ok {
					return ParameterValidationPlan{}, fmt.Errorf("string parameter enum contains non-string value of type %T", raw)
				}
				values = append(values, s)
			}
			plan.HasStringEnum = true
			plan.StringEnum = values
		}
		return plan, nil
	case openapi3.TypeInteger:
		plan := ParameterValidationPlan{Kind: *schema.Type}
		if schema.Minimum != nil {
			plan.HasIntegerMinimum = true
			plan.IntegerMinimum = integerConstraintString(schema.MinimumText, *schema.Minimum)
			if schema.ExclusiveMinimum != nil {
				plan.ExclusiveIntegerMinimum = *schema.ExclusiveMinimum
			}
		}
		if schema.Maximum != nil {
			plan.HasIntegerMaximum = true
			plan.IntegerMaximum = integerConstraintString(schema.MaximumText, *schema.Maximum)
			if schema.ExclusiveMaximum != nil {
				plan.ExclusiveIntegerMaximum = *schema.ExclusiveMaximum
			}
		}
		if schema.HasEnum {
			values := make([]string, 0, len(schema.Enum))
			for _, raw := range schema.Enum {
				n, ok := anyToIntegerString(raw)
				if !ok {
					return ParameterValidationPlan{}, fmt.Errorf("numeric parameter enum contains non-numeric value of type %T", raw)
				}
				values = append(values, n)
			}
			plan.HasIntegerEnum = true
			plan.IntegerEnum = values
		}
		return plan, nil
	case openapi3.TypeNumber:
		plan := ParameterValidationPlan{Kind: *schema.Type}
		if schema.Minimum != nil {
			plan.HasMinimum = true
			plan.Minimum = *schema.Minimum
			if schema.ExclusiveMinimum != nil {
				plan.ExclusiveMinimum = *schema.ExclusiveMinimum
			}
		}
		if schema.Maximum != nil {
			plan.HasMaximum = true
			plan.Maximum = *schema.Maximum
			if schema.ExclusiveMaximum != nil {
				plan.ExclusiveMaximum = *schema.ExclusiveMaximum
			}
		}
		if schema.HasEnum {
			values := make([]float64, 0, len(schema.Enum))
			for _, raw := range schema.Enum {
				n, ok := anyToFloat64(raw)
				if !ok {
					return ParameterValidationPlan{}, fmt.Errorf("numeric parameter enum contains non-numeric value of type %T", raw)
				}
				values = append(values, n)
			}
			plan.HasNumberEnum = true
			plan.NumberEnum = values
		}
		return plan, nil
	case openapi3.TypeArray, openapi3.TypeObject:
		return ParameterValidationPlan{SkipReason: "unsupported container schema"}, nil
	default:
		return ParameterValidationPlan{}, nil
	}
}

func parseParameterValidationSchema(node *yaml.Node) (parameterValidationSchema, error) {
	out := parameterValidationSchema{}
	if node == nil || node.Kind != yaml.MappingNode {
		return out, nil
	}
	if v := yamlMapValue(node, "$ref"); v != nil && v.Kind == yaml.ScalarNode {
		out.Ref = v.Value
	}
	out.Type = yamlMapString(node, "type")
	out.Format = yamlMapString(node, "format")
	out.MinLength = yamlMapUint64(node, "minLength")
	out.MaxLength = yamlMapUint64(node, "maxLength")
	out.Pattern = yamlMapString(node, "pattern")
	if v := yamlMapValue(node, "enum"); v != nil {
		out.HasEnum = true
		if err := v.Decode(&out.Enum); err != nil {
			return out, err
		}
	}
	out.Minimum = yamlMapFloat64(node, "minimum")
	out.Maximum = yamlMapFloat64(node, "maximum")
	out.MinimumText = yamlMapString(node, "minimum")
	out.MaximumText = yamlMapString(node, "maximum")
	out.ExclusiveMinimum = yamlMapBool(node, "exclusiveMinimum")
	out.ExclusiveMaximum = yamlMapBool(node, "exclusiveMaximum")
	out.XGoType = yamlMapString(node, extPropGoType)
	if v := yamlMapValue(node, extPropGoImport); v != nil {
		var decoded any
		if err := v.Decode(&decoded); err != nil {
			return out, err
		}
		out.XGoTypeImport = decoded
	}
	if v := yamlMapValue(node, extPropGoRef); v != nil {
		var decoded any
		if err := v.Decode(&decoded); err != nil {
			return out, err
		}
		out.XGoRef = decoded
	}
	out.HasAllOf = yamlMapValue(node, "allOf") != nil
	out.HasAnyOf = yamlMapValue(node, "anyOf") != nil
	out.HasOneOf = yamlMapValue(node, "oneOf") != nil
	out.HasNot = yamlMapValue(node, "not") != nil
	return out, nil
}

func resolveYAMLReference(currentFile, ref string) (string, string, error) {
	parts := strings.SplitN(ref, "#", 2)
	targetFile := currentFile
	if parts[0] != "" {
		if isURLReference(parts[0]) {
			return "", "", fmt.Errorf("unsupported remote reference: %s", ref)
		}
		targetFile = filepath.Clean(filepath.Join(filepath.Dir(currentFile), filepath.FromSlash(parts[0])))
	}
	fragment := ""
	if len(parts) == 2 {
		fragment = parts[1]
	}
	return targetFile, fragment, nil
}

func followYAMLPath(root *yaml.Node, parts ...string) (*yaml.Node, error) {
	node := root
	var err error
	for _, part := range parts {
		switch node.Kind {
		case yaml.MappingNode:
			node = yamlMapValue(node, part)
			if node == nil {
				return nil, fmt.Errorf("yaml path not found at %q", part)
			}
		case yaml.SequenceNode:
			idx, convErr := strconv.Atoi(part)
			if convErr != nil || idx < 0 || idx >= len(node.Content) {
				return nil, fmt.Errorf("yaml sequence index %q out of range", part)
			}
			node = node.Content[idx]
		case yaml.DocumentNode:
			if len(node.Content) == 0 {
				return nil, fmt.Errorf("empty yaml document")
			}
			node = node.Content[0]
			node, err = followYAMLPath(node, part)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("yaml path traversed through non-container node")
		}
	}
	return node, nil
}

func followJSONPointer(root *yaml.Node, fragment string) (*yaml.Node, error) {
	if fragment == "" {
		return root, nil
	}
	if !strings.HasPrefix(fragment, "/") {
		return nil, fmt.Errorf("unsupported reference fragment %q", fragment)
	}
	parts := strings.Split(fragment[1:], "/")
	for i := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(parts[i], "~1", "/"), "~0", "~")
	}
	return followYAMLPath(root, parts...)
}

func yamlMapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func yamlMapString(node *yaml.Node, key string) *string {
	v := yamlMapValue(node, key)
	if v == nil {
		return nil
	}
	var out string
	if err := v.Decode(&out); err != nil {
		return nil
	}
	return &out
}

func yamlMapUint64(node *yaml.Node, key string) *uint64 {
	v := yamlMapValue(node, key)
	if v == nil {
		return nil
	}
	var out uint64
	if err := v.Decode(&out); err != nil {
		return nil
	}
	return &out
}

func yamlMapFloat64(node *yaml.Node, key string) *float64 {
	v := yamlMapValue(node, key)
	if v == nil {
		return nil
	}
	var out float64
	if err := v.Decode(&out); err != nil {
		return nil
	}
	return &out
}

func yamlMapBool(node *yaml.Node, key string) *bool {
	v := yamlMapValue(node, key)
	if v == nil {
		return nil
	}
	var out bool
	if err := v.Decode(&out); err != nil {
		return nil
	}
	return &out
}

func isURLReference(v string) bool {
	u, err := url.Parse(v)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func anyToFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func anyToIntegerString(v any) (string, bool) {
	switch n := v.(type) {
	case int:
		return strconv.FormatInt(int64(n), 10), true
	case int8:
		return strconv.FormatInt(int64(n), 10), true
	case int16:
		return strconv.FormatInt(int64(n), 10), true
	case int32:
		return strconv.FormatInt(int64(n), 10), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case uint:
		return strconv.FormatUint(uint64(n), 10), true
	case uint8:
		return strconv.FormatUint(uint64(n), 10), true
	case uint16:
		return strconv.FormatUint(uint64(n), 10), true
	case uint32:
		return strconv.FormatUint(uint64(n), 10), true
	case uint64:
		return strconv.FormatUint(n, 10), true
	case float32:
		if n != float32(int64(n)) {
			return "", false
		}
		return strconv.FormatInt(int64(n), 10), true
	case float64:
		if n != float64(int64(n)) {
			return "", false
		}
		return strconv.FormatInt(int64(n), 10), true
	default:
		return "", false
	}
}

func integerConstraintString(valueText *string, valueFloat float64) string {
	if valueText != nil && *valueText != "" {
		return *valueText
	}
	return strconv.FormatFloat(valueFloat, 'f', -1, 64)
}

func goStringSliceLiteral(values []string) string {
	if len(values) == 0 {
		return "nil"
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strconv.Quote(value)
	}
	return "[]string{" + strings.Join(out, ", ") + "}"
}

func goFloat64SliceLiteral(values []float64) string {
	if len(values) == 0 {
		return "nil"
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strconv.FormatFloat(value, 'f', -1, 64)
	}
	return "[]float64{" + strings.Join(out, ", ") + "}"
}
