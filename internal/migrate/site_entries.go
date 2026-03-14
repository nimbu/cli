package migrate

import (
	"context"
	"fmt"
	"sort"

	"github.com/nimbu/cli/internal/api"
)

// CopySiteEntries copies entries across multiple channels using a shared ID mapping,
// dependency-aware ordering, and deferred resolution for circular references.
func CopySiteEntries(ctx context.Context, fromClient, toClient *api.Client, fromRef, toRef SiteRef,
	channels []string, opts RecordCopyOptions,
) ([]RecordCopyResult, error) {
	allChannels, err := api.ListChannelDetails(ctx, fromClient)
	if err != nil {
		return nil, err
	}
	channelMap := make(map[string]api.ChannelDetail, len(allChannels))
	for _, ch := range allChannels {
		channelMap[ch.Slug] = ch
	}

	graph := api.BuildChannelDependencyGraph(allChannels)
	ordered := topoSortEntryChannels(channels, graph)

	copier := &recordCopier{
		fromClient: fromClient,
		toClient:   toClient,
		options:    opts,
		mapping:    map[string]map[string]string{},
		queued:     map[string]map[string]struct{}{},
		channelMap: channelMap,
	}

	var results []RecordCopyResult
	total := len(ordered)
	for i, channel := range ordered {
		emitStageItem(ctx, "Channel Entries", channel, int64(i+1), int64(total))
		if _, ok := channelMap[channel]; !ok {
			emitWarning(ctx, fmt.Sprintf("unknown channel %q, skipping", channel))
			continue
		}
		result := RecordCopyResult{From: fromRef, To: toRef, Resource: channel}
		copier.result = &result
		detail := channelMap[channel]
		info := buildSchemaInfo(channel, detail.Customizations)
		records, err := copier.listRecords(ctx, channel, nil)
		if err != nil {
			return results, err
		}
		copier.preMatchEntries(ctx, channel, channel, records, info)
		_, warnings, err := copier.copyRecords(ctx, channel, channel, info, records)
		result.Warnings = warnings
		results = append(results, result)
		if err != nil {
			return results, err
		}
		synced, skipped := countActions(result.Items)
		summary := fmt.Sprintf("%d synced", synced)
		if skipped > 0 {
			summary += fmt.Sprintf(", %d unchanged", skipped)
		}
		emitSubStageDone(ctx, "Channel Entries", channel, summary)
	}

	if len(copier.deferredRefs) > 0 {
		emitStageItem(ctx, "Channel Entries", "resolving cross-references", 0, 0)
		warnings, err := copier.resolveDeferredRefs(ctx)
		if len(results) > 0 {
			results[len(results)-1].Warnings = append(results[len(results)-1].Warnings, warnings...)
		}
		if err != nil {
			return results, err
		}
		emitSubStageDone(ctx, "Channel Entries", "cross-references", fmt.Sprintf("%d resolved", len(copier.deferredRefs)))
	}

	// Report unresolved references that couldn't be mapped during copy.
	if unresolved := copier.UnresolvedWarnings(); len(unresolved) > 0 {
		emitStageStart(ctx, "Unresolved References")
		if len(results) > 0 {
			results[len(results)-1].Warnings = append(results[len(results)-1].Warnings, unresolved...)
		}
		emitStageDone(ctx, "Unresolved References", fmt.Sprintf("%d fields need manual update", len(unresolved)))
		for _, w := range unresolved {
			emitWarning(ctx, w)
		}
	}

	// Validate copied entries against source.
	emitStageStart(ctx, "Validation")
	validationWarnings := ValidateEntries(ctx, fromClient, toClient, copier.mapping, channelMap)
	if len(results) > 0 {
		results[len(results)-1].Warnings = append(results[len(results)-1].Warnings, validationWarnings...)
	}
	emitStageDone(ctx, "Validation", fmt.Sprintf("%d issues", len(validationWarnings)))
	for _, w := range validationWarnings {
		emitWarning(ctx, w)
	}

	return results, nil
}

// topoSortEntryChannels sorts the given channel slugs by dependency order,
// only including channels from the provided list.
func topoSortEntryChannels(channels []string, graph api.ChannelDependencyGraph) []string {
	include := make(map[string]bool, len(channels))
	for _, ch := range channels {
		include[ch] = true
	}
	var out []string
	seen := map[string]bool{}
	var visit func(string)
	visit = func(slug string) {
		if seen[slug] || !include[slug] {
			return
		}
		seen[slug] = true
		for _, dep := range graph.DirectDependencies(slug) {
			visit(dep)
		}
		out = append(out, slug)
	}
	sorted := make([]string, len(channels))
	copy(sorted, channels)
	sort.Strings(sorted)
	for _, ch := range sorted {
		visit(ch)
	}
	return out
}
