package biz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterAutomaticProbeTargets_SkipsMissingConfig(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)

	svc := &ChannelProbeService{
		AbstractService:         &AbstractService{db: client},
		ModelProbeConfigService: NewModelProbeConfigService(client),
	}

	targets := []ModelHealthProbeTarget{
		{DisplayModel: "gpt-4o", ChannelID: 1, ActualModelID: "gpt-4o-2024-11-20"},
	}

	filtered := svc.filterAutomaticProbeTargets(ctx, targets)
	require.Empty(t, filtered, "targets with no config row should be skipped")
}

func TestFilterAutomaticProbeTargets_SkipsDisabled(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")

	_, err := client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(false).
		Save(ctx)
	require.NoError(t, err)

	svc := &ChannelProbeService{
		AbstractService:         &AbstractService{db: client},
		ModelProbeConfigService: NewModelProbeConfigService(client),
	}

	targets := []ModelHealthProbeTarget{
		{DisplayModel: "gpt-4o", ChannelID: ch.ID, ActualModelID: "gpt-4o-2024-11-20"},
	}

	filtered := svc.filterAutomaticProbeTargets(ctx, targets)
	require.Empty(t, filtered)
}

func TestFilterAutomaticProbeTargets_KeepsEnabled(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")

	_, err := client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &ChannelProbeService{
		AbstractService:         &AbstractService{db: client},
		ModelProbeConfigService: NewModelProbeConfigService(client),
	}

	targets := []ModelHealthProbeTarget{
		{DisplayModel: "gpt-4o", ChannelID: ch.ID, ActualModelID: "gpt-4o-2024-11-20"},
		{DisplayModel: "gpt-4o", ChannelID: ch.ID, ActualModelID: "gpt-4o-mini"},
	}

	filtered := svc.filterAutomaticProbeTargets(ctx, targets)
	require.Len(t, filtered, 1)
	require.Equal(t, "gpt-4o-2024-11-20", filtered[0].ActualModelID)
}

func TestFilterAutomaticProbeTargets_NilServicePassThrough(t *testing.T) {
	svc := &ChannelProbeService{}
	targets := []ModelHealthProbeTarget{{DisplayModel: "x", ChannelID: 1, ActualModelID: "y"}}
	require.Equal(t, targets, svc.filterAutomaticProbeTargets(context.Background(), targets))
}
