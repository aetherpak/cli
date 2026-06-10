// Package configedit performs comment-preserving edits of aetherpak.yaml by
// operating on the yaml.v3 node tree rather than re-marshaling the whole struct.
package configedit

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/config"
	"gopkg.in/yaml.v3"
)

// appOut is the minimal projection of an App that `add` writes. Field order
// here is the emitted key order; omitempty keeps the entry tidy.
type appOut struct {
	ID            string                   `yaml:"id"`
	Branch        string                   `yaml:"branch,omitempty"`
	Arches        []string                 `yaml:"arches,omitempty"`
	Manifest      string                   `yaml:"manifest,omitempty"`
	RunLinter     bool                     `yaml:"run-linter,omitempty"`
	CCache        *bool                    `yaml:"ccache,omitempty"`
	NoInstallDeps *bool                    `yaml:"no_install_deps,omitempty"`
	NoFlathub     *bool                    `yaml:"no_flathub,omitempty"`
	BuilderArgs   []string                 `yaml:"builder_args,omitempty"`
	Bundles       map[string]config.Bundle `yaml:"bundles,omitempty"`
}

// appNode marshals an App into a standalone YAML mapping node.
func appNode(app config.App) (*yaml.Node, error) {
	out := appOut{
		ID:            app.ID,
		Branch:        app.Branch,
		Arches:        app.Arches,
		Manifest:      app.Manifest,
		RunLinter:     app.RunLinter,
		CCache:        app.CCache,
		NoInstallDeps: app.NoInstallDeps,
		NoFlathub:     app.NoFlathub,
		BuilderArgs:   app.BuilderArgs,
		Bundles:       app.Bundles,
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("unexpected marshaled app shape")
	}
	return doc.Content[0], nil
}

// rootMapping returns the document and top-level mapping node of a parsed
// config, creating an empty document+mapping when existing is empty.
func rootMapping(existing []byte) (*yaml.Node, *yaml.Node, error) {
	var doc yaml.Node
	if len(bytes.TrimSpace(existing)) == 0 {
		mapping := &yaml.Node{Kind: yaml.MappingNode}
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
		return &doc, mapping, nil
	}
	if err := yaml.Unmarshal(existing, &doc); err != nil {
		return nil, nil, fmt.Errorf("failed to parse existing config: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil, fmt.Errorf("config is not a YAML document")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("config root is not a mapping")
	}
	return &doc, root, nil
}

// isNullNode reports whether n is an explicit or implicit YAML null scalar
// (e.g. the value of a bare "apps:" key).
func isNullNode(n *yaml.Node) bool {
	if n.Kind != yaml.ScalarNode {
		return false
	}
	return n.Tag == "!!null" || n.Value == "" || n.Value == "null" || n.Value == "~"
}

// findValue returns the value node for key in a mapping, or nil.
func findValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// AppendApp adds app to the apps sequence of existing, preserving all comments
// and ordering elsewhere, and returns the re-rendered bytes.
func AppendApp(existing []byte, app config.App) ([]byte, error) {
	doc, root, err := rootMapping(existing)
	if err != nil {
		return nil, err
	}

	apps := findValue(root, "apps")
	switch {
	case apps == nil:
		// Create apps: key with an empty sequence.
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "apps"}
		apps = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		root.Content = append(root.Content, keyNode, apps)
	case isNullNode(apps):
		// "apps:" with an empty/null value — convert the existing value node
		// (already wired into root.Content) into an empty sequence in place.
		apps.Kind = yaml.SequenceNode
		apps.Tag = "!!seq"
		apps.Value = ""
	case apps.Kind != yaml.SequenceNode:
		return nil, fmt.Errorf("'apps' is not a sequence")
	}

	node, err := appNode(app)
	if err != nil {
		return nil, err
	}
	apps.Content = append(apps.Content, node)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("failed to render config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("failed to close yaml encoder: %w", err)
	}
	return buf.Bytes(), nil
}

// HasApp reports whether existing already contains an app with the given id.
func HasApp(existing []byte, id string) (bool, error) {
	if len(bytes.TrimSpace(existing)) == 0 {
		return false, nil
	}
	var cfg config.Config
	if err := yaml.Unmarshal(existing, &cfg); err != nil {
		return false, fmt.Errorf("failed to parse existing config: %w", err)
	}
	for _, a := range cfg.Apps {
		if a.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// SetValue sets a value (scalar or structure) at keyPath in the YAML document,
// preserving comments and formatting, and returning the updated document bytes.
func SetValue(existing []byte, keyPath string, value interface{}) ([]byte, error) {
	doc, root, err := rootMapping(existing)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(keyPath, ".")
	curr := root
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		// Find key in mapping
		var valueNode *yaml.Node
		for j := 0; j+1 < len(curr.Content); j += 2 {
			if curr.Content[j].Value == part {
				valueNode = curr.Content[j+1]
				break
			}
		}

		if i == len(parts)-1 {
			// Leaf node: set or insert
			var newValNode *yaml.Node
			switch val := value.(type) {
			case string:
				newValNode = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: val}
			case bool:
				valStr := "false"
				if val {
					valStr = "true"
				}
				newValNode = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: valStr}
			case int:
				newValNode = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprintf("%d", val)}
			default:
				// Fallback to marshal
				var tempNode yaml.Node
				data, err := yaml.Marshal(val)
				if err != nil {
					return nil, err
				}
				if err := yaml.Unmarshal(data, &tempNode); err != nil {
					return nil, err
				}
				if len(tempNode.Content) > 0 {
					newValNode = tempNode.Content[0]
				} else {
					newValNode = &tempNode
				}
			}

			if valueNode != nil {
				// Retain comments
				newValNode.HeadComment = valueNode.HeadComment
				newValNode.LineComment = valueNode.LineComment
				newValNode.FootComment = valueNode.FootComment

				// Replace in mapping
				for j := 0; j+1 < len(curr.Content); j += 2 {
					if curr.Content[j].Value == part {
						curr.Content[j+1] = newValNode
						break
					}
				}
			} else {
				// Insert new key and value
				newKeyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: part}
				curr.Content = append(curr.Content, newKeyNode, newValNode)
			}
		} else {
			// Nested map node traversal
			if valueNode == nil {
				newKeyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: part}
				newValueNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				curr.Content = append(curr.Content, newKeyNode, newValueNode)
				curr = newValueNode
			} else if valueNode.Kind == yaml.MappingNode {
				curr = valueNode
			} else {
				return nil, fmt.Errorf("key path %q conflicts with existing non-map value at %q", keyPath, part)
			}
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("failed to render config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("failed to close yaml encoder: %w", err)
	}
	return buf.Bytes(), nil
}
