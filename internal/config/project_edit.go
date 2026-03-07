package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// UpsertProjectApp inserts or replaces one app entry in nimbu.yml.
func UpsertProjectApp(path string, app AppProjectConfig) error {
	root, err := readOrInitProjectNode(path)
	if err != nil {
		return err
	}

	body := ensureMappingNode(root)
	appsNode := ensureMapValue(body, "apps", yaml.SequenceNode)
	if appsNode.Kind != yaml.SequenceNode {
		appsNode.Kind = yaml.SequenceNode
		appsNode.Tag = "!!seq"
		appsNode.Content = nil
	}

	newNode, err := encodeNode(app)
	if err != nil {
		return err
	}
	quoteMappingValue(newNode, "glob")

	replaced := false
	for idx, item := range appsNode.Content {
		var existing AppProjectConfig
		if err := item.Decode(&existing); err != nil {
			continue
		}
		if existing.ID == app.ID && existing.Host == app.Host && existing.Site == app.Site {
			appsNode.Content[idx] = newNode
			replaced = true
			break
		}
	}
	if !replaced {
		appsNode.Content = append(appsNode.Content, newNode)
	}

	return writeProjectNode(path, root)
}

func readOrInitProjectNode(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			root := &yaml.Node{Kind: yaml.DocumentNode}
			root.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
			return root, nil
		}
		return nil, err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 {
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	return &root, nil
}

func ensureMappingNode(root *yaml.Node) *yaml.Node {
	if len(root.Content) == 0 {
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	body := root.Content[0]
	if body.Kind == 0 {
		body.Kind = yaml.MappingNode
		body.Tag = "!!map"
	}
	return body
}

func ensureMapValue(node *yaml.Node, key string, kind yaml.Kind) *yaml.Node {
	for idx := 0; idx+1 < len(node.Content); idx += 2 {
		if node.Content[idx].Value == key {
			return node.Content[idx+1]
		}
	}

	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: kind}
	switch kind {
	case yaml.MappingNode:
		valueNode.Tag = "!!map"
	case yaml.SequenceNode:
		valueNode.Tag = "!!seq"
	}
	node.Content = append(node.Content, keyNode, valueNode)
	return valueNode
}

func encodeNode(value any) (*yaml.Node, error) {
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return nil, err
	}
	return &node, nil
}

func quoteMappingValue(node *yaml.Node, key string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for idx := 0; idx+1 < len(node.Content); idx += 2 {
		if node.Content[idx].Value == key {
			node.Content[idx+1].Style = yaml.DoubleQuotedStyle
			return
		}
	}
}

func writeProjectNode(path string, root *yaml.Node) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}

	if buf.Len() == 0 {
		return fmt.Errorf("empty project config")
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
