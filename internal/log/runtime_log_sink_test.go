package log

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRuntimeLogSink struct {
	records []RuntimeLogRecord
}

func (s *testRuntimeLogSink) WriteRuntimeLog(_ context.Context, record RuntimeLogRecord) {
	s.records = append(s.records, record)
}

func TestBuildRuntimeLogRecord(t *testing.T) {
	now := time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC)

	record := buildRuntimeLogRecord(
		now,
		"axonhub",
		WarnLevel,
		"request process failed",
		"internal/server/orchestrator/request_execution.go:193",
		[]Field{
			String("trace_id", "trace-123"),
			String("request_id", "req-456"),
			String("operation_name", "POST /v1/responses"),
			Int("channel_id", 4),
			String("channel_name", "ggboom"),
			String("model_id", "gpt-5.4"),
			String("error", "503 Service Unavailable"),
			ByteString("response_body", []byte(`{"error":{"message":"Service temporarily unavailable"}}`)),
		},
	)

	assert.Equal(t, now, record.CreatedAt)
	assert.Equal(t, "axonhub", record.Logger)
	assert.Equal(t, RuntimeLogLevelWarn, record.Level)
	assert.Equal(t, "request process failed", record.Message)
	assert.Equal(t, "internal/server/orchestrator/request_execution.go:193", record.Caller)
	assert.Equal(t, "trace-123", record.TraceID)
	assert.Equal(t, "req-456", record.RequestID)
	assert.Equal(t, "POST /v1/responses", record.OperationName)
	require.NotNil(t, record.ChannelID)
	assert.Equal(t, 4, *record.ChannelID)
	assert.Equal(t, "ggboom", record.ChannelName)
	assert.Equal(t, "gpt-5.4", record.ModelID)
	assert.Equal(t, "503 Service Unavailable", record.FieldsJSON["error"])
	assert.Equal(t, `{"error":{"message":"Service temporarily unavailable"}}`, record.FieldsJSON["response_body"])
	_, hasTraceID := record.FieldsJSON["trace_id"]
	assert.False(t, hasTraceID)
}

func TestLoggerPublishesRuntimeLogRecord(t *testing.T) {
	logger, _ := createTestLogger(DebugLevel)
	sink := &testRuntimeLogSink{}
	SetRuntimeLogSink(sink)
	defer SetRuntimeLogSink(nil)

	ctx := context.Background()
	logger.Debug(ctx, "debug message", String("channel_name", "ignored"))
	logger.Info(ctx, "info message", String("channel_name", "primary"))
	logger.Warn(ctx, "warn message", Int("channel_id", 7))
	logger.Error(ctx, "error message", String("trace_id", "trace-999"))

	require.Len(t, sink.records, 3)
	assert.Equal(t, RuntimeLogLevelInfo, sink.records[0].Level)
	assert.Equal(t, "primary", sink.records[0].ChannelName)
	assert.Equal(t, RuntimeLogLevelWarn, sink.records[1].Level)
	require.NotNil(t, sink.records[1].ChannelID)
	assert.Equal(t, 7, *sink.records[1].ChannelID)
	assert.Equal(t, RuntimeLogLevelError, sink.records[2].Level)
	assert.Equal(t, "trace-999", sink.records[2].TraceID)
	assert.NotEmpty(t, sink.records[2].Caller)
}
