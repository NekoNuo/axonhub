package biz

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/objects"
)

func setupProbeConfigTestClient(t *testing.T) (*ent.Client, context.Context) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
	t.Cleanup(func() { _ = client.Close() })
	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))

	return client, ctx
}

func createProbeTestChannel(t *testing.T, ctx context.Context, client *ent.Client, name string) *ent.Channel {
	t.Helper()
	ch, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName(name).
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-4o-2024-11-20"}).
		SetDefaultTestModel("gpt-4o-2024-11-20").
		Save(ctx)
	require.NoError(t, err)

	return ch
}

func createProbeTestModel(t *testing.T, ctx context.Context, client *ent.Client, modelID string, channelID int, actualModelID string) *ent.Model {
	t.Helper()
	m, err := client.Model.Create().
		SetDeveloper("openai").
		SetModelID(modelID).
		SetType(model.TypeChat).
		SetName(modelID).
		SetIcon("openai").
		SetGroup("openai").
		SetStatus(model.StatusEnabled).
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{
			ProbeEnabled: true,
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

	return m
}

func TestModelProbeConfigService_SetProbeEnabled_CreatesWhenMissing(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")

	svc := NewModelProbeConfigService(client)
	cfg, err := svc.SetProbeEnabled(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20", true)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.True(t, cfg.ProbeEnabled)
	require.Equal(t, 0, cfg.ConsecutiveFailures)
	require.Nil(t, cfg.AutoDisabledAt)
}

func TestModelProbeConfigService_SetProbeEnabled_ResetsCountersOnEnable(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")

	ts := int64(1_700_000_000)
	_, err := client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(false).
		SetConsecutiveFailures(3).
		SetAutoDisabledAt(ts).
		Save(ctx)
	require.NoError(t, err)

	svc := NewModelProbeConfigService(client)
	cfg, err := svc.SetProbeEnabled(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20", true)
	require.NoError(t, err)
	require.True(t, cfg.ProbeEnabled)
	require.Equal(t, 0, cfg.ConsecutiveFailures)
	require.Nil(t, cfg.AutoDisabledAt)
}

func TestModelProbeConfigService_SetProbeEnabled_DisableDoesNotResetCounters(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")

	_, err := client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(true).
		SetConsecutiveFailures(2).
		Save(ctx)
	require.NoError(t, err)

	svc := NewModelProbeConfigService(client)
	cfg, err := svc.SetProbeEnabled(ctx, "gpt-4o", ch.ID, "gpt-4o-2024-11-20", false)
	require.NoError(t, err)
	require.False(t, cfg.ProbeEnabled)
	require.Equal(t, 2, cfg.ConsecutiveFailures)
}

func TestModelProbeConfigService_BatchSetChannelProbeEnabled_UpsertsFromAssociations(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")
	createProbeTestModel(t, ctx, client, "gpt-4o", ch.ID, "gpt-4o-2024-11-20")

	svc := NewModelProbeConfigService(client)
	cfgs, err := svc.BatchSetChannelProbeEnabled(ctx, "gpt-4o", ch.ID, true)
	require.NoError(t, err)
	require.Len(t, cfgs, 1)
	require.True(t, cfgs[0].ProbeEnabled)
	require.Equal(t, "gpt-4o-2024-11-20", cfgs[0].ActualModelID)
}

func TestModelProbeConfigService_BatchSetChannelProbeEnabled_WorksForDisabledChannel(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")
	createProbeTestModel(t, ctx, client, "gpt-4o", ch.ID, "gpt-4o-2024-11-20")

	_, err := client.Channel.UpdateOneID(ch.ID).SetStatus("disabled").Save(ctx)
	require.NoError(t, err)

	svc := NewModelProbeConfigService(client)
	cfgs, err := svc.BatchSetChannelProbeEnabled(ctx, "gpt-4o", ch.ID, true)
	require.NoError(t, err)
	require.Len(t, cfgs, 1)
	require.True(t, cfgs[0].ProbeEnabled)
	require.Equal(t, "gpt-4o-2024-11-20", cfgs[0].ActualModelID)
}

func TestModelProbeConfigService_BatchSetChannelProbeEnabled_WorksForDisabledModel(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")
	m := createProbeTestModel(t, ctx, client, "gpt-4o", ch.ID, "gpt-4o-2024-11-20")

	_, err := client.Model.UpdateOneID(m.ID).SetStatus(model.StatusDisabled).Save(ctx)
	require.NoError(t, err)

	svc := NewModelProbeConfigService(client)
	cfgs, err := svc.BatchSetChannelProbeEnabled(ctx, "gpt-4o", ch.ID, true)
	require.NoError(t, err)
	require.Len(t, cfgs, 1)
	require.True(t, cfgs[0].ProbeEnabled)
	require.Equal(t, "gpt-4o-2024-11-20", cfgs[0].ActualModelID)
}

func TestModelProbeConfigService_GetByDisplayModels_Filters(t *testing.T) {
	client, ctx := setupProbeConfigTestClient(t)
	ch := createProbeTestChannel(t, ctx, client, "ch-a")

	_, err := client.ModelProbeConfig.Create().
		SetDisplayModel("gpt-4o").
		SetChannelID(ch.ID).
		SetActualModelID("gpt-4o-2024-11-20").
		SetProbeEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ModelProbeConfig.Create().
		SetDisplayModel("claude-3-5-sonnet").
		SetChannelID(ch.ID).
		SetActualModelID("claude-3-5-sonnet-20241022").
		SetProbeEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := NewModelProbeConfigService(client)
	got, err := svc.GetByDisplayModels(ctx, []string{"gpt-4o"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "gpt-4o", got[0].DisplayModel)

	all, err := svc.GetByDisplayModels(ctx, nil)
	require.NoError(t, err)
	require.Len(t, all, 2)
}
