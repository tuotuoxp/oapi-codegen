package util

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/speakeasy-api/openapi/overlay/loader"
	"gopkg.in/yaml.v3"
)

func LoadSwagger(filePath string) (swagger *openapi3.T, err error) {

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	u, err := url.Parse(filePath)
	if err == nil && u.Scheme != "" && u.Host != "" {
		return loader.LoadFromURI(u)
	}

	specData, err := preprocessSwaggerIncludes(filePath)
	if err != nil {
		return nil, err
	}
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to determine absolute path for specification %q: %w", filePath, err)
	}

	return loader.LoadFromDataWithPath(specData, &url.URL{
		Path: filepath.ToSlash(absFilePath),
	})
}

// Deprecated: In kin-openapi v0.126.0 (https://github.com/getkin/kin-openapi/tree/v0.126.0?tab=readme-ov-file#v01260) the Circular Reference Counter functionality was removed, instead resolving all references with backtracking, to avoid needing to provide a limit to reference counts.
//
// This is now identital in method as `LoadSwagger`.
func LoadSwaggerWithCircularReferenceCount(filePath string, _ int) (swagger *openapi3.T, err error) {
	return LoadSwagger(filePath)
}

type LoadSwaggerWithOverlayOpts struct {
	Path   string
	Strict bool
}

func LoadSwaggerWithOverlay(filePath string, opts LoadSwaggerWithOverlayOpts) (swagger *openapi3.T, err error) {
	spec, err := LoadSwagger(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI specification: %w", err)
	}

	if opts.Path == "" {
		return spec, nil
	}

	// parse out the yaml.Node, which is required by the overlay library
	buf := &bytes.Buffer{}
	enc := yaml.NewEncoder(buf)
	// set to 2 to work around https://github.com/yaml/go-yaml/issues/76
	enc.SetIndent(2)
	err = enc.Encode(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec from %#v as YAML: %w", filePath, err)
	}

	var node yaml.Node
	err = yaml.NewDecoder(buf).Decode(&node)
	if err != nil {
		return nil, fmt.Errorf("failed to parse spec from %#v: %w", filePath, err)
	}

	overlay, err := loader.LoadOverlay(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to load Overlay from %#v: %v", opts.Path, err)
	}

	err = overlay.Validate()
	if err != nil {
		return nil, fmt.Errorf("the Overlay in %#v was not valid: %v", opts.Path, err)
	}

	if opts.Strict {
		vs, err := overlay.ApplyToStrict(&node)
		if err != nil {
			return nil, fmt.Errorf("failed to apply Overlay %#v to specification %#v: %v\nAdditionally, the following validation errors were found:\n- %s", opts.Path, filePath, err, strings.Join(vs, "\n- "))
		}
	} else {
		err = overlay.ApplyTo(&node)
		if err != nil {
			return nil, fmt.Errorf("failed to apply Overlay %#v to specification %#v: %v", opts.Path, filePath, err)
		}
	}

	b, err := yaml.Marshal(&node)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize Overlay'd specification %#v: %v", opts.Path, err)
	}

	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	swagger, err = loader.LoadFromDataWithPath(b, &url.URL{
		Path: filepath.ToSlash(filePath),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to serialize Overlay'd specification %#v: %v", opts.Path, err)
	}

	return swagger, nil
}

func preprocessSwaggerIncludes(filePath string) ([]byte, error) {
	entryPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to determine absolute path for specification %q: %w", filePath, err)
	}

	rootDir := filepath.Dir(entryPath)
	rootNode, err := loadYAMLDocument(entryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse specification %q: %w", entryPath, err)
	}

	if err := expandIncludes(rootNode, entryPath, rootDir, []string{entryPath}); err != nil {
		return nil, err
	}

	specData, err := yaml.Marshal(rootNode)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize specification %q after include preprocessing: %w", entryPath, err)
	}

	return specData, nil
}

func loadYAMLDocument(filePath string) (*yaml.Node, error) {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var document yaml.Node
	if err := yaml.Unmarshal(fileData, &document); err != nil {
		return nil, err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return nil, errors.New("YAML document is empty")
	}

	return &document, nil
}

func expandIncludes(node *yaml.Node, currentFile string, rootDir string, stack []string) error {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := expandIncludes(child, currentFile, rootDir, stack); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		if err := expandIncludesInMap(node, currentFile, rootDir, stack); err != nil {
			return err
		}
	case yaml.SequenceNode:
		if err := expandIncludesInArray(node, currentFile, rootDir, stack); err != nil {
			return err
		}
	default:
		if node.Tag == "!include_array" {
			return fmt.Errorf("invalid use of !include_array in %q: !include_array is only supported for array items", currentFile)
		}
	}

	if node.Tag == "!include" {
		includedNode, _, err := loadIncludedNode(node, currentFile, rootDir, stack)
		if err != nil {
			return err
		}
		*node = *includedNode
	}

	return nil
}

func expandIncludesInMap(node *yaml.Node, currentFile string, rootDir string, stack []string) error {
	mergedValues := map[string]*yaml.Node{}
	mergedKeys := make([]string, 0)
	mergedKeyNodes := map[string]*yaml.Node{}
	resultPairs := make([]*yaml.Node, 0, len(node.Content))

	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "<<" && valueNode.Tag == "!include" {
			includedNode, includePath, err := loadIncludedNode(valueNode, currentFile, rootDir, stack)
			if err != nil {
				return err
			}
			if includedNode.Kind != yaml.MappingNode {
				return fmt.Errorf("failed to process merge include in %q for %q: included value must be an object/map", currentFile, includePath)
			}

			for j := 0; j+1 < len(includedNode.Content); j += 2 {
				mergedKeyNode := includedNode.Content[j]
				mergedValueNode := includedNode.Content[j+1]
				if mergedKeyNode.Kind != yaml.ScalarNode {
					return fmt.Errorf("failed to process merge include in %q for %q: included object contains a non-scalar key", currentFile, includePath)
				}

				mergedKey := mergedKeyNode.Value
				if _, found := mergedValues[mergedKey]; !found {
					mergedKeys = append(mergedKeys, mergedKey)
					mergedKeyNodes[mergedKey] = mergedKeyNode
				}
				mergedValues[mergedKey] = mergedValueNode
			}
			continue
		}

		if err := expandIncludes(valueNode, currentFile, rootDir, stack); err != nil {
			return err
		}
		resultPairs = append(resultPairs, keyNode, valueNode)

	if len(mergedKeys) == 0 {
		node.Content = resultPairs
		return nil
	}

	localExplicitKeys := map[string]struct{}{}
	for i := 0; i+1 < len(resultPairs); i += 2 {
		localExplicitKeys[resultPairs[i].Value] = struct{}{}
	}

	mergedPairs := make([]*yaml.Node, 0, len(mergedKeys)*2+len(resultPairs))
	for _, mergedKey := range mergedKeys {
		if _, found := localExplicitKeys[mergedKey]; found {
			continue
		}
		mergedPairs = append(mergedPairs, mergedKeyNodes[mergedKey], mergedValues[mergedKey])
	}
	mergedPairs = append(mergedPairs, resultPairs...)
	node.Content = mergedPairs
	return nil
}

func expandIncludesInArray(node *yaml.Node, currentFile string, rootDir string, stack []string) error {
	expandedItems := make([]*yaml.Node, 0, len(node.Content))
	for _, itemNode := range node.Content {
		if itemNode.Tag == "!include_array" {
			includedNode, includePath, err := loadIncludedNode(itemNode, currentFile, rootDir, stack)
			if err != nil {
				return err
			}
			if includedNode.Kind != yaml.SequenceNode {
				return fmt.Errorf("failed to process !include_array in %q for %q: included value must be an array", currentFile, includePath)
			}
			for _, includedItem := range includedNode.Content {
				expandedItems = append(expandedItems, cloneYAMLNode(includedItem))
			}
			continue
		}

		if err := expandIncludes(itemNode, currentFile, rootDir, stack); err != nil {
			return err
		}
		expandedItems = append(expandedItems, cloneYAMLNode(itemNode))
	}

	node.Content = expandedItems
	return nil
}

func loadIncludedNode(includeNode *yaml.Node, currentFile string, rootDir string, stack []string) (*yaml.Node, string, error) {
	if includeNode.Kind != yaml.ScalarNode {
		return nil, "", fmt.Errorf("invalid include in %q: include target must be a scalar path", currentFile)
	}

	includeTarget := strings.TrimSpace(includeNode.Value)
	if includeTarget == "" {
		return nil, "", fmt.Errorf("invalid include in %q: include target is empty", currentFile)
	}

	resolvedPath, err := resolveIncludePath(currentFile, rootDir, includeTarget)
	if err != nil {
		return nil, "", err
	}

	if stackIndex := slices.Index(stack, resolvedPath); stackIndex >= 0 {
		chain := append(append([]string{}, stack[stackIndex:]...), resolvedPath)
		return nil, "", fmt.Errorf("include cycle detected: %s", strings.Join(chain, " -> "))
	}

	includedDocument, err := loadYAMLDocument(resolvedPath)
	if err != nil {
		return nil, resolvedPath, fmt.Errorf("failed to include %q in %q: %w", resolvedPath, currentFile, err)
	}

	if err := expandIncludes(includedDocument, resolvedPath, rootDir, append(stack, resolvedPath)); err != nil {
		return nil, resolvedPath, err
	}

	return cloneYAMLNode(includedDocument.Content[0]), resolvedPath, nil
}

func resolveIncludePath(currentFile string, rootDir string, includeTarget string) (string, error) {
	includePath := includeTarget
	if !path.IsAbs(includeTarget) && !filepath.IsAbs(includeTarget) {
		includePath = filepath.Join(filepath.Dir(currentFile), includeTarget)
	}

	includePath = filepath.Clean(includePath)
	if !filepath.IsAbs(includePath) {
		absPath, err := filepath.Abs(includePath)
		if err != nil {
			return "", fmt.Errorf("failed to resolve include path %q in %q: %w", includeTarget, currentFile, err)
		}
		includePath = absPath
	}

	relativeToRoot, err := filepath.Rel(rootDir, includePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve include path %q in %q: %w", includeTarget, currentFile, err)
	}
	if relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, fmt.Sprintf("..%c", filepath.Separator)) {
		return "", fmt.Errorf("include path %q in %q resolves outside specification root %q", includeTarget, currentFile, rootDir)
	}

	return includePath, nil
}

func cloneYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}

	cloned := *node
	if len(node.Content) > 0 {
		cloned.Content = make([]*yaml.Node, len(node.Content))
		for index, child := range node.Content {
			cloned.Content[index] = cloneYAMLNode(child)
		}
	}

	return &cloned
}
