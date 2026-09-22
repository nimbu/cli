package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/nimbu/cli/internal/api"
)

func resolveSchemaRenames(ctx context.Context, flags *RootFlags, client *api.Client, docs []schemaDocument, plans []schemaPlan, prune bool) ([]schemaPlan, error) {
	if flags.NoInput || flags.Readonly {
		return plans, nil
	}
	for i, plan := range plans {
		candidates := map[string][]string{}
		for _, op := range plan.Ops {
			if op["reason"] == "possible rename; declare renamed_from" {
				to, _ := op["to"].(string)
				from, _ := op["from"].(string)
				candidates[to] = append(candidates[to], from)
			}
		}
		changed := false
		for _, raw := range docs[i].Body["fields"].([]any) {
			field, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name, _ := field["name"].(string)
			sources := candidates[name]
			if len(sources) != 1 {
				continue
			}
			promptFlags := *flags
			promptFlags.Force = false
			yes, err := confirmPrompt(&promptFlags, fmt.Sprintf("declare %s renamed_from: %s in %s", name, sources[0], docs[i].Path))
			if err != nil {
				return nil, err
			}
			if !yes {
				plans[i].Ops = filterSchemaRenameCandidates(plans[i].Ops, sources[0])
				continue
			}
			if err = writeSchemaRename(docs[i].Path, name, sources[0]); err != nil {
				return nil, err
			}
			field["renamed_from"] = sources[0]
			changed = true
		}
		if changed {
			fresh, err := fetchSchemaPlan(ctx, client, docs[i], prune)
			if err != nil {
				return nil, err
			}
			plans[i] = fresh
		}
	}
	return plans, nil
}

func writeSchemaRename(path, name, from string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err = decoder.Decode(&document); err != nil {
		return err
	}
	var trailing yaml.Node
	if err = decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%s: expected a single YAML document", path)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s: schema must be a YAML mapping; plan again", path)
	}
	root := document.Content[0]
	var target *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "fields" {
			continue
		}
		fields := root.Content[i+1]
		if fields.Kind != yaml.SequenceNode {
			return fmt.Errorf("%s: fields must be a sequence to insert renamed_from", path)
		}
		for _, field := range fields.Content {
			if field.Kind != yaml.MappingNode {
				return fmt.Errorf("%s: fields must be mappings to insert renamed_from", path)
			}
			matches, hasRename := false, false
			for j := 0; j+1 < len(field.Content); j += 2 {
				if field.Content[j].Value == "name" && field.Content[j+1].Value == name {
					matches = true
				}
				if field.Content[j].Value == "renamed_from" {
					hasRename = true
				}
			}
			if matches {
				if target != nil || hasRename {
					return fmt.Errorf("%s: field %s has changed; plan again", path, name)
				}
				target = field
			}
		}
	}
	if target == nil {
		return fmt.Errorf("%s: field %s is missing; plan again", path, name)
	}
	target.Content = append(target.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "renamed_from"}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: from})
	data, err = yaml.Marshal(&document)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func filterSchemaRenameCandidates(ops []map[string]any, from string) []map[string]any {
	result := make([]map[string]any, 0, len(ops))
	for _, op := range ops {
		if op["reason"] == "possible rename; declare renamed_from" && op["from"] == from {
			continue
		}
		result = append(result, op)
	}
	return result
}
