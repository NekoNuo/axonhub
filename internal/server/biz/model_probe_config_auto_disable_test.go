package biz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/objects"
)

// setModelAutoDisableThreshold writes the per-model threshold used by
// auto-disable. Helper keeps each test focused on counter behavior.
func setModelAutoDisableThreshold(t *testing.T, ctx context.Context, client *ent.Client, modelID string, threshold int, channelID int, actualModelID string) {
	t.Helper()
	_, err := client.Model.Create().
		SetDeveloper("openai").
		SetModelID(modelID).
		SetType(model.TypeChat).
		SetName(modelID).
		SetIcon("openai").
		SetGroup("openai").
		SetStatus(model.StatusEnabled).
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{
			ProbeEnabled:                             true,
			ProbeAutoDisableAfterConsecutiveFailures: threshold,
			Associations: []*objects.ModelAssociation{
				{
					Type: "channel_model",
					ChannelModel: &objects.ChannelModelAssociation{
						ChannelID: channelID,
						ModelID:   actualModelID,
					},
				},
			},
		}).
		Save(ctx)
	require.NoError(t, err)
}

func TestUpdateOnProbeResult_Healthy_ResetsCounter(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")

	_, err := client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(true).
		SetConsecutiveFailures(3).
		Save(ctx)
	require.NoError(t, err)

	svc := NewModelProbeConfigService(client)
	err = svc.UpdateOnProbeResult(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20", true, 1700000000)
	require.NoError(t, err)

	cfg, err := svc.Get(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20")
	require.NoError(t, err)
	require.Equal(t, 0, cfg.ConsecutiveFailures)
	require.True(t, cfg.ProbeEnabled)
}

func TestUpdateOnProbeResult_Unhealthy_IncrementsBelowThreshold(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")
	setModelAutoDisableThreshold(t, ctx, client, "gpt-4o", 5, ch.ID, "gpt-4o-2024-11-20")

	_, err := client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(true).
		SetConsecutiveFailures(2).
		Save(ctx)
	require.NoError(t, err)

	svc := NewModelProbeConfigService(client)
	err = svc.UpdateOnProbeResult(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20", false, 1700000000)
	require.NoError(t, err)

	cfg, err := svc.Get(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20")
	require.NoError(t, err)
	require.Equal(t, 3, cfg.ConsecutiveFailures)
	require.True(t, cfg.ProbeEnabled)
	require.Nil(t, cfg.AutoDisabledAt)
}

func TestUpdateOnProbeResult_Unhealthy_AtThreshold_AutoDisables(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")
	setModelAutoDisableThreshold(t, ctx, client, "gpt-4o", 2, ch.ID, "gpt-4o-2024-11-20")

	_, err := client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(true).
		SetConsecutiveFailures(1).
		Save(ctx)
	require.NoError(t, err)

	svc := NewModelProbeConfigService(client)
	err = svc.UpdateOnProbeResult(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20", false, 1700000000)
	require.NoError(t, err)

	cfg, err := svc.Get(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20")
	require.NoError(t, err)
	require.Equal(t, 2, cfg.ConsecutiveFailures)
	require.False(t, cfg.ProbeEnabled)
	require.NotNil(t, cfg.AutoDisabledAt)
	require.Equal(t, int64(1700000000), *cfg.AutoDisabledAt)
}

func TestUpdateOnProbeResult_ThresholdZero_NeverAutoDisables(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")
	setModelAutoDisableThreshold(t, ctx, client, "gpt-4o", 0, ch.ID, "gpt-4o-2024-11-20")

	_, err := client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(true).
		SetConsecutiveFailures(99).
		Save(ctx)
	require.NoError(t, err)

	svc := NewModelProbeConfigService(client)
	err = svc.UpdateOnProbeResult(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20", false, 1700000000)
	require.NoError(t, err)

	cfg, err := svc.Get(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20")
	require.NoError(t, err)
	require.Equal(t, 100, cfg.ConsecutiveFailures)
	require.True(t, cfg.ProbeEnabled)
}

func TestUpdateOnProbeResult_CreatesRowIfMissing(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")

	svc := NewModelProbeConfigService(client)
	err := svc.UpdateOnProbeResult(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20", false, 1700000000)
	require.NoError(t, err)

	cfg, err := svc.Get(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, 1, cfg.ConsecutiveFailures)
	require.True(t, cfg.ProbeEnabled)
}

func TestPersistModelHealthResult_ManualProbe_DoesNotUpdateCounter(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")

	_, err := client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(true).
		SetConsecutiveFailures(1).
		Save(ctx)
	require.NoError(t, err)

	svc := &ChannelProbeService{
		AbstractService:         &AbstractService{db: client},
		ModelProbeConfigService: NewModelProbeConfigService(client),
	}

	target := ModelHealthProbeTarget{
		DisplayModel:  "gpt-4o",
		ChannelID:     ch.ID,
		ActualModelID: "gpt-4o-2024-11-20",
	}
	err = svc.persistModelHealthResult(ctx, target, false, 1700000000, true)
	require.NoError(t, err)

	cfg, err := svc.ModelProbeConfigService.Get(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20")
	require.NoError(t, err)
	require.Equal(t, 1, cfg.ConsecutiveFailures, "manual probe must not move counter")
	require.True(t, cfg.ProbeEnabled)
}

func TestPersistModelHealthResult_AutoProbe_UpdatesCounter(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")
	setModelAutoDisableThreshold(t, ctx, client, "gpt-4o", 2, ch.ID, "gpt-4o-2024-11-20")

	_, err := client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(true).
		SetConsecutiveFailures(1).
		Save(ctx)
	require.NoError(t, err)

	svc := &ChannelProbeService{
		AbstractService:         &AbstractService{db: client},
		ModelProbeConfigService: NewModelProbeConfigService(client),
	}

	target := ModelHealthProbeTarget{
		DisplayModel:  "gpt-4o",
		ChannelID:     ch.ID,
		ActualModelID: "gpt-4o-2024-11-20",
	}
	err = svc.persistModelHealthResult(ctx, target, false, 1700000000, false)
	require.NoError(t, err)

	cfg, err := svc.ModelProbeConfigService.Get(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20")
	require.NoError(t, err)
	require.Equal(t, 2, cfg.ConsecutiveFailures)
	require.False(t, cfg.ProbeEnabled, "at threshold the triple must be auto-disabled")
	require.NotNil(t, cfg.AutoDisabledAt)
}
