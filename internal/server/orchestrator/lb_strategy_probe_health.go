package orchestrator

import (
	"context"
	"time"

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
	provider          ChannelProbeHealthProvider
	connectionTracker ConnectionTracker
	mode              ProbeHealthMode

	softLatencyMs float64
	hardLatencyMs float64
	observedTTL   time.Duration
}

func NewProbeHealthStrategy(provider ChannelProbeHealthProvider, connectionTracker ConnectionTracker, mode ProbeHealthMode) *ProbeHealthStrategy {
	return &ProbeHealthStrategy{
		provider:          provider,
		connectionTracker: connectionTracker,
		mode:              mode,
		softLatencyMs:     1200,
		hardLatencyMs:     3000,
		observedTTL:       5 * time.Minute,
	}
}

func (s *ProbeHealthStrategy) Score(ctx context.Context, channel *biz.Channel) float64 {
	if s.provider == nil {
		return 0
	}

	health, ok := s.provider.GetChannelProbeHealth(channel.ID)
	if !ok || health == nil {
		return s.scoreWithoutProbeHealth(channel, nil)
	}

	if !s.hasRecordedProbeHealth(health) {
		return s.scoreWithoutProbeHealth(channel, health)
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
		score := s.scoreWithoutProbeHealth(channel, nil)
		hasActiveConnections := s.hasActiveConnections(channel)

		return score, StrategyScore{
			StrategyName: s.Name(),
			Score:        score,
			Details: map[string]any{
				"mode":                   string(s.mode),
				"health_found":           false,
				"active_connections":     s.activeConnections(channel),
				"has_active_connections": hasActiveConnections,
				"fallback_reason":        "missing_probe_health",
			},
		}
	}

	if !s.hasRecordedProbeHealth(health) {
		score := s.scoreWithoutProbeHealth(channel, health)
		details := map[string]any{
			"mode":                     string(s.mode),
			"health_found":             true,
			"probe_health_recorded":    false,
			"observed_health_recorded": health.ObservedHealthRecorded,
			"observed_alive":           health.ObservedAlive,
			"observed_timestamp":       health.ObservedTimestamp,
			"active_connections":       s.activeConnections(channel),
			"fallback_reason":          "observed_traffic_health",
		}
		if health.ObservedLatencyMs != nil {
			details["observed_latency_ms"] = *health.ObservedLatencyMs
		}

		return score, StrategyScore{
			StrategyName: s.Name(),
			Score:        score,
			Details:      details,
		}
	}

	score := s.score(health)
	details := map[string]any{
		"mode":               string(s.mode),
		"health_found":       true,
		"alive":              health.Alive,
		"models_alive":       health.ModelsAlive,
		"probe_model_alive":  health.ProbeModelAlive,
		"soft_latency_ms":    s.softLatencyMs,
		"hard_latency_ms":    s.hardLatencyMs,
		"timestamp":          health.Timestamp,
		"observed_alive":     health.ObservedAlive,
		"observed_timestamp": health.ObservedTimestamp,
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
	if health.ObservedLatencyMs != nil {
		details["observed_latency_ms"] = *health.ObservedLatencyMs
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

func (s *ProbeHealthStrategy) scoreWithoutProbeHealth(channel *biz.Channel, health *biz.ChannelProbeHealth) float64 {
	if observedScore, ok := s.scoreObservedHealth(health); ok {
		return observedScore
	}

	if s.hasActiveConnections(channel) {
		switch s.mode {
		case ProbeHealthModeLowLatency:
			return 180
		default:
			return 260
		}
	}

	return 0
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

	if health.ActiveProbeLatencyMs != nil {
		return health.ActiveProbeLatencyMs
	}

	if s.observedHealthUsable(health) {
		return health.ObservedLatencyMs
	}

	return nil
}

func (s *ProbeHealthStrategy) hasActiveConnections(channel *biz.Channel) bool {
	return s.activeConnections(channel) > 0
}

func (s *ProbeHealthStrategy) activeConnections(channel *biz.Channel) int {
	if s.connectionTracker == nil || channel == nil {
		return 0
	}

	return s.connectionTracker.GetActiveConnections(channel.ID)
}

func (s *ProbeHealthStrategy) scoreObservedHealth(health *biz.ChannelProbeHealth) (float64, bool) {
	if !s.observedHealthUsable(health) {
		return 0, false
	}

	latency := health.ObservedLatencyMs
	switch s.mode {
	case ProbeHealthModeLowLatency:
		base := 420.0
		if latency == nil {
			return base, true
		}

		value := *latency
		if value > s.hardLatencyMs {
			return -800, true
		}

		if value <= s.softLatencyMs {
			return base + 180, true
		}

		ratio := (value - s.softLatencyMs) / (s.hardLatencyMs - s.softLatencyMs)
		return (base + 180) - (ratio * 520), true
	default:
		base := 340.0
		if latency == nil {
			return base, true
		}

		value := *latency
		if value > s.hardLatencyMs {
			return -250, true
		}

		if value <= s.softLatencyMs {
			return base + 100, true
		}

		ratio := (value - s.softLatencyMs) / (s.hardLatencyMs - s.softLatencyMs)
		return (base + 100) - (ratio * 260), true
	}
}

func (s *ProbeHealthStrategy) observedHealthUsable(health *biz.ChannelProbeHealth) bool {
	if health == nil || !health.ObservedHealthRecorded || !health.ObservedAlive || health.ObservedTimestamp <= 0 {
		return false
	}

	return time.Since(time.Unix(health.ObservedTimestamp, 0)) <= s.observedTTL
}

func (s *ProbeHealthStrategy) hasRecordedProbeHealth(health *biz.ChannelProbeHealth) bool {
	if health == nil {
		return false
	}

	if health.ProbeHealthRecorded {
		return true
	}

	if health.Timestamp > 0 {
		return true
	}

	return health.ActiveProbeLatencyMs != nil || health.ProbeModelLatencyMs != nil || health.Alive || health.ModelsAlive || health.ProbeModelAlive
}
