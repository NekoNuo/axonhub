package orchestrator

import (
	"context"

	"github.com/looplj/axonhub/internal/server/biz"
)

type candidateActualModelContextKey struct{}

func contextWithCandidateActualModel(ctx context.Context, modelID string) context.Context {
	return context.WithValue(ctx, candidateActualModelContextKey{}, modelID)
}

func candidateActualModelFromContext(ctx context.Context) string {
	if model, ok := ctx.Value(candidateActualModelContextKey{}).(string); ok {
		return model
	}

	return ""
}

type ModelHealthProvider interface {
	GetEffectiveModelHealth(ctx context.Context, displayModel string, channelID int, actualModelID string) (*biz.ModelHealthSnapshotView, bool, error)
}

type ModelHealthStrategy struct {
	modelProvider   ModelHealthProvider
	channelProvider ChannelProbeHealthProvider
	connectionTracker ConnectionTracker
	mode            ProbeHealthMode
	fallback        *ProbeHealthStrategy
}

func NewModelHealthStrategy(
	modelProvider ModelHealthProvider,
	channelProvider ChannelProbeHealthProvider,
	connectionTracker ConnectionTracker,
	mode ProbeHealthMode,
) *ModelHealthStrategy {
	return &ModelHealthStrategy{
		modelProvider:     modelProvider,
		channelProvider:   channelProvider,
		connectionTracker: connectionTracker,
		mode:              mode,
		fallback:          NewProbeHealthStrategy(channelProvider, connectionTracker, mode),
	}
}

func (s *ModelHealthStrategy) Name() string {
	return "ModelHealth"
}

func (s *ModelHealthStrategy) Score(ctx context.Context, channel *biz.Channel) float64 {
	score, _ := s.score(ctx, channel)
	return score
}

func (s *ModelHealthStrategy) ScoreWithDebug(ctx context.Context, channel *biz.Channel) (float64, StrategyScore) {
	score, details := s.score(ctx, channel)
	return score, StrategyScore{
		StrategyName: s.Name(),
		Score:        score,
		Details:      details,
	}
}

func (s *ModelHealthStrategy) score(ctx context.Context, channel *biz.Channel) (float64, map[string]any) {
	if s.modelProvider == nil {
		score := s.fallback.Score(ctx, channel)
		return score, map[string]any{"mode": string(s.mode), "reason": "no_model_health_provider", "fallback": true}
	}

	displayModel := requestedModelFromContext(ctx)
	actualModel := candidateActualModelFromContext(ctx)
	if displayModel == "" || actualModel == "" {
		score := s.fallback.Score(ctx, channel)
		return score, map[string]any{"mode": string(s.mode), "reason": "missing_model_context", "fallback": true}
	}

	view, ok, err := s.modelProvider.GetEffectiveModelHealth(ctx, displayModel, channel.ID, actualModel)
	if err != nil || !ok || view == nil {
		score := s.fallback.Score(ctx, channel)
		return score, map[string]any{
			"mode":          string(s.mode),
			"fallback":      true,
			"display_model": displayModel,
			"actual_model":  actualModel,
			"reason":        "missing_model_health",
		}
	}

	if !view.IsHealthy {
		return -5000, map[string]any{
			"mode":            string(s.mode),
			"display_model":   displayModel,
			"actual_model":    actualModel,
			"manual_override": view.ManualOverride,
			"is_healthy":      false,
			"probed_at":       view.ProbedAt,
		}
	}

	weightScore := 1000.0 + float64(channel.OrderingWeight)
	return weightScore, map[string]any{
		"mode":            string(s.mode),
		"display_model":   displayModel,
		"actual_model":    actualModel,
		"manual_override": view.ManualOverride,
		"is_healthy":      true,
		"probed_at":       view.ProbedAt,
	}
}
