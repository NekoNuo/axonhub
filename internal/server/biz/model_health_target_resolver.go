package biz

import (
	"context"
	"fmt"
	"sort"

	"github.com/samber/lo"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/model"
)

const (
	ModelHealthTargetSourceRecent = "recent"
	ModelHealthTargetSourceManual = "manual"
)

type ModelHealthProbeTarget struct {
	DisplayModel string
	ActualModelID string
	ChannelID    int
	Sources      []string
}

type ModelHealthTargetResolver struct {
	client       *ent.Client
	modelService *ModelService
}

func NewModelHealthTargetResolver(client *ent.Client, modelService *ModelService) *ModelHealthTargetResolver {
	return &ModelHealthTargetResolver{
		client:       client,
		modelService: modelService,
	}
}

func (r *ModelHealthTargetResolver) Resolve(ctx context.Context, recent []ModelHealthRecentUsage) ([]ModelHealthProbeTarget, error) {
	targets := make(map[ChannelModelKey]*ModelHealthProbeTarget)

	for _, usage := range recent {
		for _, actual := range usage.ActualModels {
			for _, channelID := range actual.ChannelIDs {
				key := ChannelModelKey{ChannelID: channelID, ModelID: actual.ActualModelID}
				targets[key] = mergeModelHealthTarget(targets[key], ModelHealthProbeTarget{
					DisplayModel: usage.DisplayModel,
					ActualModelID: actual.ActualModelID,
					ChannelID:    channelID,
					Sources:      []string{ModelHealthTargetSourceRecent},
				})
			}
		}
	}

	manualModels, err := r.client.Model.Query().
		Where(model.StatusEQ(model.StatusEnabled)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query manual probe-enabled models: %w", err)
	}

	for _, m := range manualModels {
		if m.Settings == nil || !m.Settings.ProbeEnabled {
			continue
		}

		connections, connErr := r.modelService.QueryModelChannelConnections(ctx, m.Settings.Associations)
		if connErr != nil {
			return nil, fmt.Errorf("resolve model associations for %s: %w", m.ModelID, connErr)
		}

		for _, conn := range connections {
			for _, modelEntry := range conn.Models {
				key := ChannelModelKey{ChannelID: conn.Channel.ID, ModelID: modelEntry.ActualModel}
				targets[key] = mergeModelHealthTarget(targets[key], ModelHealthProbeTarget{
					DisplayModel: m.ModelID,
					ActualModelID: modelEntry.ActualModel,
					ChannelID:    conn.Channel.ID,
					Sources:      []string{ModelHealthTargetSourceManual},
				})
			}
		}
	}

	resultPtrs := lo.Values(targets)
	sort.Slice(resultPtrs, func(i, j int) bool {
		if resultPtrs[i].DisplayModel != resultPtrs[j].DisplayModel {
			return resultPtrs[i].DisplayModel < resultPtrs[j].DisplayModel
		}
		if resultPtrs[i].ActualModelID != resultPtrs[j].ActualModelID {
			return resultPtrs[i].ActualModelID < resultPtrs[j].ActualModelID
		}
		return resultPtrs[i].ChannelID < resultPtrs[j].ChannelID
	})

	result := lo.Map(resultPtrs, func(item *ModelHealthProbeTarget, _ int) ModelHealthProbeTarget {
		return *item
	})

	return result, nil
}

func mergeModelHealthTarget(existing *ModelHealthProbeTarget, incoming ModelHealthProbeTarget) *ModelHealthProbeTarget {
	if existing == nil {
		incoming.Sources = sortUniqueStrings(incoming.Sources)
		return &incoming
	}

	if existing.DisplayModel == "" {
		existing.DisplayModel = incoming.DisplayModel
	}
	existing.Sources = sortUniqueStrings(append(existing.Sources, incoming.Sources...))

	return existing
}

func sortUniqueStrings(values []string) []string {
	unique := lo.Uniq(values)
	sort.Strings(unique)
	return unique
}
