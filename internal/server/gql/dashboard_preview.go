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
	ModelID        string
	ActiveStrategy string
	Strategies     []*loadBalancerPreviewStrategy
}

type loadBalancerPreviewSummary struct {
	PrimaryChannelName    string
	FirstRetryChannelName string
	FallbackChannelName   string
}

type loadBalancerPreviewCandidate struct {
	ChannelName    string
	TotalScore     float64
	Reason         string
	HealthStatus   string
	LatencyMS      *float64
	RecentFailures int64
	ScoreBreakdown []*loadBalancerPreviewScoreBreakdown
}

type loadBalancerPreviewStep struct {
	Attempt            int
	ChannelName        string
	WaitMSAfterFailure int
	Reason             string
}

type loadBalancerPreviewStrategy struct {
	Strategy   string
	Summary    loadBalancerPreviewSummary
	Candidates []*loadBalancerPreviewCandidate
	Steps      []*loadBalancerPreviewStep
}

type loadBalancerPreviewScoreBreakdown struct {
	StrategyName string
	Score        float64
	Reason       string
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
	activeStrategy := derivePreviewStrategy(retryPolicy)

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
			ModelID:        modelID,
			ActiveStrategy: activeStrategy,
			Strategies:     []*loadBalancerPreviewStrategy{},
		}, nil
	}

	preview := &loadBalancerPreviewSnapshot{
		ModelID:        modelID,
		ActiveStrategy: activeStrategy,
		Strategies:     make([]*loadBalancerPreviewStrategy, 0, len(previewStrategies)),
	}

	for _, strategy := range previewStrategies {
		item, err := buildStrategyPreview(ctx, r, strategy, retryPolicy, candidates, modelID)
		if err != nil {
			return nil, err
		}
		preview.Strategies = append(preview.Strategies, item)
	}

	return preview, nil
}

var previewStrategies = []string{
	biz.LoadBalancerStrategyAdaptive,
	biz.LoadBalancerStrategyFailover,
	biz.LoadBalancerStrategyCircuitBreaker,
	biz.LoadBalancerStrategyHighAvailability,
	biz.LoadBalancerStrategyLowLatency,
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

func buildStrategyPreview(
	ctx context.Context,
	r *Resolver,
	strategy string,
	retryPolicy *biz.RetryPolicy,
	candidates []*orchestrator.ChannelModelsCandidate,
	modelID string,
) (*loadBalancerPreviewStrategy, error) {
	loadBalancer, scorers := newPreviewLoadBalancer(r, strategy)
	sorted := loadBalancer.Sort(ctx, cloneCandidates(candidates), modelID)

	item := &loadBalancerPreviewStrategy{
		Strategy:   strategy,
		Candidates: make([]*loadBalancerPreviewCandidate, 0, len(sorted)),
		Steps:      make([]*loadBalancerPreviewStep, 0, len(sorted)),
	}

	for idx, candidate := range sorted {
		totalScore := 0.0
		breakdowns := make([]*loadBalancerPreviewScoreBreakdown, 0, len(scorers))
		reasons := make([]string, 0, len(scorers))
		for _, scorer := range scorers {
			score, detail := scorer.ScoreWithDebug(ctx, candidate.Channel)
			totalScore += score
			reason := scoreReason(detail)
			if reason != "" {
				reasons = append(reasons, reason)
			}
			breakdowns = append(breakdowns, &loadBalancerPreviewScoreBreakdown{
				StrategyName: detail.StrategyName,
				Score:        detail.Score,
				Reason:       reason,
			})
		}

		reasonText := "selected by current ranking"
		if len(reasons) > 0 {
			reasonText = reasons[0]
		}
		item.Candidates = append(item.Candidates, &loadBalancerPreviewCandidate{
			ChannelName:    candidate.Channel.Name,
			TotalScore:     totalScore,
			Reason:         reasonText,
			HealthStatus:   candidateHealthStatus(r.channelService, candidate.Channel.ID),
			LatencyMS:      candidateLatency(r.channelService, candidate.Channel.ID),
			RecentFailures: candidateRecentFailures(ctx, r.channelService, candidate.Channel.ID),
			ScoreBreakdown: breakdowns,
		})

		waitMS := retryPolicy.RetryDelayMs
		if idx == len(sorted)-1 {
			waitMS = 0
		}
		item.Steps = append(item.Steps, &loadBalancerPreviewStep{
			Attempt:            idx + 1,
			ChannelName:        candidate.Channel.Name,
			WaitMSAfterFailure: waitMS,
			Reason:             reasonText,
		})
	}

	if len(sorted) > 0 {
		item.Summary.PrimaryChannelName = sorted[0].Channel.Name
	}
	if len(sorted) > 1 {
		item.Summary.FirstRetryChannelName = sorted[1].Channel.Name
	}
	if len(sorted) > 2 {
		item.Summary.FallbackChannelName = sorted[len(sorted)-1].Channel.Name
	} else if len(sorted) > 0 {
		item.Summary.FallbackChannelName = sorted[len(sorted)-1].Channel.Name
	}

	return item, nil
}

func cloneCandidates(candidates []*orchestrator.ChannelModelsCandidate) []*orchestrator.ChannelModelsCandidate {
	cloned := make([]*orchestrator.ChannelModelsCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		cloned = append(cloned, &orchestrator.ChannelModelsCandidate{
			Channel:  candidate.Channel,
			Priority: candidate.Priority,
			Models:   candidate.Models,
		})
	}
	return cloned
}

func scoreReason(detail orchestrator.StrategyScore) string {
	if detail.Details == nil {
		return ""
	}
	if reason, ok := detail.Details["reason"].(string); ok && reason != "" {
		return humanizeReason(reason, detail)
	}
	if reason, ok := detail.Details["fallback_reason"].(string); ok && reason != "" {
		return humanizeReason(reason, detail)
	}
	return ""
}

func humanizeReason(reason string, detail orchestrator.StrategyScore) string {
	switch reason {
	case "zero_requests":
		if weight, ok := detail.Details["ordering_weight"]; ok {
			return fmt.Sprintf("No recent traffic; weight %v keeps it near the front", weight)
		}
		return "No recent traffic, so the strategy keeps this candidate competitive"
	case "last_successful_channel_in_trace":
		return "Matches the last successful channel in the current trace"
	case "not_last_successful_channel":
		return "Not the last successful trace channel, so it gets no trace boost"
	case "missing_probe_health":
		return "No active probe snapshot is available yet"
	case "observed_traffic_health":
		return "Using observed real-traffic health because no fresh active probe is available"
	case "no_probe_health_provider":
		return "Probe health provider is unavailable"
	case "no_trace_in_context":
		return "No trace context is available, so trace affinity is skipped"
	default:
		return reason
	}
}

func candidateHealthStatus(channelService *biz.ChannelService, channelID int) string {
	health, ok := channelService.GetChannelProbeHealth(channelID)
	if !ok || health == nil {
		return "unknown"
	}
	if health.ProbeHealthRecorded {
		if !health.Alive {
			return "unhealthy"
		}
		if health.ProbeModelLatencyMs != nil && *health.ProbeModelLatencyMs > 1800 {
			return "degraded"
		}
		return "healthy"
	}
	if health.ObservedHealthRecorded {
		if health.ObservedAlive {
			return "observed-healthy"
		}
		return "observed-unhealthy"
	}
	return "unknown"
}

func candidateLatency(channelService *biz.ChannelService, channelID int) *float64 {
	health, ok := channelService.GetChannelProbeHealth(channelID)
	if !ok || health == nil {
		return nil
	}
	if health.ProbeModelLatencyMs != nil {
		return lo.ToPtr(*health.ProbeModelLatencyMs)
	}
	if health.ActiveProbeLatencyMs != nil {
		return lo.ToPtr(*health.ActiveProbeLatencyMs)
	}
	if health.ObservedLatencyMs != nil {
		return lo.ToPtr(*health.ObservedLatencyMs)
	}
	return nil
}

func candidateRecentFailures(ctx context.Context, channelService *biz.ChannelService, channelID int) int64 {
	metrics, err := channelService.GetChannelMetrics(ctx, channelID)
	if err != nil || metrics == nil {
		return 0
	}
	return metrics.FailureCount
}

func newPreviewLoadBalancer(r *Resolver, strategy string) (*orchestrator.LoadBalancer, []orchestrator.LoadBalanceStrategy) {
	switch strategy {
	case biz.LoadBalancerStrategyFailover:
		scorers := []orchestrator.LoadBalanceStrategy{
			orchestrator.NewWeightStrategy(),
			orchestrator.NewRandomStrategy(),
		}
		return orchestrator.NewLoadBalancer(r.systemService, nil, scorers...), scorers
	case biz.LoadBalancerStrategyCircuitBreaker:
		scorers := []orchestrator.LoadBalanceStrategy{
			orchestrator.NewWeightStrategy(),
			orchestrator.NewModelAwareCircuitBreakerStrategy(biz.NewModelCircuitBreaker()),
		}
		return orchestrator.NewLoadBalancer(r.systemService, nil, scorers...), scorers
	case biz.LoadBalancerStrategyHighAvailability:
		scorers := []orchestrator.LoadBalanceStrategy{
			orchestrator.NewProbeHealthStrategy(r.channelService, nil, orchestrator.ProbeHealthModeAvailability),
			orchestrator.NewWeightStrategy(),
		}
		return orchestrator.NewLoadBalancer(r.systemService, nil, scorers...), scorers
	case biz.LoadBalancerStrategyLowLatency:
		scorers := []orchestrator.LoadBalanceStrategy{
			orchestrator.NewProbeHealthStrategy(r.channelService, nil, orchestrator.ProbeHealthModeLowLatency),
			orchestrator.NewWeightStrategy(),
		}
		return orchestrator.NewLoadBalancer(r.systemService, nil, scorers...), scorers
	default:
		scorers := []orchestrator.LoadBalanceStrategy{
			orchestrator.NewTraceAwareStrategy(r.requestService),
			orchestrator.NewErrorAwareStrategy(r.channelService),
			orchestrator.NewWeightRoundRobinStrategy(r.channelService),
			orchestrator.NewConnectionAwareStrategy(r.channelService, nil),
		}
		return orchestrator.NewLoadBalancer(r.systemService, nil, scorers...), scorers
	}
}
