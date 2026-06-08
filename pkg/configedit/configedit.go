// Package configedit performs comment-preserving edits of aetherpak.yaml by
// operating on the yaml.v3 node tree rather than re-marshaling the whole struct.
package configedit

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aetherpak/aetherpak/pkg/config"
	"gopkg.in/yaml.v3"
)

// keyKind describes the expected type for a settable config key.
type keyKind int

const (
	kindString keyKind = iota
	kindBool
	kindInt
	kindStringSlice
)

// keyInfo describes a single settable configuration field.
type keyInfo struct {
	kind keyKind
}

// settableKeys is the registry of config keys that `config set` can write.
// Keys that require structured input (apps, defaults.remotes, defaults.flatpaks)
// are intentionally excluded — they need the `add` workflow or manual editing.
var settableKeys = map[string]keyInfo{
	// Top-level scalar fields
	"registry":       {kind: kindString},
	"pages_url":      {kind: kindString},
	"oci_repository": {kind: kindString},
	"remote_name":    {kind: kindString},
	"no_sign":        {kind: kindBool},
	"repo_title":     {kind: kindString},
	"repo_homepage":  {kind: kindString},
	"runtime_repo":   {kind: kindString},
	"output_dir":     {kind: kindString},

	// Linter
	"linter.strict":          {kind: kindBool},
	"linter.ignore_rules":    {kind: kindStringSlice},
	"linter.exceptions":      {kind: kindStringSlice},
	"linter.exceptions_file": {kind: kindString},

	// Branding
	"branding.logo_url":       {kind: kindString},
	"branding.favicon_url":    {kind: kindString},
	"branding.accent_color":   {kind: kindString},
	"branding.footer_text":    {kind: kindString},
	"branding.index_template": {kind: kindString},

	// Defaults
	"defaults.ccache":       {kind: kindBool},
	"defaults.ccache_dir":   {kind: kindString},
	"defaults.state_dir":    {kind: kindString},
	"defaults.run_linter":   {kind: kindBool},
	"defaults.builder_args": {kind: kindStringSlice},

	// Channel mappings (dynamic sub-keys)
	// Handled specially: any "channel_mappings.<pattern>" is valid.
}

// ValidConfigKeys returns a sorted list of all settable configuration key
// paths. Keys under channel_mappings are not included since they accept
// arbitrary sub-keys.
func ValidConfigKeys() []string {
	keys := make([]string, 0, len(settableKeys))
	for k := range settableKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// lookupKey resolves a dot-separated key path against the settable key
// registry. It also handles dynamic channel_mappings.<pattern> keys.
func lookupKey(key string) (keyInfo, bool) {
	if info, ok := settableKeys[key]; ok {
		return info, true
	}
	// Support channel_mappings.<pattern> as a string value.
	if strings.HasPrefix(key, "channel_mappings.") && len(key) > len("channel_mappings.") {
		return keyInfo{kind: kindString}, true
	}
	return keyInfo{}, false
}

// parseValue converts a raw string (or multiple strings for list types) into
// the Go value matching the key's expected type. It returns the parsed value
// and a YAML tag string for the node.
func parseValue(info keyInfo, values []string) (interface{}, string, error) {
	if len(values) == 0 {
		return nil, "", fmt.Errorf("no value provided")
	}

	switch info.kind {
	case kindBool:
		if len(values) != 1 {
			return nil, "", fmt.Errorf("boolean key expects exactly one value (true or false)")
		}
		switch strings.ToLower(values[0]) {
		case "true":
			return true, "!!bool", nil
		case "false":
			return false, "!!bool", nil
		default:
			return nil, "", fmt.Errorf("invalid boolean value %q (must be true or false)", values[0])
		}

	case kindInt:
		if len(values) != 1 {
			return nil, "", fmt.Errorf("integer key expects exactly one value")
		}
		n, err := strconv.Atoi(values[0])
		if err != nil {
			return nil, "", fmt.Errorf("invalid integer value %q: %w", values[0], err)
		}
		return n, "!!int", nil

	case kindStringSlice:
		// Accept either comma-separated in a single arg or multiple args.
		var items []string
		for _, v := range values {
			for _, part := range strings.Split(v, ",") {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					items = append(items, trimmed)
				}
			}
		}
		return items, "!!seq", nil

	default: // kindString
		if len(values) != 1 {
			return nil, "", fmt.Errorf("string key expects exactly one value")
		}
		return values[0], "!!str", nil
	}
}

// SetValue performs a comment-preserving edit of a single configuration key
// in the YAML config bytes. It validates the key against the known schema,
// parses the value to the correct type, and edits the yaml.v3 node tree
// in-place to preserve comments, ordering, and formatting.
//
// values supports both single and multiple value arguments; for list-typed
// keys, values are also split on commas.
func SetValue(existing []byte, key string, values []string) ([]byte, error) {
	info, ok := lookupKey(key)
	if !ok {
		known := ValidConfigKeys()
		return nil, fmt.Errorf("unknown configuration key %q; valid keys are:\n  %s\n  channel_mappings.<pattern>",
			key, strings.Join(known, "\n  "))
	}

	parsed, tag, err := parseValue(info, values)
	if err != nil {
		return nil, fmt.Errorf("invalid value for %q: %w", key, err)
	}

	doc, root, err := rootMapping(existing)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(key, ".")
	if err := setNodeValue(root, parts, parsed, tag); err != nil {
		return nil, err
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

// setNodeValue walks the dot-separated key path through mapping nodes,
// creating intermediate mappings as needed, and sets the leaf value.
func setNodeValue(mapping *yaml.Node, parts []string, value interface{}, tag string) error {
	curr := mapping
	for i := 0; i < len(parts)-1; i++ {
		child := findValue(curr, parts[i])
		if child == nil {
			// Create intermediate mapping node.
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: parts[i]}
			child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			curr.Content = append(curr.Content, keyNode, child)
		} else if isNullNode(child) {
			// Convert null to mapping.
			child.Kind = yaml.MappingNode
			child.Tag = "!!map"
			child.Value = ""
		} else if child.Kind != yaml.MappingNode {
			return fmt.Errorf("key %q is not a mapping (cannot set nested key under it)", parts[i])
		}
		curr = child
	}

	leaf := parts[len(parts)-1]
	existing := findValue(curr, leaf)

	if tag == "!!seq" {
		// List value: build a sequence node.
		items, ok := value.([]string)
		if !ok {
			return fmt.Errorf("internal error: expected []string for sequence value")
		}
		seqNode := &yaml.Node{
			Kind:  yaml.SequenceNode,
			Tag:   "!!seq",
			Style: yaml.FlowStyle,
		}
		for _, item := range items {
			seqNode.Content = append(seqNode.Content, &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: item,
			})
		}
		if existing != nil {
			// Replace the existing node's content in-place.
			existing.Kind = yaml.SequenceNode
			existing.Tag = "!!seq"
			existing.Value = ""
			existing.Style = yaml.FlowStyle
			existing.Content = seqNode.Content
		} else {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: leaf}
			curr.Content = append(curr.Content, keyNode, seqNode)
		}
		return nil
	}

	// Scalar value.
	valStr := fmt.Sprintf("%v", value)
	if existing != nil {
		existing.Kind = yaml.ScalarNode
		existing.Tag = tag
		existing.Value = valStr
		existing.Content = nil
	} else {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: leaf}
		valNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: valStr}
		curr.Content = append(curr.Content, keyNode, valNode)
	}
	return nil
}

// appOut is the minimal projection of an App that `add` writes. Field order
// here is the emitted key order; omitempty keeps the entry tidy.
type appOut struct {
	ID          string                   `yaml:"id"`
	Branch      string                   `yaml:"branch,omitempty"`
	Arches      []string                 `yaml:"arches,omitempty"`
	Manifest    string                   `yaml:"manifest,omitempty"`
	RunLinter   bool                     `yaml:"run-linter,omitempty"`
	CCache      *bool                    `yaml:"ccache,omitempty"`
	BuilderArgs []string                 `yaml:"builder_args,omitempty"`
	Bundles     map[string]config.Bundle `yaml:"bundles,omitempty"`
}

// appNode marshals an App into a standalone YAML mapping node.
func appNode(app config.App) (*yaml.Node, error) {
	out := appOut{
		ID:          app.ID,
		Branch:      app.Branch,
		Arches:      app.Arches,
		Manifest:    app.Manifest,
		RunLinter:   app.RunLinter,
		CCache:      app.CCache,
		BuilderArgs: app.BuilderArgs,
		Bundles:     app.Bundles,
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
