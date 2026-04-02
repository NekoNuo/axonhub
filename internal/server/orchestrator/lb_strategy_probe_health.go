package orchestrator

import (
	"context"

	"github.com/looplj/axonhub/internal/server/biz"
)

type ProbeHealthMode string

const (
	ProbeHealthModeAvailability ProbeHealthMode = "high-availability"
	ProbeHealthModeLowLatency   ProbeHealthMode = "low-latency"
)

// ChannelProbeHealthProvider provides latest probe-based health snapshots.
type ChannelProbeHealthProvider interface {
	GetChannelProbeHealth(channelID int) (*biz.ChannelProbeHealth, bool)
}

// ProbeHealthStrategy scores channels by probe-based health and latency.
// It strongly penalizes unhealthy channels and (mode-dependently) high-latency channels.
type ProbeHealthStrategy struct {
	provider ChannelProbeHealthProvider
	mode     ProbeHealthMode

	softLatencyMs float64
	hardLatencyMs float64
}

func NewProbeHealthStrategy(provider ChannelProbeHealthProvider, mode ProbeHealthMode) *ProbeHealthStrategy {
	return &ProbeHealthStrategy{
		provider:      provider,
		mode:          mode,
		softLatencyMs: 1200,
		hardLatencyMs: 3000,
	}
}

func (s *ProbeHealthStrategy) Score(ctx context.Context, channel *biz.Channel) float64 {
	if s.provider == nil {
		return 0
	}

	health, ok := s.provider.GetChannelProbeHealth(channel.ID)
	if !ok || health == nil {
		return 0
	}

	return s.score(health)
}

func (s *ProbeHealthStrategy) ScoreWithDebug(ctx context.Context, channel *biz.Channel) (float64, StrategyScore) {
	if s.provider == nil {
		return 0, StrategyScore{
			StrategyName: s.Name(),
			Score:        0,
			Details: map[string]any{
				"mode":   string(s.mode),
				"reason": "no_probe_health_provider",
			},
		}
	}

	health, ok := s.provider.GetChannelProbeHealth(channel.ID)
	if !ok || health == nil {
		return 0, StrategyScore{
			StrategyName: s.Name(),
			Score:        0,
			Details: map[string]any{
				"mode":         string(s.mode),
				"health_found": false,
			},
		}
	}

	score := s.score(health)
	details := map[string]any{
		"mode":              string(s.mode),
		"health_found":      true,
		"alive":             health.Alive,
		"models_alive":      health.ModelsAlive,
		"probe_model_alive": health.ProbeModelAlive,
		"soft_latency_ms":   s.softLatencyMs,
		"hard_latency_ms":   s.hardLatencyMs,
		"timestamp":         health.Timestamp,
	}
	if latency := s.preferredLatency(health); latency != nil {
		details["latency_ms"] = *latency
	}
	if health.ActiveProbeLatencyMs != nil {
		details["active_probe_latency_ms"] = *health.ActiveProbeLatencyMs
	}
	if health.ProbeModelLatencyMs != nil {
		details["probe_model_latency_ms"] = *health.ProbeModelLatencyMs
	}

	return score, StrategyScore{
		StrategyName: s.Name(),
		Score:        score,
		Details:      details,
	}
}

func (s *ProbeHealthStrategy) Name() string {
	return "ProbeHealth"
}

func (s *ProbeHealthStrategy) score(health *biz.ChannelProbeHealth) float64 {
	if health == nil {
		return 0
	}

	if !health.Alive {
		return -3000
	}

	switch s.mode {
	case ProbeHealthModeLowLatency:
		return s.scoreLowLatency(health)
	default:
		return s.scoreAvailability(health)
	}
}

func (s *ProbeHealthStrategy) scoreAvailability(health *biz.ChannelProbeHealth) float64 {
	// Healthy channels get a stable positive baseline.
	base := 450.0
	latency := s.preferredLatency(health)
	if latency == nil {
		return base
	}

	value := *latency
	if value > s.hardLatencyMs {
		return -1000
	}

	if value <= s.softLatencyMs {
		return base + 100
	}

	// Linear penalty between soft/hard thresholds.
	ratio := (value - s.softLatencyMs) / (s.hardLatencyMs - s.softLatencyMs)
	return (base + 100) - (ratio * 320)
}

func (s *ProbeHealthStrategy) scoreLowLatency(health *biz.ChannelProbeHealth) float64 {
	// Low-latency mode aggressively prefers fast channels.
	base := 600.0
	latency := s.preferredLatency(health)
	if latency == nil {
		return -300
	}

	value := *latency
	if value > s.hardLatencyMs {
		return -2500
	}

	if value <= s.softLatencyMs {
		return base + 400
	}

	// Linear drop from fast to degraded in the threshold band.
	ratio := (value - s.softLatencyMs) / (s.hardLatencyMs - s.softLatencyMs)
	return (base + 400) - (ratio * 900)
}

func (s *ProbeHealthStrategy) preferredLatency(health *biz.ChannelProbeHealth) *float64 {
	if health == nil {
		return nil
	}

	if health.ProbeModelLatencyMs != nil {
		return health.ProbeModelLatencyMs
	}

	return health.ActiveProbeLatencyMs
}
