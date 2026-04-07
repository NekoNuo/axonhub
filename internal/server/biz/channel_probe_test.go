package biz

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/channelprobe"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/ent/requestexecution"
	"github.com/looplj/axonhub/internal/ent/usagelog"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xtime"
)

func TestShouldRunProbe(t *testing.T) {
	now := time.Date(2026, 4, 7, 1, 38, 0, 0, time.UTC)

	t.Run("skip immediate execution on startup", func(t *testing.T) {
		assert.False(t, shouldRunProbe(ProbeFrequency1Hour, now, time.Time{}))
	})

	t.Run("skip when current bucket already executed", func(t *testing.T) {
		lastExecution := time.Date(2026, 4, 7, 1, 0, 0, 0, time.UTC)
		assert.False(t, shouldRunProbe(ProbeFrequency1Hour, now, lastExecution))
	})

	t.Run("run when entering next bucket", func(t *testing.T) {
		lastExecution := time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC)
		assert.True(t, shouldRunProbe(ProbeFrequency1Hour, now, lastExecution))
	})
}

// TestTPSCalculation_RetryScenario tests that only successful executions are counted
func TestTPSCalculation_RetryScenario(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c1").
		SetStatus(channel.StatusEnabled).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	endTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	startTime := endTime.Add(-time.Minute)
	now := startTime.Add(30 * time.Second)

	// Create request
	req, err := client.Request.Create().
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		SetChannelID(ch.ID).
		SetStream(true).
		SetMetricsLatencyMs(3000).
		SetMetricsFirstTokenLatencyMs(500).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Create 3 executions: 2 failed, 1 successful
	_, err = client.RequestExecution.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusFailed).
		SetStream(true).
		SetMetricsLatencyMs(1000).
		SetMetricsFirstTokenLatencyMs(200).
		SetCreatedAt(now.Add(-2 * time.Minute)).
		SetUpdatedAt(now.Add(-2 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.RequestExecution.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusFailed).
		SetStream(true).
		SetMetricsLatencyMs(2000).
		SetMetricsFirstTokenLatencyMs(300).
		SetCreatedAt(now.Add(-1 * time.Minute)).
		SetUpdatedAt(now.Add(-1 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	// Successful execution (most recent) - this should be used
	_, err = client.RequestExecution.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusCompleted).
		SetStream(true).
		SetMetricsLatencyMs(3000).
		SetMetricsFirstTokenLatencyMs(500).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Create usage log with all token types
	_, err = client.UsageLog.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetCompletionTokens(50).
		SetCompletionReasoningTokens(30).
		SetCompletionAudioTokens(20).
		SetTotalTokens(100).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Query successful executions only - should return only the most recent successful one
	execs, err := client.RequestExecution.Query().
		Where(
			requestexecution.RequestID(req.ID),
			requestexecution.StatusEQ(requestexecution.StatusCompleted),
		).
		Order(ent.Desc(requestexecution.FieldCreatedAt)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, execs, 1)

	// Verify the successful execution has the correct latency
	assert.Equal(t, int64(3000), *execs[0].MetricsLatencyMs)
	assert.Equal(t, int64(500), *execs[0].MetricsFirstTokenLatencyMs)

	// Verify usage log has all token types
	ul, err := client.UsageLog.Query().
		Where(usagelog.RequestID(req.ID)).
		Only(ctx)
	require.NoError(t, err)

	totalTokens := ul.CompletionTokens + ul.CompletionReasoningTokens + ul.CompletionAudioTokens
	assert.Equal(t, int64(100), totalTokens)

	// TPS calculation: 100 tokens / ((3000 - 500) / 1000) = 100 / 2.5 = 40 tokens/s
	effectiveLatency := *execs[0].MetricsLatencyMs - *execs[0].MetricsFirstTokenLatencyMs
	tps := float64(totalTokens) / (float64(effectiveLatency) / 1000.0)
	assert.InDelta(t, 40.0, tps, 0.01)
}

// TestTPSCalculation_AllTokenTypes tests that all token types are included
func TestTPSCalculation_AllTokenTypes(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c1").
		SetStatus(channel.StatusEnabled).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	endTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	startTime := endTime.Add(-time.Minute)
	now := startTime.Add(30 * time.Second)

	req, err := client.Request.Create().
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		SetChannelID(ch.ID).
		SetStream(true).
		SetMetricsLatencyMs(2000).
		SetMetricsFirstTokenLatencyMs(400).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.RequestExecution.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusCompleted).
		SetStream(true).
		SetMetricsLatencyMs(2000).
		SetMetricsFirstTokenLatencyMs(400).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Usage log with all token types: completion=100, reasoning=50, audio=25, total=175
	_, err = client.UsageLog.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetCompletionTokens(100).
		SetCompletionReasoningTokens(50).
		SetCompletionAudioTokens(25).
		SetTotalTokens(175).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Verify total tokens calculation
	ul, err := client.UsageLog.Query().
		Where(usagelog.RequestID(req.ID)).
		Only(ctx)
	require.NoError(t, err)

	totalTokens := ul.CompletionTokens + ul.CompletionReasoningTokens + ul.CompletionAudioTokens
	assert.Equal(t, int64(175), totalTokens)

	// TPS: 175 tokens / ((2000 - 400) / 1000) = 175 / 1.6 = 109.375 tokens/s
	effectiveLatency := int64(2000) - int64(400)
	tps := float64(totalTokens) / (float64(effectiveLatency) / 1000.0)
	assert.InDelta(t, 109.375, tps, 0.01)
}

// TestTPSCalculation_StreamingVsNonStreaming tests streaming vs non-streaming formulas
func TestTPSCalculation_StreamingVsNonStreaming(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c1").
		SetStatus(channel.StatusEnabled).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	endTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	startTime := endTime.Add(-time.Minute)
	now := startTime.Add(30 * time.Second)

	// Streaming request
	req1, err := client.Request.Create().
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		SetChannelID(ch.ID).
		SetStream(true).
		SetMetricsLatencyMs(3000).
		SetMetricsFirstTokenLatencyMs(500).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.RequestExecution.Create().
		SetRequestID(req1.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusCompleted).
		SetStream(true).
		SetMetricsLatencyMs(3000).
		SetMetricsFirstTokenLatencyMs(500).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(req1.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetCompletionTokens(100).
		SetTotalTokens(100).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Non-streaming request
	req2, err := client.Request.Create().
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		SetChannelID(ch.ID).
		SetStream(false).
		SetMetricsLatencyMs(2000).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.RequestExecution.Create().
		SetRequestID(req2.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusCompleted).
		SetStream(false).
		SetMetricsLatencyMs(2000).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(req2.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetCompletionTokens(100).
		SetTotalTokens(100).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Calculate combined TPS
	// Streaming effective latency: 3000 - 500 = 2500
	// Non-streaming effective latency: 2000
	// Total effective latency: 2500 + 2000 = 4500
	// Total tokens: 100 + 100 = 200
	// TPS: 200 tokens / (4500 / 1000) = 200 / 4.5 = 44.44 tokens/s
	streamingLatency := int64(3000) - int64(500)
	nonStreamingLatency := int64(2000)
	totalEffectiveLatency := streamingLatency + nonStreamingLatency
	totalTokens := int64(200)
	tps := float64(totalTokens) / (float64(totalEffectiveLatency) / 1000.0)
	assert.InDelta(t, 44.44, tps, 0.01)
}

// TestTPSCalculation_EdgeCases tests edge cases
func TestTPSCalculation_EdgeCases(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c1").
		SetStatus(channel.StatusEnabled).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	endTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	startTime := endTime.Add(-time.Minute)
	now := startTime.Add(30 * time.Second)

	// Request with zero first token latency
	req, err := client.Request.Create().
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		SetChannelID(ch.ID).
		SetStream(true).
		SetMetricsLatencyMs(1000).
		SetMetricsFirstTokenLatencyMs(0).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.RequestExecution.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusCompleted).
		SetStream(true).
		SetMetricsLatencyMs(1000).
		SetMetricsFirstTokenLatencyMs(0).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetCompletionTokens(50).
		SetTotalTokens(50).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// TPS: 50 tokens / ((1000 - 0) / 1000) = 50 tokens/s
	effectiveLatency := int64(1000) - int64(0)
	tps := float64(50) / (float64(effectiveLatency) / 1000.0)
	assert.InDelta(t, 50.0, tps, 0.01)
}

// TestTPSCalculation_EmptyResults tests empty results
func TestTPSCalculation_EmptyResults(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c1").
		SetStatus(channel.StatusEnabled).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	endTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	startTime := endTime.Add(-time.Minute)

	// Create the ChannelProbeService with the test client
	svc := &ChannelProbeService{
		AbstractService: &AbstractService{
			db: client,
		},
	}

	// Call computeAllChannelProbeStats with a channel that has no execution data
	stats, err := svc.computeAllChannelProbeStats(ctx, []int{ch.ID}, startTime, endTime)
	require.NoError(t, err)
	assert.Empty(t, stats, "stats should be empty for channel with no execution data")
}

// TestComputeAllChannelProbeStats_Integration tests the actual computeAllChannelProbeStats function
// with real database query and result scanning.
func TestComputeAllChannelProbeStats_Integration(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	// Create a channel
	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("test-channel").
		SetStatus(channel.StatusEnabled).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	endTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	startTime := endTime.Add(-time.Minute)
	now := startTime.Add(30 * time.Second)

	// Create a request with streaming
	req, err := client.Request.Create().
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		SetChannelID(ch.ID).
		SetStream(true).
		SetMetricsLatencyMs(3000).
		SetMetricsFirstTokenLatencyMs(500).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Create successful execution
	_, err = client.RequestExecution.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusCompleted).
		SetStream(true).
		SetMetricsLatencyMs(3000).
		SetMetricsFirstTokenLatencyMs(500).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Create usage log with tokens
	_, err = client.UsageLog.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetCompletionTokens(100).
		SetCompletionReasoningTokens(50).
		SetCompletionAudioTokens(25).
		SetTotalTokens(175).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Create the ChannelProbeService with the test client
	svc := &ChannelProbeService{
		AbstractService: &AbstractService{
			db: client,
		},
	}

	// Call the actual computeAllChannelProbeStats function
	stats, err := svc.computeAllChannelProbeStats(ctx, []int{ch.ID}, startTime, endTime)
	require.NoError(t, err)
	require.NotNil(t, stats, "stats should not be nil")
	require.Contains(t, stats, ch.ID, "stats should contain channel ID")

	channelStats := stats[ch.ID]
	require.NotNil(t, channelStats, "channel stats should not be nil")

	// Verify total and success counts
	assert.Equal(t, 1, channelStats.total)
	assert.Equal(t, 1, channelStats.success)

	// Verify TPS calculation: 175 tokens / ((3000 - 500) / 1000) = 175 / 2.5 = 70 tokens/s
	require.NotNil(t, channelStats.avgTokensPerSecond, "avgTokensPerSecond should not be nil")
	assert.InDelta(t, 70.0, *channelStats.avgTokensPerSecond, 0.01)

	// Verify TTFT calculation: 500ms / 1 request = 500ms
	require.NotNil(t, channelStats.avgTimeToFirstTokenMs, "avgTimeToFirstTokenMs should not be nil")
	assert.InDelta(t, 500.0, *channelStats.avgTimeToFirstTokenMs, 0.01)
}

// TestComputeAllChannelProbeStats_MultipleRequests tests with multiple requests
func TestComputeAllChannelProbeStats_MultipleRequests(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("multi-req-channel").
		SetStatus(channel.StatusEnabled).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	endTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	startTime := endTime.Add(-time.Minute)

	// Create 3 requests with different latencies and tokens
	requests := []struct {
		latencyMs         int64
		firstTokenLatency int64
		completionTokens  int64
		reasoningTokens   int64
		audioTokens       int64
		stream            bool
	}{
		{3000, 500, 100, 50, 25, true}, // TPS: 175 / 2.5 = 70
		{2000, 400, 80, 20, 0, true},   // TPS: 100 / 1.6 = 62.5
		{4000, 0, 200, 0, 0, false},    // TPS: 200 / 4 = 50 (non-streaming)
	}

	for i, r := range requests {
		reqTime := startTime.Add(time.Duration(i+1) * 10 * time.Second)
		req, err := client.Request.Create().
			SetModelID("gpt-4").
			SetRequestBody(objects.JSONRawMessage(`{}`)).
			SetStatus(request.StatusCompleted).
			SetChannelID(ch.ID).
			SetStream(r.stream).
			SetMetricsLatencyMs(r.latencyMs).
			SetMetricsFirstTokenLatencyMs(r.firstTokenLatency).
			SetCreatedAt(reqTime).
			SetUpdatedAt(reqTime).
			Save(ctx)
		require.NoError(t, err)

		_, err = client.RequestExecution.Create().
			SetRequestID(req.ID).
			SetChannelID(ch.ID).
			SetModelID("gpt-4").
			SetRequestBody(objects.JSONRawMessage(`{}`)).
			SetStatus(requestexecution.StatusCompleted).
			SetStream(r.stream).
			SetMetricsLatencyMs(r.latencyMs).
			SetMetricsFirstTokenLatencyMs(r.firstTokenLatency).
			SetCreatedAt(reqTime).
			SetUpdatedAt(reqTime).
			Save(ctx)
		require.NoError(t, err)

		totalTokens := r.completionTokens + r.reasoningTokens + r.audioTokens
		_, err = client.UsageLog.Create().
			SetRequestID(req.ID).
			SetChannelID(ch.ID).
			SetModelID("gpt-4").
			SetCompletionTokens(r.completionTokens).
			SetCompletionReasoningTokens(r.reasoningTokens).
			SetCompletionAudioTokens(r.audioTokens).
			SetTotalTokens(totalTokens).
			SetCreatedAt(reqTime).
			SetUpdatedAt(reqTime).
			Save(ctx)
		require.NoError(t, err)
	}

	svc := &ChannelProbeService{
		AbstractService: &AbstractService{
			db: client,
		},
	}

	stats, err := svc.computeAllChannelProbeStats(ctx, []int{ch.ID}, startTime, endTime)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Contains(t, stats, ch.ID)

	channelStats := stats[ch.ID]
	assert.Equal(t, 3, channelStats.total)
	assert.Equal(t, 3, channelStats.success)

	// Total tokens: 175 + 100 + 200 = 475
	// Effective latency: (3000-500) + (2000-400) + 4000 = 2500 + 1600 + 4000 = 8100
	// TPS: 475 / 8.1 = 58.64 tokens/s
	require.NotNil(t, channelStats.avgTokensPerSecond)
	assert.InDelta(t, 58.64, *channelStats.avgTokensPerSecond, 0.1)

	// TTFT: (500 + 400 + 0) / 2 = 450ms (only streaming requests count: 2 out of 3 requests are streaming)
	require.NotNil(t, channelStats.avgTimeToFirstTokenMs)
	assert.InDelta(t, 450.0, *channelStats.avgTimeToFirstTokenMs, 0.01)
}

// TestComputeAllChannelProbeStats_EmptyChannel tests with a channel that has no data
func TestComputeAllChannelProbeStats_EmptyChannel(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("empty-channel").
		SetStatus(channel.StatusEnabled).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	endTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	startTime := endTime.Add(-time.Minute)

	svc := &ChannelProbeService{
		AbstractService: &AbstractService{
			db: client,
		},
	}

	stats, err := svc.computeAllChannelProbeStats(ctx, []int{ch.ID}, startTime, endTime)
	require.NoError(t, err)
	assert.NotNil(t, stats, "stats should not be nil")
	assert.Empty(t, stats, "stats should be empty for channel with no data")
}

// TestComputeAllChannelProbeStats_MultipleChannels tests with multiple channels
func TestComputeAllChannelProbeStats_MultipleChannels(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	// Create two channels
	ch1, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("channel-1").
		SetStatus(channel.StatusEnabled).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	ch2, err := client.Channel.Create().
		SetType(channel.TypeAnthropicFake).
		SetName("channel-2").
		SetStatus(channel.StatusEnabled).
		SetSupportedModels([]string{"claude-3-5-sonnet"}).
		SetDefaultTestModel("claude-3-5-sonnet").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	endTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	startTime := endTime.Add(-time.Minute)
	now := startTime.Add(30 * time.Second)

	// Create request for channel 1
	req1, err := client.Request.Create().
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		SetChannelID(ch1.ID).
		SetStream(true).
		SetMetricsLatencyMs(2000).
		SetMetricsFirstTokenLatencyMs(400).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.RequestExecution.Create().
		SetRequestID(req1.ID).
		SetChannelID(ch1.ID).
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusCompleted).
		SetStream(true).
		SetMetricsLatencyMs(2000).
		SetMetricsFirstTokenLatencyMs(400).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(req1.ID).
		SetChannelID(ch1.ID).
		SetModelID("gpt-4").
		SetCompletionTokens(100).
		SetTotalTokens(100).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Create request for channel 2
	req2, err := client.Request.Create().
		SetModelID("claude-3-5-sonnet").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		SetChannelID(ch2.ID).
		SetStream(true).
		SetMetricsLatencyMs(3000).
		SetMetricsFirstTokenLatencyMs(600).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.RequestExecution.Create().
		SetRequestID(req2.ID).
		SetChannelID(ch2.ID).
		SetModelID("claude-3-5-sonnet").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusCompleted).
		SetStream(true).
		SetMetricsLatencyMs(3000).
		SetMetricsFirstTokenLatencyMs(600).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(req2.ID).
		SetChannelID(ch2.ID).
		SetModelID("claude-3-5-sonnet").
		SetCompletionTokens(150).
		SetTotalTokens(150).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	svc := &ChannelProbeService{
		AbstractService: &AbstractService{
			db: client,
		},
	}

	// Query both channels at once
	stats, err := svc.computeAllChannelProbeStats(ctx, []int{ch1.ID, ch2.ID}, startTime, endTime)
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Len(t, stats, 2, "should have stats for both channels")

	// Verify channel 1 stats: 100 tokens / ((2000-400)/1000) = 100 / 1.6 = 62.5 TPS
	require.Contains(t, stats, ch1.ID)
	stats1 := stats[ch1.ID]
	assert.Equal(t, 1, stats1.total)
	assert.Equal(t, 1, stats1.success)
	require.NotNil(t, stats1.avgTokensPerSecond)
	assert.InDelta(t, 62.5, *stats1.avgTokensPerSecond, 0.01)
	require.NotNil(t, stats1.avgTimeToFirstTokenMs)
	assert.InDelta(t, 400.0, *stats1.avgTimeToFirstTokenMs, 0.01)

	// Verify channel 2 stats: 150 tokens / ((3000-600)/1000) = 150 / 2.4 = 62.5 TPS
	require.Contains(t, stats, ch2.ID)
	stats2 := stats[ch2.ID]
	assert.Equal(t, 1, stats2.total)
	assert.Equal(t, 1, stats2.success)
	require.NotNil(t, stats2.avgTokensPerSecond)
	assert.InDelta(t, 62.5, *stats2.avgTokensPerSecond, 0.01)
	require.NotNil(t, stats2.avgTimeToFirstTokenMs)
	assert.InDelta(t, 600.0, *stats2.avgTimeToFirstTokenMs, 0.01)
}

// TestComputeAllChannelProbeStats_FailedExecutions tests that failed executions are excluded
func TestComputeAllChannelProbeStats_FailedExecutions(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("failed-exec-channel").
		SetStatus(channel.StatusEnabled).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	endTime := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	startTime := endTime.Add(-time.Minute)
	now := startTime.Add(30 * time.Second)

	// Create a request
	req, err := client.Request.Create().
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		SetChannelID(ch.ID).
		SetStream(true).
		SetMetricsLatencyMs(3000).
		SetMetricsFirstTokenLatencyMs(500).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Create a failed execution (should be excluded)
	_, err = client.RequestExecution.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusFailed).
		SetStream(true).
		SetMetricsLatencyMs(1000).
		SetMetricsFirstTokenLatencyMs(200).
		SetCreatedAt(now.Add(-1 * time.Minute)).
		SetUpdatedAt(now.Add(-1 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	// Create a successful execution (should be included)
	_, err = client.RequestExecution.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusCompleted).
		SetStream(true).
		SetMetricsLatencyMs(3000).
		SetMetricsFirstTokenLatencyMs(500).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	// Create usage log
	_, err = client.UsageLog.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4").
		SetCompletionTokens(100).
		SetTotalTokens(100).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	require.NoError(t, err)

	svc := &ChannelProbeService{
		AbstractService: &AbstractService{
			db: client,
		},
	}

	stats, err := svc.computeAllChannelProbeStats(ctx, []int{ch.ID}, startTime, endTime)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Contains(t, stats, ch.ID)

	channelStats := stats[ch.ID]
	// Should only count the successful execution
	assert.Equal(t, 1, channelStats.total)
	assert.Equal(t, 1, channelStats.success)

	// TPS: 100 tokens / ((3000-500)/1000) = 100 / 2.5 = 40 tokens/s
	require.NotNil(t, channelStats.avgTokensPerSecond)
	assert.InDelta(t, 40.0, *channelStats.avgTokensPerSecond, 0.01)
}

func TestFillIdleChannelProbeStats(t *testing.T) {
	svc := &ChannelProbeService{}
	svc.idleChannelProber = func(_ context.Context, ch *ent.Channel) (time.Duration, bool, error) {
		switch ch.ID {
		case 2:
			return 120 * time.Millisecond, false, fmt.Errorf("probe failed")
		case 3:
			return 80 * time.Millisecond, true, nil
		default:
			return 50 * time.Millisecond, false, nil
		}
	}

	channels := []*ent.Channel{
		{ID: 1, Type: channel.TypeOpenaiFake, BaseURL: "https://provider-1.example"},
		{ID: 2, Type: channel.TypeOpenaiFake, BaseURL: "https://provider-2.example"},
		{ID: 3, Type: channel.TypeOpenaiFake, BaseURL: "https://provider-3.example"},
	}

	allStats := map[int]*channelProbeStats{
		1: {
			total:   2,
			success: 2,
		},
	}

	probedCount, successCount := svc.fillIdleChannelProbeStats(context.Background(), channels, allStats)
	assert.Equal(t, 3, probedCount)
	assert.Equal(t, 1, successCount)

	assert.Equal(t, 1, allStats[1].total)
	assert.Equal(t, 0, allStats[1].success)
	require.NotNil(t, allStats[1].activeProbeLatencyMs)
	assert.InDelta(t, 50.0, *allStats[1].activeProbeLatencyMs, 0.01)
	require.NotNil(t, allStats[1].latencyMs)
	assert.InDelta(t, 50.0, *allStats[1].latencyMs, 0.01)
	assert.Equal(t, 1, allStats[2].total)
	assert.Equal(t, 0, allStats[2].success)
	require.NotNil(t, allStats[2].activeProbeLatencyMs)
	assert.InDelta(t, 120.0, *allStats[2].activeProbeLatencyMs, 0.01)
	require.NotNil(t, allStats[2].latencyMs)
	assert.InDelta(t, 120.0, *allStats[2].latencyMs, 0.01)
	assert.Equal(t, 1, allStats[3].total)
	assert.Equal(t, 1, allStats[3].success)
	require.NotNil(t, allStats[3].activeProbeLatencyMs)
	assert.InDelta(t, 80.0, *allStats[3].activeProbeLatencyMs, 0.01)
	require.NotNil(t, allStats[3].latencyMs)
	assert.InDelta(t, 80.0, *allStats[3].latencyMs, 0.01)
}

func TestFillIdleChannelProbeStats_ProbeModelEnabledUsesBothChecks(t *testing.T) {
	svc := &ChannelProbeService{}
	svc.idleChannelProber = func(_ context.Context, ch *ent.Channel) (time.Duration, bool, error) {
		return 90 * time.Millisecond, true, nil
	}
	svc.idleChannelModelProber = func(_ context.Context, ch *ent.Channel, modelID string) (time.Duration, bool, error) {
		require.Equal(t, 1, ch.ID)
		require.Equal(t, "gpt-4o-mini", modelID)
		return 140 * time.Millisecond, true, nil
	}

	channels := []*ent.Channel{
		{ID: 1, Type: channel.TypeOpenaiFake, BaseURL: "https://provider-1.example", DefaultTestModel: "gpt-4o-mini"},
	}
	allStats := map[int]*channelProbeStats{}

	probedCount, successCount := svc.fillIdleChannelProbeStatsWithSettings(
		context.Background(),
		channels,
		allStats,
		ChannelProbeSetting{ActiveProbeIdleChannels: true, ProbeModelIdleChannels: true},
	)

	assert.Equal(t, 1, probedCount)
	assert.Equal(t, 1, successCount)
	require.Contains(t, allStats, 1)
	assert.True(t, allStats[1].alive)
	require.NotNil(t, allStats[1].activeProbeLatencyMs)
	assert.InDelta(t, 90.0, *allStats[1].activeProbeLatencyMs, 0.01)
	require.NotNil(t, allStats[1].activeProbeModelLatencyMs)
	assert.InDelta(t, 140.0, *allStats[1].activeProbeModelLatencyMs, 0.01)
}

func TestFillIdleChannelProbeStats_ProbeModelFallsBackToModelsWhenDefaultModelMissing(t *testing.T) {
	svc := &ChannelProbeService{}
	svc.idleChannelProber = func(_ context.Context, ch *ent.Channel) (time.Duration, bool, error) {
		return 75 * time.Millisecond, true, nil
	}

	modelProbeCalls := 0
	svc.idleChannelModelProber = func(_ context.Context, ch *ent.Channel, modelID string) (time.Duration, bool, error) {
		modelProbeCalls++
		return 0, false, fmt.Errorf("unexpected model probe call for channel %d model %q", ch.ID, modelID)
	}

	channels := []*ent.Channel{
		{ID: 1, Type: channel.TypeOpenaiFake, BaseURL: "https://provider-1.example"},
	}
	allStats := map[int]*channelProbeStats{}

	probedCount, successCount := svc.fillIdleChannelProbeStatsWithSettings(
		context.Background(),
		channels,
		allStats,
		ChannelProbeSetting{ActiveProbeIdleChannels: true, ProbeModelIdleChannels: true},
	)

	assert.Equal(t, 1, probedCount)
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 0, modelProbeCalls)
	require.Contains(t, allStats, 1)
	assert.True(t, allStats[1].alive)
	require.NotNil(t, allStats[1].activeProbeLatencyMs)
	assert.InDelta(t, 75.0, *allStats[1].activeProbeLatencyMs, 0.01)
	require.NotNil(t, allStats[1].activeProbeModelLatencyMs)
	assert.InDelta(t, 75.0, *allStats[1].activeProbeModelLatencyMs, 0.01)
}

func TestRunProbe_ActiveModelProbeLoadsDefaultTestModel(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	err := systemService.SetChannelSetting(ctx, SystemChannelSettings{
		Probe: ChannelProbeSetting{
			Enabled:                 true,
			Frequency:               ProbeFrequency1Min,
			ActiveProbeIdleChannels: true,
			ProbeModelIdleChannels:  true,
		},
	})
	require.NoError(t, err)

	_, err = client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("probe-channel").
		SetStatus(channel.StatusEnabled).
		SetBaseURL("https://provider.example").
		SetSupportedModels([]string{"gpt-4o-mini"}).
		SetDefaultTestModel("gpt-4o-mini").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	modelProbeCalls := 0
	svc := &ChannelProbeService{
		AbstractService: &AbstractService{db: client},
		SystemService:   systemService,
	}
	svc.idleChannelProber = func(_ context.Context, ch *ent.Channel) (time.Duration, bool, error) {
		return 50 * time.Millisecond, true, nil
	}
	svc.idleChannelModelProber = func(_ context.Context, ch *ent.Channel, modelID string) (time.Duration, bool, error) {
		modelProbeCalls++
		assert.Equal(t, "gpt-4o-mini", modelID)
		return 80 * time.Millisecond, true, nil
	}

	svc.runProbe(ctx)

	assert.Equal(t, 1, modelProbeCalls)
}

func TestRunProbe_ActiveProbeAlsoProbesActiveChannels(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	err := systemService.SetChannelSetting(ctx, SystemChannelSettings{
		Probe: ChannelProbeSetting{
			Enabled:                 true,
			Frequency:               ProbeFrequency1Min,
			ActiveProbeIdleChannels: true,
		},
	})
	require.NoError(t, err)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("active-channel").
		SetStatus(channel.StatusEnabled).
		SetBaseURL("https://provider.example").
		SetSupportedModels([]string{"gpt-4o-mini"}).
		SetDefaultTestModel("gpt-4o-mini").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	now := xtime.UTCNow().Truncate(time.Minute)
	req, err := client.Request.Create().
		SetModelID("gpt-4o-mini").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		SetChannelID(ch.ID).
		SetStream(true).
		SetMetricsLatencyMs(1200).
		SetMetricsFirstTokenLatencyMs(300).
		SetCreatedAt(now.Add(-30 * time.Second)).
		SetUpdatedAt(now.Add(-30 * time.Second)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.RequestExecution.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4o-mini").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(requestexecution.StatusCompleted).
		SetStream(true).
		SetMetricsLatencyMs(1200).
		SetMetricsFirstTokenLatencyMs(300).
		SetCreatedAt(now.Add(-30 * time.Second)).
		SetUpdatedAt(now.Add(-30 * time.Second)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(req.ID).
		SetChannelID(ch.ID).
		SetModelID("gpt-4o-mini").
		SetCompletionTokens(100).
		SetTotalTokens(100).
		SetCreatedAt(now.Add(-30 * time.Second)).
		SetUpdatedAt(now.Add(-30 * time.Second)).
		Save(ctx)
	require.NoError(t, err)

	channelService := &ChannelService{
		channelProbeHealth: make(map[int]*ChannelProbeHealth),
	}
	probeCalls := 0
	svc := &ChannelProbeService{
		AbstractService: &AbstractService{db: client},
		SystemService:   systemService,
		ChannelService:  channelService,
	}
	svc.idleChannelProber = func(_ context.Context, got *ent.Channel) (time.Duration, bool, error) {
		probeCalls++
		require.Equal(t, ch.ID, got.ID)
		return 80 * time.Millisecond, true, nil
	}

	svc.runProbe(ctx)

	assert.Equal(t, 1, probeCalls)

	health, ok := channelService.GetChannelProbeHealth(ch.ID)
	require.True(t, ok)
	require.NotNil(t, health)
	assert.True(t, health.Alive)
	require.NotNil(t, health.ActiveProbeLatencyMs)
	assert.InDelta(t, 80.0, *health.ActiveProbeLatencyMs, 0.01)
}

func TestRunProbeNow_OverridesCurrentBucketAfterScheduledProbe(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := ent.NewContext(t.Context(), client)
	ctx = authz.WithTestBypass(ctx)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	err := systemService.SetChannelSetting(ctx, SystemChannelSettings{
		Probe: ChannelProbeSetting{
			Enabled:                 true,
			Frequency:               ProbeFrequency1Min,
			ActiveProbeIdleChannels: true,
		},
	})
	require.NoError(t, err)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("manual-probe-channel").
		SetStatus(channel.StatusEnabled).
		SetBaseURL("https://provider.example").
		SetSupportedModels([]string{"gpt-4o-mini"}).
		SetDefaultTestModel("gpt-4o-mini").
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	probeCalls := 0
	svc := &ChannelProbeService{
		AbstractService: &AbstractService{db: client},
		SystemService:   systemService,
	}
	svc.lastExecutionTime = xtime.UTCNow().UTC().Truncate(time.Minute).Add(-time.Minute)
	svc.idleChannelProber = func(_ context.Context, got *ent.Channel) (time.Duration, bool, error) {
		probeCalls++
		require.Equal(t, ch.ID, got.ID)
		if probeCalls == 1 {
			return 80 * time.Millisecond, true, nil
		}

		return 160 * time.Millisecond, false, nil
	}

	svc.runProbe(ctx)
	svc.RunProbeNow(ctx)

	assert.Equal(t, 2, probeCalls)

	probes, err := client.ChannelProbe.Query().
		Where(channelprobe.ChannelIDEQ(ch.ID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, probes, 1)

	assert.Equal(t, 1, probes[0].TotalRequestCount)
	assert.Equal(t, 0, probes[0].SuccessRequestCount)
	require.NotNil(t, probes[0].ActiveProbeLatencyMs)
	assert.InDelta(t, 160.0, *probes[0].ActiveProbeLatencyMs, 0.01)
}
