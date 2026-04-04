package biz

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/samber/lo"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/ent/usagelog"
)

type ModelHealthRecentUsage struct {
	DisplayModel string
	ActualModels []ModelHealthRecentActualModel
}

type ModelHealthRecentActualModel struct {
	ActualModelID string
	ChannelIDs    []int
}

type ModelHealthRecentUsageAggregator struct {
	client *ent.Client
	window time.Duration
}

func NewModelHealthRecentUsageAggregator(client *ent.Client) *ModelHealthRecentUsageAggregator {
	return &ModelHealthRecentUsageAggregator{
		client: client,
		window: time.Hour,
	}
}

func (a *ModelHealthRecentUsageAggregator) Aggregate(ctx context.Context, now time.Time) ([]ModelHealthRecentUsage, error) {
	since := now.Add(-a.window)

	logs, err := a.client.UsageLog.Query().
		Where(usagelog.CreatedAtGTE(since)).
		WithRequest(func(q *ent.RequestQuery) {
			q.Select(request.FieldID, request.FieldModelID)
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query recent usage logs: %w", err)
	}

	displayMap := make(map[string]map[string]map[int]struct{})

	for _, logItem := range logs {
		if logItem.Edges.Request == nil || logItem.Edges.Request.ModelID == "" {
			continue
		}

		displayModel := logItem.Edges.Request.ModelID
		actualModel := logItem.ModelID

		actualMap, ok := displayMap[displayModel]
		if !ok {
			actualMap = make(map[string]map[int]struct{})
			displayMap[displayModel] = actualMap
		}

		channelSet, ok := actualMap[actualModel]
		if !ok {
			channelSet = make(map[int]struct{})
			actualMap[actualModel] = channelSet
		}

		channelSet[logItem.ChannelID] = struct{}{}
	}

	result := make([]ModelHealthRecentUsage, 0, len(displayMap))
	for displayModel, actualMap := range displayMap {
		actualModels := lo.Map(lo.Keys(actualMap), func(actualModel string, _ int) ModelHealthRecentActualModel {
			channelIDs := lo.Keys(actualMap[actualModel])
			sort.Ints(channelIDs)

			return ModelHealthRecentActualModel{
				ActualModelID: actualModel,
				ChannelIDs:    channelIDs,
			}
		})

		sort.Slice(actualModels, func(i, j int) bool {
			return actualModels[i].ActualModelID < actualModels[j].ActualModelID
		})

		result = append(result, ModelHealthRecentUsage{
			DisplayModel: displayModel,
			ActualModels: actualModels,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].DisplayModel < result[j].DisplayModel
	})

	return result, nil
}
