package gql

import (
	"context"
	"slices"

	"github.com/looplj/axonhub/internal/ent"
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

func listProbeEnabledDisplayModels(ctx context.Context, client *ent.Client) (map[string]struct{}, error) {
	models, err := client.Model.Query().
		Where(model.StatusEQ(model.StatusEnabled)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	keys := make(map[string]struct{})
	for _, modelEntity := range models {
		if modelEntity.Settings == nil || !modelEntity.Settings.ProbeEnabled {
			continue
		}
		keys[modelEntity.ModelID] = struct{}{}
	}

	return keys, nil
}

func filterDiscoveredSnapshots(
	snapshots []*ent.ModelHealthSnapshot,
	probeEnabledDisplayModels map[string]struct{},
) []*ent.ModelHealthSnapshot {
	return slices.DeleteFunc(snapshots, func(snapshot *ent.ModelHealthSnapshot) bool {
		_, exists := probeEnabledDisplayModels[snapshot.DisplayModel]
		return exists
	})
}
