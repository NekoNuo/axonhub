package gql

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/usagelog"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/internal/server/orchestrator"
	"github.com/looplj/axonhub/llm"
)

type loadBalancerPreviewSnapshot struct {
	ModelID    string
	Strategy   string
	Summary    loadBalancerPreviewSummary
	Candidates []*loadBalancerPreviewCandidate
	Steps      []*loadBalancerPreviewStep
}

type loadBalancerPreviewSummary struct {
	PrimaryChannelName    string
	FirstRetryChannelName string
	FallbackChannelName   string
}

type loadBalancerPreviewCandidate struct {
	ChannelName string
}

type loadBalancerPreviewStep struct {
	Attempt            int
	ChannelName        string
	WaitMSAfterFailure int
}

func buildLoadBalancerPreview(ctx context.Context, r *Resolver) (*loadBalancerPreviewSnapshot, error) {
	if r == nil || r.client == nil || r.systemService == nil || r.channelService == nil || r.modelService == nil {
		return nil, nil
	}

	ctx = authz.WithSystemBypass(ctx, "dashboard-load-balancer-preview")

	modelID, err := hottestRecentModel(ctx, r.client, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	if modelID == "" {
		return nil, nil
	}

	retryPolicy := r.systemService.RetryPolicyOrDefault(ctx)
	strategy := derivePreviewStrategy(retryPolicy)

	selector := orchestrator.NewDefaultSelector(r.channelService, r.modelService, r.systemService)
	candidates, err := selector.Select(ctx, &llm.Request{Model: modelID})
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		candidates, err = selectChannelsDirectly(ctx, r.channelService, modelID)
		if err != nil {
			return nil, err
		}
	}
	if len(candidates) == 0 {
		return &loadBalancerPreviewSnapshot{
			ModelID:    modelID,
			Strategy:   strategy,
			Candidates: []*loadBalancerPreviewCandidate{},
			Steps:      []*loadBalancerPreviewStep{},
		}, nil
	}

	loadBalancer := newPreviewLoadBalancer(r, strategy)
	sorted := loadBalancer.Sort(ctx, candidates, modelID)

	preview := &loadBalancerPreviewSnapshot{
		ModelID:    modelID,
		Strategy:   strategy,
		Candidates: make([]*loadBalancerPreviewCandidate, 0, len(sorted)),
		Steps:      make([]*loadBalancerPreviewStep, 0, len(sorted)),
	}

	for idx, candidate := range sorted {
		preview.Candidates = append(preview.Candidates, &loadBalancerPreviewCandidate{
			ChannelName: candidate.Channel.Name,
		})

		step := &loadBalancerPreviewStep{
			Attempt:            idx + 1,
			ChannelName:        candidate.Channel.Name,
			WaitMSAfterFailure: retryPolicy.RetryDelayMs,
		}
		if idx == len(sorted)-1 {
			step.WaitMSAfterFailure = 0
		}
		preview.Steps = append(preview.Steps, step)
	}

	if len(sorted) > 0 {
		preview.Summary.PrimaryChannelName = sorted[0].Channel.Name
	}
	if len(sorted) > 1 {
		preview.Summary.FirstRetryChannelName = sorted[1].Channel.Name
	}
	if len(sorted) > 2 {
		preview.Summary.FallbackChannelName = sorted[len(sorted)-1].Channel.Name
	}

	return preview, nil
}

func hottestRecentModel(ctx context.Context, client *ent.Client, window time.Duration) (string, error) {
	type modelUsage struct {
		ModelID string `json:"model_id"`
		Count   int    `json:"count"`
	}

	var results []modelUsage
	since := time.Now().Add(-window)
	err := client.UsageLog.Query().
		Where(usagelog.CreatedAtGTE(since)).
		GroupBy(usagelog.FieldModelID).
		Aggregate(ent.Count()).
		Scan(ctx, &results)
	if err != nil {
		return "", fmt.Errorf("query hottest recent model: %w", err)
	}
	if len(results) == 0 {
		return "", nil
	}

	top := lo.MaxBy(results, func(a, b modelUsage) bool {
		if a.Count == b.Count {
			return a.ModelID < b.ModelID
		}
		return a.Count > b.Count
	})

	return top.ModelID, nil
}

func selectChannelsDirectly(ctx context.Context, channelService *biz.ChannelService, modelID string) ([]*orchestrator.ChannelModelsCandidate, error) {
	channels := channelService.GetEnabledChannels()
	candidates := make([]*orchestrator.ChannelModelsCandidate, 0, len(channels))
	for _, ch := range channels {
		entries := ch.GetModelEntries()
		entry, ok := entries[modelID]
		if !ok {
			continue
		}
		candidates = append(candidates, &orchestrator.ChannelModelsCandidate{
			Channel:  ch,
			Priority: 0,
			Models:   []biz.ChannelModelEntry{entry},
		})
	}
	return candidates, nil
}

func derivePreviewStrategy(retryPolicy *biz.RetryPolicy) string {
	if retryPolicy == nil || retryPolicy.LoadBalancerStrategy == "" {
		return biz.LoadBalancerStrategyAdaptive
	}
	return retryPolicy.LoadBalancerStrategy
}

func newPreviewLoadBalancer(r *Resolver, strategy string) *orchestrator.LoadBalancer {
	switch strategy {
	case biz.LoadBalancerStrategyFailover:
		return orchestrator.NewLoadBalancer(r.systemService, r.channelService,
			orchestrator.NewWeightStrategy(),
			orchestrator.NewRandomStrategy(),
		)
	case biz.LoadBalancerStrategyCircuitBreaker:
		return orchestrator.NewLoadBalancer(r.systemService, r.channelService,
			orchestrator.NewWeightStrategy(),
			orchestrator.NewModelAwareCircuitBreakerStrategy(biz.NewModelCircuitBreaker()),
		)
	case biz.LoadBalancerStrategyHighAvailability:
		return orchestrator.NewLoadBalancer(r.systemService, r.channelService,
			orchestrator.NewProbeHealthStrategy(r.channelService, nil, orchestrator.ProbeHealthModeAvailability),
			orchestrator.NewWeightStrategy(),
		)
	case biz.LoadBalancerStrategyLowLatency:
		return orchestrator.NewLoadBalancer(r.systemService, r.channelService,
			orchestrator.NewProbeHealthStrategy(r.channelService, nil, orchestrator.ProbeHealthModeLowLatency),
			orchestrator.NewWeightStrategy(),
		)
	default:
		return orchestrator.NewLoadBalancer(r.systemService, r.channelService,
			orchestrator.NewTraceAwareStrategy(r.requestService),
			orchestrator.NewErrorAwareStrategy(r.channelService),
			orchestrator.NewWeightRoundRobinStrategy(r.channelService),
			orchestrator.NewConnectionAwareStrategy(r.channelService, nil),
		)
	}
}
