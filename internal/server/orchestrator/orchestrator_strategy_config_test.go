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

	haProbe, ok := processor.highAvailabilityLoadBalancer.strategies[0].(*ProbeHealthStrategy)
	require.True(t, ok)
	assert.Equal(t, ProbeHealthModeAvailability, haProbe.mode)
	assert.NotNil(t, haProbe.connectionTracker)
	assert.IsType(t, &WeightStrategy{}, processor.highAvailabilityLoadBalancer.strategies[1])

	require.NotNil(t, processor.lowLatencyLoadBalancer)
	require.Len(t, processor.lowLatencyLoadBalancer.strategies, 2)

	llProbe, ok := processor.lowLatencyLoadBalancer.strategies[0].(*ProbeHealthStrategy)
	require.True(t, ok)
	assert.Equal(t, ProbeHealthModeLowLatency, llProbe.mode)
	assert.NotNil(t, llProbe.connectionTracker)
	assert.IsType(t, &WeightStrategy{}, processor.lowLatencyLoadBalancer.strategies[1])
}
