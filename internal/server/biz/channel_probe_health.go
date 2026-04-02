package biz

// ChannelProbeHealth represents the latest probe-based health snapshot for a channel.
// It is maintained in-memory and refreshed by ChannelProbeService.
type ChannelProbeHealth struct {
	Alive bool
	// ModelsAlive reflects active /models probe health when available.
	ModelsAlive bool
	// ProbeModelAlive reflects active defaultTestModel probe health when available.
	ProbeModelAlive bool
	// ActiveProbeLatencyMs stores active /models probe latency in milliseconds.
	ActiveProbeLatencyMs *float64
	// ProbeModelLatencyMs stores active defaultTestModel probe latency in milliseconds.
	ProbeModelLatencyMs *float64
	// Timestamp is the probe window timestamp (unix seconds).
	Timestamp int64
}

func (h *ChannelProbeHealth) Clone() *ChannelProbeHealth {
	if h == nil {
		return nil
	}

	clone := &ChannelProbeHealth{
		Alive:           h.Alive,
		ModelsAlive:     h.ModelsAlive,
		ProbeModelAlive: h.ProbeModelAlive,
		Timestamp:       h.Timestamp,
	}

	if h.ActiveProbeLatencyMs != nil {
		latency := *h.ActiveProbeLatencyMs
		clone.ActiveProbeLatencyMs = &latency
	}

	if h.ProbeModelLatencyMs != nil {
		latency := *h.ProbeModelLatencyMs
		clone.ProbeModelLatencyMs = &latency
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
