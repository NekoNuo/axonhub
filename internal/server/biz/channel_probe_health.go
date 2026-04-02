package biz

import "time"

// ChannelProbeHealth represents the latest probe-based health snapshot for a channel.
// It is maintained in-memory and refreshed by ChannelProbeService.
type ChannelProbeHealth struct {
	ProbeHealthRecorded bool
	Alive               bool
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

	ObservedHealthRecorded bool
	ObservedAlive          bool
	// ObservedLatencyMs stores the most recent successful real-request latency in milliseconds.
	ObservedLatencyMs *float64
	// ObservedTimestamp is when the latest successful real request was observed (unix seconds).
	ObservedTimestamp int64
}

func (h *ChannelProbeHealth) Clone() *ChannelProbeHealth {
	if h == nil {
		return nil
	}

	clone := &ChannelProbeHealth{
		ProbeHealthRecorded:    h.ProbeHealthRecorded,
		Alive:                  h.Alive,
		ModelsAlive:            h.ModelsAlive,
		ProbeModelAlive:        h.ProbeModelAlive,
		Timestamp:              h.Timestamp,
		ObservedHealthRecorded: h.ObservedHealthRecorded,
		ObservedAlive:          h.ObservedAlive,
		ObservedTimestamp:      h.ObservedTimestamp,
	}

	if h.ActiveProbeLatencyMs != nil {
		latency := *h.ActiveProbeLatencyMs
		clone.ActiveProbeLatencyMs = &latency
	}

	if h.ProbeModelLatencyMs != nil {
		latency := *h.ProbeModelLatencyMs
		clone.ProbeModelLatencyMs = &latency
	}

	if h.ObservedLatencyMs != nil {
		latency := *h.ObservedLatencyMs
		clone.ObservedLatencyMs = &latency
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

	merged := health.Clone()
	merged.ProbeHealthRecorded = true

	if existing, ok := svc.channelProbeHealth[channelID]; ok && existing != nil {
		merged.ObservedHealthRecorded = existing.ObservedHealthRecorded
		merged.ObservedAlive = existing.ObservedAlive
		merged.ObservedTimestamp = existing.ObservedTimestamp
		merged.ObservedLatencyMs = cloneFloat64Ptr(existing.ObservedLatencyMs)
	}

	svc.channelProbeHealth[channelID] = merged
}

// UpdateChannelObservedHealth updates the latest successful real-traffic health snapshot for a channel.
func (svc *ChannelService) UpdateChannelObservedHealth(channelID int, latencyMs *float64, observedAt time.Time) {
	svc.channelProbeHealthLock.Lock()
	defer svc.channelProbeHealthLock.Unlock()

	var merged *ChannelProbeHealth
	if existing, ok := svc.channelProbeHealth[channelID]; ok && existing != nil {
		merged = existing.Clone()
	} else {
		merged = &ChannelProbeHealth{}
	}

	merged.ObservedHealthRecorded = true
	merged.ObservedAlive = true
	merged.ObservedTimestamp = observedAt.Unix()
	merged.ObservedLatencyMs = cloneFloat64Ptr(latencyMs)

	svc.channelProbeHealth[channelID] = merged
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

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}
