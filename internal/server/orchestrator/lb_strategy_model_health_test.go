package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/server/biz"
)

type mockModelHealthProvider struct {
	health map[string]*biz.ModelHealthSnapshotView
}

func (m *mockModelHealthProvider) GetEffectiveModelHealth(_ context.Context, displayModel string, channelID int, actualModelID string) (*biz.ModelHealthSnapshotView, bool, error) {
	if m == nil || m.health == nil {
		return nil, false, nil
	}

	key := modelHealthKey(displayModel, channelID, actualModelID)
	item, ok := m.health[key]
	if !ok || item == nil {
		return nil, false, nil
	}

	return item, true, nil
}

func modelHealthKey(displayModel string, channelID int, actualModelID string) string {
	return displayModel + "|" + actualModelID + "|" + string(rune(channelID))
}

func TestModelHealthStrategy(t *testing.T) {
	ctx := context.Background()
	ctx = contextWithRequestedModel(ctx, "gpt-4o")

	fastHealthy := &biz.Channel{
		Channel: &ent.Channel{
			ID:             1,
			Name:           "fast-healthy",
			OrderingWeight: 10,
		},
	}
	slowHealthy := &biz.Channel{
		Channel: &ent.Channel{
			ID:             2,
			Name:           "slow-healthy",
			OrderingWeight: 30,
		},
	}
	channelOnlyHealthy := &biz.Channel{
		Channel: &ent.Channel{
			ID:             3,
			Name:           "channel-only",
			OrderingWeight: 50,
		},
	}
	manualOverride := &biz.Channel{
		Channel: &ent.Channel{
			ID:             4,
			Name:           "manual-override",
			OrderingWeight: 5,
		},
	}

	probeProvider := &mockProbeHealthProvider{
		health: map[int]*biz.ChannelProbeHealth{
			1: {Alive: true, ModelsAlive: true, ProbeModelAlive: true, ProbeModelLatencyMs: float64Ptr(150), Timestamp: 1},
			2: {Alive: true, ModelsAlive: true, ProbeModelAlive: true, ProbeModelLatencyMs: float64Ptr(180), Timestamp: 1},
			3: {Alive: true, ModelsAlive: true, ProbeModelAlive: true, ProbeModelLatencyMs: float64Ptr(100), Timestamp: 1},
			4: {Alive: true, ModelsAlive: true, ProbeModelAlive: true, ProbeModelLatencyMs: float64Ptr(220), Timestamp: 1},
		},
	}

	modelProvider := &mockModelHealthProvider{
		health: map[string]*biz.ModelHealthSnapshotView{
			modelHealthKey("gpt-4o", 1, "gpt-4o-2024-11-20"): {
				DisplayModel:   "gpt-4o",
				ChannelID:      1,
				ActualModelID:  "gpt-4o-2024-11-20",
				IsHealthy:      true,
				ManualOverride: false,
				ProbedAt:       100,
			},
			modelHealthKey("gpt-4o", 2, "gpt-4o-2024-11-20"): {
				DisplayModel:   "gpt-4o",
				ChannelID:      2,
				ActualModelID:  "gpt-4o-2024-11-20",
				IsHealthy:      true,
				ManualOverride: false,
				ProbedAt:       100,
			},
			modelHealthKey("gpt-4o", 4, "gpt-4o-2024-11-20"): {
				DisplayModel:   "gpt-4o",
				ChannelID:      4,
				ActualModelID:  "gpt-4o-2024-11-20",
				IsHealthy:      false,
				ManualOverride: true,
				ProbedAt:       101,
			},
		},
	}

	strategy := NewModelHealthStrategy(modelProvider, probeProvider, nil, ProbeHealthModeAvailability)

	ctxFast := contextWithCandidateActualModel(ctx, "gpt-4o-2024-11-20")
	fastScore := strategy.Score(ctxFast, fastHealthy)
	slowScore := strategy.Score(ctxFast, slowHealthy)
	fallbackScore := strategy.Score(ctxFast, channelOnlyHealthy)
	manualScore := strategy.Score(ctxFast, manualOverride)

	assert.Greater(t, fastScore, fallbackScore)
	assert.Greater(t, slowScore, fallbackScore)
	assert.Greater(t, slowScore, fastScore)
	assert.Less(t, manualScore, fallbackScore)
}
