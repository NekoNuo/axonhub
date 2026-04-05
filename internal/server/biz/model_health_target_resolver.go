package biz

import (
	"context"
	"fmt"
	"sort"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/model"
)

const (
	ModelHealthTargetSourceRecent = "recent"
	ModelHealthTargetSourceManual = "manual"

	ModelHealthSourceAssociated            = "associated"
	ModelHealthSourceDiscoveredRecentUsage = "discovered_recent_usage"
	modelHealthDiscoveredRetention         = 24 * 60 * 60
)

type ModelHealthProbeTarget struct {
	DisplayModel  string
	ActualModelID string
	ChannelID     int
	Sources       []string
	Source        string
}

type ModelHealthTargetResolver struct {
	client *ent.Client
}

func ModelHealthDiscoveredRetentionForResolver() int64 {
	return modelHealthDiscoveredRetention
}

func NewModelHealthTargetResolver(client *ent.Client, _ any) *ModelHealthTargetResolver {
	return &ModelHealthTargetResolver{client: client}
}

func (r *ModelHealthTargetResolver) Resolve(ctx context.Context, recent []ModelHealthRecentUsage) ([]ModelHealthProbeTarget, error) {
	models, err := r.client.Model.Query().
		Where(model.StatusEQ(model.StatusEnabled)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	associatedByDisplayModel := make(map[string]*ent.Model, len(models))
	for _, item := range models {
		associatedByDisplayModel[item.ModelID] = item
	}

	channels, err := r.client.Channel.Query().
		Where(channel.StatusEQ(channel.StatusEnabled)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	targets := make([]ModelHealthProbeTarget, 0)
	seen := make(map[string]struct{})

	for _, usage := range recent {
		if modelEntry, ok := associatedByDisplayModel[usage.DisplayModel]; ok && modelEntry.Settings != nil && modelEntry.Settings.ProbeEnabled {
			assocTargets := resolveAssociatedModelHealthTargets(modelEntry, channels)
			for _, target := range assocTargets {
				key := modelHealthTargetKey(target)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				targets = append(targets, target)
			}
			continue
		}

		discoveredTargets := resolveDiscoveredModelHealthTargets(usage, channels)
		for _, target := range discoveredTargets {
			key := modelHealthTargetKey(target)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, target)
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		if targets[i].DisplayModel != targets[j].DisplayModel {
			return targets[i].DisplayModel < targets[j].DisplayModel
		}
		if targets[i].ActualModelID != targets[j].ActualModelID {
			return targets[i].ActualModelID < targets[j].ActualModelID
		}
		return targets[i].ChannelID < targets[j].ChannelID
	})

	return targets, nil
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

func resolveAssociatedModelHealthTargets(item *ent.Model, channelEntities []*ent.Channel) []ModelHealthProbeTarget {
	channels := make([]*Channel, 0, len(channelEntities))
	for _, channelEntity := range channelEntities {
		channels = append(channels, &Channel{Channel: channelEntity})
	}

	connections := MatchAssociations(item.Settings.Associations, channels)
	targets := make([]ModelHealthProbeTarget, 0)

	for _, connection := range connections {
		for _, matchedModel := range connection.Models {
			targets = append(targets, ModelHealthProbeTarget{
				DisplayModel:  item.ModelID,
				ActualModelID: matchedModel.ActualModel,
				ChannelID:     connection.Channel.ID,
				Sources:       []string{ModelHealthTargetSourceRecent},
				Source:        ModelHealthSourceAssociated,
			})
		}
	}

	return targets
}

func resolveDiscoveredModelHealthTargets(usage ModelHealthRecentUsage, channelEntities []*ent.Channel) []ModelHealthProbeTarget {
	recentActualModels := make(map[ChannelModelKey]struct{})
	for _, actual := range usage.ActualModels {
		for _, channelID := range actual.ChannelIDs {
			recentActualModels[ChannelModelKey{
				ChannelID: channelID,
				ModelID:   actual.ActualModelID,
			}] = struct{}{}
		}
	}

	targets := make([]ModelHealthProbeTarget, 0)
	for _, channelEntity := range channelEntities {
		channelWrapper := &Channel{Channel: channelEntity}

		if entry, ok := channelWrapper.GetModelEntries()[usage.DisplayModel]; ok {
			targets = append(targets, ModelHealthProbeTarget{
				DisplayModel:  usage.DisplayModel,
				ActualModelID: entry.ActualModel,
				ChannelID:     channelEntity.ID,
				Sources:       []string{ModelHealthTargetSourceRecent},
				Source:        ModelHealthSourceDiscoveredRecentUsage,
			})
		}

		for _, entry := range channelWrapper.GetModelEntries() {
			if _, ok := recentActualModels[ChannelModelKey{
				ChannelID: channelEntity.ID,
				ModelID:   entry.ActualModel,
			}]; !ok {
				continue
			}

			targets = append(targets, ModelHealthProbeTarget{
				DisplayModel:  usage.DisplayModel,
				ActualModelID: entry.ActualModel,
				ChannelID:     channelEntity.ID,
				Sources:       []string{ModelHealthTargetSourceRecent},
				Source:        ModelHealthSourceDiscoveredRecentUsage,
			})
		}
	}

	return targets
}

func modelHealthTargetKey(target ModelHealthProbeTarget) string {
	return fmt.Sprintf("%s:%s:%d:%s", target.Source, target.DisplayModel, target.ChannelID, target.ActualModelID)
}

func sortUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}
