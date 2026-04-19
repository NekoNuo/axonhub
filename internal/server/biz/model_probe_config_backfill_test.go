package biz

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackfillFromSnapshots_CreatesEnabledForExistingSnapshots(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")

	_, err := client.ModelHealthSnapshot.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetIsHealthy(true).
		SetProbedAt(1700000000).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ModelHealthSnapshot.Create().
		SetDisplayModel("claude-3").
		SetChannelID(ch.ID).
		SetActualModelID("claude-3-5-sonnet").
		SetIsHealthy(false).
		SetProbedAt(1700000000).
		Save(ctx)
	require.NoError(t, err)

	svc := NewModelProbeConfigService(client)
	require.NoError(t, svc.BackfillFromSnapshots(ctx))

	cfgs, err := svc.GetByDisplayModels(ctx, nil)
	require.NoError(t, err)
	require.Len(t, cfgs, 2)
	for _, c := range cfgs {
		require.True(t, c.ProbeEnabled, "backfilled configs must start enabled so existing deployments keep probing")
	}
}

func TestBackfillFromSnapshots_Idempotent(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")

	_, err := client.ModelHealthSnapshot.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetIsHealthy(true).
		SetProbedAt(1700000000).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(false).
		Save(ctx)
	require.NoError(t, err)

	svc := NewModelProbeConfigService(client)
	require.NoError(t, svc.BackfillFromSnapshots(ctx))
	require.NoError(t, svc.BackfillFromSnapshots(ctx))

	cfgs, err := svc.GetByDisplayModels(ctx, nil)
	require.NoError(t, err)
	require.Len(t, cfgs, 1)
	require.False(t, cfgs[0].ProbeEnabled, "existing configs must not be overwritten — respects user intent")
}
