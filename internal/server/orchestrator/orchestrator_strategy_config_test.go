package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChatCompletionOrchestrator_StrategyConfigForHighAvailabilityAndLowLatency(t *testing.T) {
	processor := NewChatCompletionOrchestrator(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	require.NotNil(t, processor)

	require.NotNil(t, processor.highAvailabilityLoadBalancer)
	require.Len(t, processor.highAvailabilityLoadBalancer.strategies, 2)

	haModelHealth, ok := processor.highAvailabilityLoadBalancer.strategies[0].(*ModelHealthStrategy)
	require.True(t, ok)
	assert.Equal(t, ProbeHealthModeAvailability, haModelHealth.mode)
	assert.NotNil(t, haModelHealth.connectionTracker)
	assert.NotNil(t, haModelHealth.fallback)
	assert.Equal(t, ProbeHealthModeAvailability, haModelHealth.fallback.mode)
	assert.IsType(t, &WeightStrategy{}, processor.highAvailabilityLoadBalancer.strategies[1])

	require.NotNil(t, processor.lowLatencyLoadBalancer)
	require.Len(t, processor.lowLatencyLoadBalancer.strategies, 2)

	llModelHealth, ok := processor.lowLatencyLoadBalancer.strategies[0].(*ModelHealthStrategy)
	require.True(t, ok)
	assert.Equal(t, ProbeHealthModeLowLatency, llModelHealth.mode)
	assert.NotNil(t, llModelHealth.connectionTracker)
	assert.NotNil(t, llModelHealth.fallback)
	assert.Equal(t, ProbeHealthModeLowLatency, llModelHealth.fallback.mode)
	assert.IsType(t, &WeightStrategy{}, processor.lowLatencyLoadBalancer.strategies[1])
}
