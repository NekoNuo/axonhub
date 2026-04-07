package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/server/biz"
)

func TestConfigureUsageLogHooks_DoesNotWireModelHealthRefresh(t *testing.T) {
	usageLogSvc := &biz.UsageLogService{}
	usageLogSvc.OnUsageLogFromRequestCreated = func(context.Context, *ent.Request, *ent.RequestExecution, *ent.UsageLog) {}

	configureUsageLogHooks(usageLogSvc)

	require.NotNil(t, usageLogSvc.OnUsageLogCreated)
	require.Nil(t, usageLogSvc.OnUsageLogFromRequestCreated)
}
