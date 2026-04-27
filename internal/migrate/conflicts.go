package migrate

import (
	"context"
	"fmt"
)

// ExistingContentAction describes how a copy operation should handle target content that already exists.
type ExistingContentAction string

const (
	ExistingContentUpdate ExistingContentAction = "update"
	ExistingContentSkip   ExistingContentAction = "skip"
	ExistingContentReview ExistingContentAction = "review"
	ExistingContentAbort  ExistingContentAction = "abort"
)

// ExistingContentPrompt describes a type-level conflict decision.
type ExistingContentPrompt struct {
	Type          string
	Item          string
	Source        string
	Target        string
	SourceCount   int
	ExistingCount int
}

// ExistingContentDecision is returned by a type-level conflict resolver.
type ExistingContentDecision struct {
	Action     ExistingContentAction
	ApplyToAll bool
}

// ExistingContentResolver asks the caller how to handle existing target content.
type ExistingContentResolver func(context.Context, ExistingContentPrompt) (ExistingContentDecision, error)

type existingContentDecider struct {
	resolver ExistingContentResolver
	all      ExistingContentAction
	force    bool
}

func newExistingContentDecider(force bool, resolver ExistingContentResolver) *existingContentDecider {
	return &existingContentDecider{force: force, resolver: resolver}
}

func (d *existingContentDecider) decide(ctx context.Context, prompt ExistingContentPrompt) (ExistingContentAction, error) {
	if d == nil || d.force || d.resolver == nil {
		return ExistingContentUpdate, nil
	}
	if d.all != "" {
		return d.all, nil
	}
	decision, err := d.resolver(ctx, prompt)
	if err != nil {
		return "", err
	}
	action := normalizeExistingContentAction(decision.Action)
	if action == ExistingContentAbort {
		return action, fmt.Errorf("aborted")
	}
	if decision.ApplyToAll && action != ExistingContentReview {
		d.all = action
	}
	return action, nil
}

func normalizeExistingContentAction(action ExistingContentAction) ExistingContentAction {
	switch action {
	case ExistingContentSkip, ExistingContentReview, ExistingContentAbort:
		return action
	default:
		return ExistingContentUpdate
	}
}

func resolveExistingItem(ctx context.Context, resolver ExistingContentResolver, prompt ExistingContentPrompt, fallback ExistingContentAction, all *ExistingContentAction) (ExistingContentAction, error) {
	if fallback != ExistingContentReview {
		return fallback, nil
	}
	if all != nil && *all != "" {
		return *all, nil
	}
	if resolver == nil {
		return ExistingContentUpdate, nil
	}
	decision, err := resolver(ctx, prompt)
	if err != nil {
		return "", err
	}
	action := normalizeExistingContentAction(decision.Action)
	if action == ExistingContentReview {
		action = ExistingContentUpdate
	}
	if action == ExistingContentAbort {
		return action, fmt.Errorf("aborted")
	}
	if decision.ApplyToAll && all != nil {
		*all = action
	}
	return action, nil
}
