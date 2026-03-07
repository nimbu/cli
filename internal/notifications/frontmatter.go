package notifications

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type frontMatterField struct {
	Key   string
	Value string
}

func parseFrontMatter(data []byte) (map[string]any, string, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return map[string]any{}, text, nil
	}

	rest := text[len("---\n"):]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return nil, "", fmt.Errorf("parse front matter: missing terminator")
	}
	attrs := map[string]any{}
	payload := rest[:idx]
	if strings.TrimSpace(payload) != "" {
		if err := yaml.Unmarshal([]byte(payload), &attrs); err != nil {
			return nil, "", fmt.Errorf("parse front matter: %w", err)
		}
	}
	body := rest[idx+len("\n---\n"):]
	body = strings.TrimPrefix(body, "\n")
	return attrs, body, nil
}

func encodeFrontMatter(fields []frontMatterField, body string) ([]byte, error) {
	var node yaml.Node
	node.Kind = yaml.MappingNode
	node.Tag = "!!map"
	for _, field := range fields {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: field.Key}
		valueNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: field.Value}
		node.Content = append(node.Content, keyNode, valueNode)
	}

	payload, err := yaml.Marshal(&node)
	if err != nil {
		return nil, fmt.Errorf("encode front matter: %w", err)
	}

	var builder strings.Builder
	builder.WriteString("---\n")
	builder.Write(payload)
	builder.WriteString("---\n\n")
	builder.WriteString(body)
	return []byte(builder.String()), nil
}
