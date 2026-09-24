package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

func (c *SchemaApplyCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := requireWrite(flags, "apply schemas"); err != nil {
		return err
	}
	client, docs, err := schemaCommandContext(ctx)
	if err != nil {
		return err
	}
	plans, err := planSchemaDocuments(ctx, client, docs, c.Prune)
	if err != nil {
		return err
	}
	c.Plans = plans
	if !c.DisableRenamePrompts {
		plans, err = resolveSchemaRenames(ctx, flags, client, docs, plans, c.Prune)
		if err != nil {
			return err
		}
	}
	inventory, err := schemaTargetDrift(ctx, client, docs)
	if err != nil {
		return err
	}
	c.Plans = append(append([]schemaPlan{}, plans...), inventory...)
	if err = renderSchemaPlans(ctx, c.Plans); err != nil {
		return err
	}
	if schemaPlanExit(plans) == 1 {
		return fmt.Errorf("schema plan is blocked; resolve the reported operations and plan again")
	}
	if err = confirmProtectedEnvironment(ctx, flags, c.Yes, "apply schemas"); err != nil {
		return err
	}
	if schemaPlanExit(plans) == 0 {
		return nil
	}
	if !c.Yes && !IsProtectedEnvironment(flags) {
		if flags.NoInput {
			return fmt.Errorf("use --yes to confirm schema changes with --no-input")
		}
		promptFlags := *flags
		promptFlags.Force = false
		ok, e := confirmPrompt(&promptFlags, "apply the planned schema changes")
		if e != nil {
			return e
		}
		if !ok {
			return fmt.Errorf("aborted")
		}
	}
	return applySchemaDocuments(ctx, client, docs, plans, c.Prune)
}

func applySchemaDocuments(ctx context.Context, client *api.Client, docs []schemaDocument, plans []schemaPlan, prune bool) error {
	completed := []string{}
	created, err := prepareSchemaCycles(ctx, client, docs, plans)
	if err != nil {
		return schemaApplyFailure(docs, 0, completed, created, fmt.Errorf("circular reference preparation failed: %w", err))
	}
	for i, doc := range docs {
		plan := plans[i]
		targetPrune := prune
		if created[i] != "" || schemaHasMissingReferences(plan) {
			var err error
			plan, err = fetchSchemaPlan(ctx, client, doc, targetPrune)
			if err != nil {
				return schemaApplyFailure(docs, i, completed, created, err)
			}
			expectedFingerprint := plans[i].Fingerprint
			if created[i] != "" {
				expectedFingerprint = created[i]
			}
			if plan.Fingerprint != expectedFingerprint {
				return schemaApplyFailure(docs, i, completed, created, fmt.Errorf("%s changed since approval", doc.Target))
			}
			if created[i] != "" {
				plan.Ops = filterSchemaRenameCandidates(plan.Ops, "nimbu_schema_placeholder")
			}
			if schemaPlanExit([]schemaPlan{plan}) == 1 {
				return schemaApplyFailure(docs, i, completed, created, fmt.Errorf("%s became blocked after dependency creation", doc.Target))
			}
		}
		if schemaHasMissingReferences(plan) {
			return schemaApplyFailure(docs, i, completed, created, fmt.Errorf("unresolved reference on %s; create the referenced target before applying", doc.Target))
		}
		if len(plan.Ops) == 0 {
			continue
		}
		body := schemaRequestBody(doc, targetPrune)
		body["fingerprint"] = plan.Fingerprint
		body["confirm_destructive"] = true
		var result map[string]any
		if err := client.Post(ctx, doc.Endpoint+"/apply", body, &result); err != nil {
			return schemaApplyFailure(docs, i, completed, created, err)
		}
		if result["applied"] != true {
			return schemaApplyFailure(docs, i, completed, created, fmt.Errorf("server returned no success confirmation"))
		}
		completed = append(completed, doc.Target)
		if created[i] != "" && !prune && !output.FromContext(ctx).JSON {
			if _, err := output.Fprintf(ctx, "%s retains temporary field nimbu_schema_placeholder; remove it with schema apply --prune after inspecting the plan.\n", doc.Target); err != nil {
				return err
			}
		}
	}
	if !output.FromContext(ctx).JSON {
		_, err := output.Fprintf(ctx, "Applied %d schema target(s).\n", len(completed))
		return err
	}
	return nil
}

func schemaHasMissingReferences(plan schemaPlan) bool {
	for _, warning := range plan.Warnings {
		if strings.HasPrefix(warning, "reference '") && strings.Contains(warning, "not found on target") {
			return true
		}
	}
	return false
}

func schemaApplyFailure(docs []schemaDocument, failed int, completed []string, created map[int]string, err error) error {
	pending := []string{}
	temporary := []string{}
	for i, doc := range docs {
		if i > failed {
			pending = append(pending, doc.Target)
		}
		if created[i] != "" {
			temporary = append(temporary, doc.Target)
		}
	}
	return fmt.Errorf("schema apply failed at %s: %w; completed: [%s]; unattempted: [%s]; temporary channels created: [%s]. The failed target may be partially changed; inspect and re-plan before retrying", docs[failed].Target, err, strings.Join(completed, ", "), strings.Join(pending, ", "), strings.Join(temporary, ", "))
}
