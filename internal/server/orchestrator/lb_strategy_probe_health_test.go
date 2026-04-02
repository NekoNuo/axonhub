package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/server/biz"
)

type mockProbeHealthProvider struct {
	health map[int]*biz.ChannelProbeHealth
}

func (m *mockProbeHealthProvider) GetChannelProbeHealth(channelID int) (*biz.ChannelProbeHealth, bool) {
	if m.health == nil {
		return nil, false
	}

	h, ok := m.health[channelID]
	if !ok || h == nil {
		return nil, false
	}

	return h.Clone(), true
}

func newLBTestChannel(id int, name string) *biz.Channel {
	return &biz.Channel{
		Channel: &ent.Channel{
			ID:   id,
			Name: name,
		},
	}
}

func TestProbeHealthStrategy_NoHealthDataReturnsNeutral(t *testing.T) {
	strategy := NewProbeHealthStrategy(&mockProbeHealthProvider{}, ProbeHealthModeLowLatency)

	score := strategy.Score(context.Background(), newLBTestChannel(1, "ch-1"))
	assert.Equal(t, 0.0, score)
}

func TestProbeHealthStrategy_UnhealthyChannelGetsStrongPenalty(t *testing.T) {
	provider := &mockProbeHealthProvider{
		health: map[int]*biz.ChannelProbeHealth{
			1: {
				Alive:           false,
				ModelsAlive:     false,
				ProbeModelAlive: false,
				Timestamp:       1,
			},
		},
	}
	strategy := NewProbeHealthStrategy(provider, ProbeHealthModeAvailability)

	score := strategy.Score(context.Background(), newLBTestChannel(1, "down"))
	assert.Less(t, score, -2000.0)
}

func TestProbeHealthStrategy_LowLatencyPrefersFastChannels(t *testing.T) {
	provider := &mockProbeHealthProvider{
		health: map[int]*biz.ChannelProbeHealth{
			1: {
				Alive:                true,
				ModelsAlive:          true,
				ProbeModelAlive:      true,
				ActiveProbeLatencyMs: float64Ptr(300),
				ProbeModelLatencyMs:  float64Ptr(200),
				Timestamp:            1,
			},
			2: {
				Alive:                true,
				ModelsAlive:          true,
				ProbeModelAlive:      true,
				ActiveProbeLatencyMs: float64Ptr(1600),
				ProbeModelLatencyMs:  float64Ptr(1800),
				Timestamp:            1,
			},
			3: {
				Alive:                true,
				ModelsAlive:          true,
				ProbeModelAlive:      true,
				ActiveProbeLatencyMs: float64Ptr(4500),
				ProbeModelLatencyMs:  float64Ptr(5000),
				Timestamp:            1,
			},
		},
	}
	strategy := NewProbeHealthStrategy(provider, ProbeHealthModeLowLatency)

	fast := strategy.Score(context.Background(), newLBTestChannel(1, "fast"))
	slow := strategy.Score(context.Background(), newLBTestChannel(2, "slow"))
	verySlow := strategy.Score(context.Background(), newLBTestChannel(3, "very-slow"))

	assert.Greater(t, fast, slow)
	assert.Greater(t, slow, verySlow)
	assert.Less(t, verySlow, -1500.0)
}

func TestProbeHealthStrategy_AvailabilityModeStillDeprioritizesHighLatency(t *testing.T) {
	provider := &mockProbeHealthProvider{
		health: map[int]*biz.ChannelProbeHealth{
			1: {
				Alive:                true,
				ModelsAlive:          true,
				ProbeModelAlive:      true,
				ActiveProbeLatencyMs: float64Ptr(450),
				ProbeModelLatencyMs:  float64Ptr(400),
				Timestamp:            1,
			},
			2: {
				Alive:                true,
				ModelsAlive:          true,
				ProbeModelAlive:      true,
				ActiveProbeLatencyMs: float64Ptr(4000),
				ProbeModelLatencyMs:  float64Ptr(4200),
				Timestamp:            1,
			},
			3: {
				Alive:                false,
				ModelsAlive:          true,
				ProbeModelAlive:      false,
				ActiveProbeLatencyMs: float64Ptr(200),
				ProbeModelLatencyMs:  float64Ptr(200),
				Timestamp:            1,
			},
		},
	}
	strategy := NewProbeHealthStrategy(provider, ProbeHealthModeAvailability)

	healthy := strategy.Score(context.Background(), newLBTestChannel(1, "healthy"))
	highLatency := strategy.Score(context.Background(), newLBTestChannel(2, "high-latency"))
	down := strategy.Score(context.Background(), newLBTestChannel(3, "down"))

	assert.Greater(t, healthy, highLatency)
	assert.Greater(t, highLatency, down)
}

func TestProbeHealthStrategy_ScoreWithDebugIncludesModeAndHealth(t *testing.T) {
	provider := &mockProbeHealthProvider{
		health: map[int]*biz.ChannelProbeHealth{
			1: {
				Alive:                true,
				ModelsAlive:          true,
				ProbeModelAlive:      true,
				ActiveProbeLatencyMs: float64Ptr(380),
				ProbeModelLatencyMs:  float64Ptr(350),
				Timestamp:            123,
			},
		},
	}
	strategy := NewProbeHealthStrategy(provider, ProbeHealthModeLowLatency)

	score, debug := strategy.ScoreWithDebug(context.Background(), newLBTestChannel(1, "ch-1"))

	assert.Equal(t, score, debug.Score)
	assert.Equal(t, "ProbeHealth", debug.StrategyName)

	mode, ok := debug.Details["mode"].(string)
	require.True(t, ok)
	assert.Equal(t, string(ProbeHealthModeLowLatency), mode)

	alive, ok := debug.Details["alive"].(bool)
	require.True(t, ok)
	assert.True(t, alive)
}

func TestProbeHealthStrategy_LowLatencyFallsBackToModelsLatencyWhenProbeModelLatencyMissing(t *testing.T) {
	provider := &mockProbeHealthProvider{
		health: map[int]*biz.ChannelProbeHealth{
			1: {
				Alive:                true,
				ModelsAlive:          true,
				ProbeModelAlive:      true,
				ActiveProbeLatencyMs: float64Ptr(220),
				Timestamp:            1,
			},
			2: {
				Alive:                true,
				ModelsAlive:          true,
				ProbeModelAlive:      true,
				ActiveProbeLatencyMs: float64Ptr(1400),
				Timestamp:            1,
			},
		},
	}

	strategy := NewProbeHealthStrategy(provider, ProbeHealthModeLowLatency)
	assert.Greater(
		t,
		strategy.Score(context.Background(), newLBTestChannel(1, "faster-models-fallback")),
		strategy.Score(context.Background(), newLBTestChannel(2, "slower-models-fallback")),
	)
}

func float64Ptr(v float64) *float64 {
	return &v
}
