package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// EnvironmentConfig selects a site and API host together.
type EnvironmentConfig struct {
	Host      string `json:"host" yaml:"host"`
	Site      string `json:"site" yaml:"site"`
	Protected bool   `json:"protected,omitempty" yaml:"protected,omitempty"`
}

func (e EnvironmentConfig) Validate() error {
	if strings.TrimSpace(e.Host) == "" || strings.TrimSpace(e.Site) == "" {
		return fmt.Errorf("environment requires host and site")
	}
	return nil
}

// UpdateEnvironment edits only the selected mapping, retaining unrelated YAML keys and comments.
// A nil value removes the environment.
func UpdateEnvironment(path, name string, value *EnvironmentConfig) error {
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "/\\") || strings.ContainsFunc(name, unicode.IsSpace) {
		return fmt.Errorf("environment name must be nonempty and contain no whitespace or slash")
	}
	if value != nil {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&doc); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("project config must contain a single YAML document")
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("project config must be a YAML mapping")
	}
	root := doc.Content[0]
	var envs *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "environments" {
			envs = root.Content[i+1]
			break
		}
	}
	if envs == nil {
		if value == nil {
			return fmt.Errorf("environment %q not found", name)
		}
		envs = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "environments"}, envs)
	}
	if envs.Kind != yaml.MappingNode {
		return fmt.Errorf("environments must be a YAML mapping")
	}
	index := -1
	for i := 0; i < len(envs.Content); i += 2 {
		if envs.Content[i].Value == name {
			index = i
			break
		}
	}
	if value == nil {
		if index < 0 {
			return fmt.Errorf("environment %q not found", name)
		}
		envs.Content = append(envs.Content[:index], envs.Content[index+2:]...)
	} else {
		if index >= 0 {
			return fmt.Errorf("environment %q already exists; remove it first to replace it", name)
		}
		var node yaml.Node
		if err := node.Encode(value); err != nil {
			return err
		}
		envs.Content = append(envs.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, &node)
	}
	result, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, result, 0o644)
}

// WarnUnknownEnvironmentKeys returns warnings for unknown environment settings.
func WarnUnknownEnvironmentKeys(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	envs, ok := asStringMap(root["environments"])
	if !ok {
		return nil, nil
	}
	var warnings []string
	for name, raw := range envs {
		if env, ok := asStringMap(raw); ok {
			warnings = appendUnknownMapKeys(warnings, "environments."+name, env, map[string]struct{}{"host": {}, "site": {}, "protected": {}})
		}
	}
	sort.Strings(warnings)
	return warnings, nil
}
