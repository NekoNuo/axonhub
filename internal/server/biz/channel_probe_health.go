package biz

// ChannelProbeHealth represents the latest probe-based health snapshot for a channel.
// It is maintained in-memory and refreshed by ChannelProbeService.
type ChannelProbeHealth struct {
	Alive bool
	// LatencyMs is probe-observed latency in milliseconds (when available).
	LatencyMs *float64
	// Timestamp is the probe window timestamp (unix seconds).
	Timestamp int64
}

func (h *ChannelProbeHealth) Clone() *ChannelProbeHealth {
	if h == nil {
		return nil
	}

	clone := &ChannelProbeHealth{
		Alive:     h.Alive,
		Timestamp: h.Timestamp,
	}

	if h.LatencyMs != nil {
		latency := *h.LatencyMs
		clone.LatencyMs = &latency
	}

	return clone
}

// UpdateChannelProbeHealth updates in-memory probe health snapshot for a channel.
func (svc *ChannelService) UpdateChannelProbeHealth(channelID int, health *ChannelProbeHealth) {
	if health == nil {
		return
	}

	svc.channelProbeHealthLock.Lock()
	defer svc.channelProbeHealthLock.Unlock()

	svc.channelProbeHealth[channelID] = health.Clone()
}

// GetChannelProbeHealth returns the latest in-memory probe health snapshot for a channel.
func (svc *ChannelService) GetChannelProbeHealth(channelID int) (*ChannelProbeHealth, bool) {
	svc.channelProbeHealthLock.RLock()
	defer svc.channelProbeHealthLock.RUnlock()

	health, ok := svc.channelProbeHealth[channelID]
	if !ok || health == nil {
		return nil, false
	}

	return health.Clone(), true
}
