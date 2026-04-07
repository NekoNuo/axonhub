package biz

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/log"
)

func TestRuntimeLogService(t *testing.T) {
	ctx := authz.WithTestBypass(context.Background())
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	svc := NewRuntimeLogService(client)
	require.NoError(t, svc.Start(ctx))
	defer func() {
		require.NoError(t, svc.Stop(ctx))
	}()

	svc.WriteRuntimeLog(ctx, log.RuntimeLogRecord{
		CreatedAt:     time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC),
		Logger:        "axonhub",
		Level:         log.RuntimeLogLevelWarn,
		Message:       "request process failed",
		Caller:        "internal/server/orchestrator/request_execution.go:193",
		TraceID:       "trace-123",
		RequestID:     "req-456",
		OperationName: "POST /v1/responses",
		ChannelID:     func() *int { v := 4; return &v }(),
		ChannelName:   "ggboom",
		ModelID:       "gpt-5.4",
		FieldsJSON: map[string]any{
			"error": "503 Service Unavailable",
		},
	})

	require.Eventually(t, func() bool {
		count, err := client.RuntimeLog.Query().Count(ctx)
		require.NoError(t, err)
		return count == 1
	}, time.Second, 10*time.Millisecond)

	entry := client.RuntimeLog.Query().OnlyX(ctx)
	assert.Equal(t, "axonhub", entry.Logger)
	assert.Equal(t, "warn", entry.Level.String())
	assert.Equal(t, "request process failed", entry.Message)
	assert.Equal(t, "internal/server/orchestrator/request_execution.go:193", entry.Caller)
	require.NotNil(t, entry.TraceID)
	assert.Equal(t, "trace-123", *entry.TraceID)
	require.NotNil(t, entry.RequestID)
	assert.Equal(t, "req-456", *entry.RequestID)
	require.NotNil(t, entry.OperationName)
	assert.Equal(t, "POST /v1/responses", *entry.OperationName)
	require.NotNil(t, entry.ChannelID)
	assert.Equal(t, 4, *entry.ChannelID)
	require.NotNil(t, entry.ChannelName)
	assert.Equal(t, "ggboom", *entry.ChannelName)
	require.NotNil(t, entry.ModelID)
	assert.Equal(t, "gpt-5.4", *entry.ModelID)
	assert.Equal(t, "503 Service Unavailable", entry.FieldsJSON["error"])
}
