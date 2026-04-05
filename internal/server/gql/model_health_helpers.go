package gql

import (
	"context"
	"slices"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/server/biz"
)

func buildModelHealthSnapshotRows(
	ctx context.Context,
	client *ent.Client,
	snapshots []*ent.ModelHealthSnapshot,
) ([]*ent.ModelHealthSnapshot, error) {
	rows := make([]*ent.ModelHealthSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		view, err := biz.GetEffectiveModelHealthFromDBForResolver(
			ctx,
			client,
			snapshot.DisplayModel,
			snapshot.ChannelID,
			snapshot.ActualModelID,
		)
		if err != nil {
			return nil, err
		}
		if view == nil {
			continue
		}

		rows = append(rows, &ent.ModelHealthSnapshot{
			ID:             snapshot.ID,
			DisplayModel:   view.DisplayModel,
			ChannelID:      view.ChannelID,
			ActualModelID:  view.ActualModelID,
			Source:         view.Source,
			IsHealthy:      view.IsHealthy,
			ManualOverride: view.ManualOverride,
			ProbedAt:       view.ProbedAt,
		})
	}

	slices.SortFunc(rows, func(a, b *ent.ModelHealthSnapshot) int {
		if cmp := compareStrings(a.DisplayModel, b.DisplayModel); cmp != 0 {
			return cmp
		}
		if cmp := compareStrings(a.ActualModelID, b.ActualModelID); cmp != 0 {
			return cmp
		}
		switch {
		case a.ChannelID < b.ChannelID:
			return -1
		case a.ChannelID > b.ChannelID:
			return 1
		default:
			return 0
		}
	})

	return rows, nil
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func listProbeEnabledAssociatedActualModels(ctx context.Context, client *ent.Client) (map[biz.ChannelModelKey]struct{}, error) {
	models, err := client.Model.Query().
		Where(model.StatusEQ(model.StatusEnabled)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	channelEntities, err := client.Channel.Query().
		Where(channel.StatusEQ(channel.StatusEnabled)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	channels := make([]*biz.Channel, 0, len(channelEntities))
	for _, channelEntity := range channelEntities {
		channels = append(channels, &biz.Channel{Channel: channelEntity})
	}

	keys := make(map[biz.ChannelModelKey]struct{})
	for _, modelEntity := range models {
		if modelEntity.Settings == nil || !modelEntity.Settings.ProbeEnabled {
			continue
		}

		connections := biz.MatchAssociations(modelEntity.Settings.Associations, channels)
		for _, connection := range connections {
			for _, matchedModel := range connection.Models {
				keys[biz.ChannelModelKey{
					ChannelID: connection.Channel.ID,
					ModelID:   matchedModel.ActualModel,
				}] = struct{}{}
			}
		}
	}

	return keys, nil
}

func filterDiscoveredSnapshots(
	snapshots []*ent.ModelHealthSnapshot,
	probeEnabledAssociatedActualModels map[biz.ChannelModelKey]struct{},
) []*ent.ModelHealthSnapshot {
	return slices.DeleteFunc(snapshots, func(snapshot *ent.ModelHealthSnapshot) bool {
		_, exists := probeEnabledAssociatedActualModels[biz.ChannelModelKey{
			ChannelID: snapshot.ChannelID,
			ModelID:   snapshot.ActualModelID,
		}]
		return exists
	})
}
