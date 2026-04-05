package gql

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm/httpclient"
)

func setupModelHealthResolverTest(t *testing.T) (*Resolver, context.Context, *ent.Client, *biz.ChannelProbeService) {
	t.Helper()

	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))

	systemService := biz.NewSystemService(biz.SystemServiceParams{Ent: client, CacheConfig: xcache.Config{Mode: xcache.ModeMemory}})
	channelService := biz.NewChannelServiceForTest(client)
	probeService := biz.NewChannelProbeService(biz.ChannelProbeServiceParams{
		Ent:            client,
		SystemService:  systemService,
		ChannelService: channelService,
		HttpClient:     httpclient.NewHttpClient(),
	})

	resolver := &Resolver{
		client:              client,
		channelProbeService: probeService,
	}

	return resolver, ctx, client, probeService
}

func seedModelHealthData(t *testing.T, ctx context.Context, client *ent.Client, probeService *biz.ChannelProbeService) (*ent.Channel, biz.ModelHealthProbeTarget) {
	t.Helper()

	channelEntity, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-4o-2024-11-20"}).
		SetDefaultTestModel("gpt-4o-2024-11-20").
		Save(ctx)
	require.NoError(t, err)

	channelService := probeService.ChannelService
	channelService.SetEnabledChannelsForTest([]*biz.Channel{{Channel: channelEntity}})

	_, err = client.Model.Create().
		SetDeveloper("openai").
		SetModelID("gpt-4o").
		SetType(model.TypeChat).
		SetName("GPT-4o").
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
						ChannelID: channelEntity.ID,
						ModelID:   "gpt-4o-2024-11-20",
					},
				},
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	target := biz.ModelHealthProbeTarget{
		DisplayModel:  "gpt-4o",
		ActualModelID: "gpt-4o-2024-11-20",
		ChannelID:     channelEntity.ID,
	}

	return channelEntity, target
}

func TestQueryResolver_ModelHealthSnapshots(t *testing.T) {
	resolver, ctx, client, probeService := setupModelHealthResolverTest(t)
	defer client.Close()

	_, target := seedModelHealthData(t, ctx, client, probeService)

	_, err := client.ModelHealthSnapshot.Create().
		SetDisplayModel(target.DisplayModel).
		SetChannelID(target.ChannelID).
		SetActualModelID(target.ActualModelID).
		SetIsHealthy(true).
		SetManualOverride(false).
		SetProbedAt(time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC).Unix()).
		Save(ctx)
	require.NoError(t, err)

	query := &queryResolver{resolver}
	rows, err := query.ModelHealthSnapshots(ctx, GetModelHealthSnapshotsInput{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, target.DisplayModel, rows[0].DisplayModel)
}

func TestQueryResolver_ModelHealthSnapshotsUsesEffectiveHealth(t *testing.T) {
	resolver, ctx, client, probeService := setupModelHealthResolverTest(t)
	defer client.Close()

	_, target := seedModelHealthData(t, ctx, client, probeService)

	_, err := client.ModelHealthSnapshot.Create().
		SetDisplayModel(target.DisplayModel).
		SetChannelID(target.ChannelID).
		SetActualModelID(target.ActualModelID).
		SetIsHealthy(true).
		SetManualOverride(true).
		SetProbedAt(time.Date(2026, 4, 5, 10, 1, 0, 0, time.UTC).Unix()).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ModelHealthHistory.Create().
		SetDisplayModel(target.DisplayModel).
		SetChannelID(target.ChannelID).
		SetActualModelID(target.ActualModelID).
		SetIsHealthy(true).
		SetManualOverride(true).
		SetProbedAt(time.Date(2026, 4, 5, 10, 1, 0, 0, time.UTC).Unix()).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ModelHealthHistory.Create().
		SetDisplayModel(target.DisplayModel).
		SetChannelID(target.ChannelID).
		SetActualModelID(target.ActualModelID).
		SetIsHealthy(false).
		SetManualOverride(false).
		SetProbedAt(time.Date(2026, 4, 5, 10, 2, 0, 0, time.UTC).Unix()).
		Save(ctx)
	require.NoError(t, err)

	query := &queryResolver{resolver}
	rows, err := query.ModelHealthSnapshots(ctx, GetModelHealthSnapshotsInput{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.False(t, rows[0].IsHealthy)
	require.False(t, rows[0].ManualOverride)
	require.Equal(t, time.Date(2026, 4, 5, 10, 2, 0, 0, time.UTC).Unix(), rows[0].ProbedAt)
}

func TestMutationResolver_ManualModelProbe(t *testing.T) {
	resolver, ctx, client, probeService := setupModelHealthResolverTest(t)
	defer client.Close()

	channelEntity, target := seedModelHealthData(t, ctx, client, probeService)

	probeService.SetIdleChannelModelProber(func(_ context.Context, ch *ent.Channel, modelID string) (time.Duration, bool, error) {
		require.Equal(t, channelEntity.ID, ch.ID)
		require.Equal(t, target.ActualModelID, modelID)
		return 10 * time.Millisecond, true, nil
	})

	mutation := &mutationResolver{resolver}
	ok, err := mutation.ManualModelProbe(ctx, ManualModelProbeInput{
		DisplayModel:  target.DisplayModel,
		ActualModelID: target.ActualModelID,
		ChannelID: objects.GUID{
			Type: ent.TypeChannel,
			ID:   target.ChannelID,
		},
	})
	require.NoError(t, err)
	require.True(t, ok)

	snapshot, err := client.ModelHealthSnapshot.Query().Only(ctx)
	require.NoError(t, err)
	require.True(t, snapshot.ManualOverride)
	require.True(t, snapshot.IsHealthy)
}

func TestQueryResolver_DiscoveredModelHealthSnapshotsFiltersExpiredRows(t *testing.T) {
	resolver, ctx, client, _ := setupModelHealthResolverTest(t)
	defer client.Close()

	channelEntity, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-5-2"}).
		SetDefaultTestModel("gpt-5-2").
		Save(ctx)
	require.NoError(t, err)

	now := time.Now().UTC().Unix()

	_, err = client.ModelHealthSnapshot.Create().
		SetDisplayModel("gpt-5-2").
		SetChannelID(channelEntity.ID).
		SetActualModelID("gpt-5-2").
		SetSource("discovered_recent_usage").
		SetIsHealthy(true).
		SetManualOverride(false).
		SetProbedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ModelHealthSnapshot.Create().
		SetDisplayModel("gpt-5-1").
		SetChannelID(channelEntity.ID).
		SetActualModelID("gpt-5-1").
		SetSource("discovered_recent_usage").
		SetIsHealthy(true).
		SetManualOverride(false).
		SetProbedAt(now - (25 * 60 * 60)).
		Save(ctx)
	require.NoError(t, err)

	query := &queryResolver{resolver}
	rows, err := query.DiscoveredModelHealthSnapshots(ctx, GetDiscoveredModelHealthSnapshotsInput{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "gpt-5-2", rows[0].DisplayModel)
}

func TestQueryResolver_DiscoveredModelHealthSnapshotsExcludesProbeEnabledDisplayModel(t *testing.T) {
	resolver, ctx, client, _ := setupModelHealthResolverTest(t)
	defer client.Close()

	channelEntity, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-5.4-2026-03-05"}).
		SetDefaultTestModel("gpt-5.4-2026-03-05").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.Model.Create().
		SetDeveloper("openai").
		SetModelID("gpt-5-4").
		SetType(model.TypeChat).
		SetName("GPT-5.4").
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
						ChannelID: channelEntity.ID,
						ModelID:   "gpt-5.4-2026-03-05",
					},
				},
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ModelHealthSnapshot.Create().
		SetDisplayModel("gpt-5-4").
		SetChannelID(channelEntity.ID).
		SetActualModelID("gpt-5.4-2026-03-05").
		SetSource("discovered_recent_usage").
		SetIsHealthy(true).
		SetManualOverride(false).
		SetProbedAt(time.Now().UTC().Unix()).
		Save(ctx)
	require.NoError(t, err)

	query := &queryResolver{resolver}
	rows, err := query.DiscoveredModelHealthSnapshots(ctx, GetDiscoveredModelHealthSnapshotsInput{})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestListProbeEnabledDisplayModels(t *testing.T) {
	resolver, ctx, client, _ := setupModelHealthResolverTest(t)
	_ = resolver
	defer client.Close()

	channelEntity, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-5.4-2026-03-05"}).
		SetDefaultTestModel("gpt-5.4-2026-03-05").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.Model.Create().
		SetDeveloper("openai").
		SetModelID("gpt-5-4").
		SetType(model.TypeChat).
		SetName("GPT-5.4").
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
						ChannelID: channelEntity.ID,
						ModelID:   "gpt-5.4-2026-03-05",
					},
				},
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	keys, err := listProbeEnabledDisplayModels(ctx, client)
	require.NoError(t, err)
	_, exists := keys["gpt-5-4"]
	require.True(t, exists)
}

func TestQueryResolver_DiscoveredModelHealthSnapshotsKeepsDifferentDisplayModel(t *testing.T) {
	resolver, ctx, client, _ := setupModelHealthResolverTest(t)
	defer client.Close()

	channelEntity, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-4.1-mini"}).
		SetDefaultTestModel("gpt-4.1-mini").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.Model.Create().
		SetDeveloper("openai").
		SetModelID("some-other-display-model").
		SetType(model.TypeChat).
		SetName("Other Display Model").
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
						ChannelID: channelEntity.ID,
						ModelID:   "gpt-4.1-mini",
					},
				},
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ModelHealthSnapshot.Create().
		SetDisplayModel("gpt-4.1-mini").
		SetChannelID(channelEntity.ID).
		SetActualModelID("gpt-4.1-mini").
		SetSource("discovered_recent_usage").
		SetIsHealthy(true).
		SetManualOverride(false).
		SetProbedAt(time.Now().UTC().Unix()).
		Save(ctx)
	require.NoError(t, err)

	query := &queryResolver{resolver}
	rows, err := query.DiscoveredModelHealthSnapshots(ctx, GetDiscoveredModelHealthSnapshotsInput{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "gpt-4.1-mini", rows[0].DisplayModel)
}
