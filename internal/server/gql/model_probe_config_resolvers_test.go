package gql

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
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm/httpclient"
)

func setupProbeConfigResolverTest(t *testing.T) (*Resolver, context.Context, *ent.Client) {
	t.Helper()

	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
	t.Cleanup(func() { _ = client.Close() })
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

	return resolver, ctx, client
}

func seedProbeConfigChannel(t *testing.T, ctx context.Context, client *ent.Client) *ent.Channel {
	t.Helper()
	ch, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-4o-2024-11-20"}).
		SetDefaultTestModel("gpt-4o-2024-11-20").
		Save(ctx)
	require.NoError(t, err)

	return ch
}

func seedProbeConfigModel(t *testing.T, ctx context.Context, client *ent.Client, modelID string, channelID int, actualModelID string) {
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
}

func TestModelProbeConfigs_Query_FiltersByDisplayModel(t *testing.T) {
	resolver, ctx, client := setupProbeConfigResolverTest(t)
	ch := seedProbeConfigChannel(t, ctx, client)

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

	q := resolver.Query()
	got, err := q.ModelProbeConfigs(ctx, GetModelProbeConfigsInput{DisplayModels: []string{"gpt-4o"}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "gpt-4o", got[0].DisplayModel)
}

func TestSetModelProbeEnabled_Mutation_CreatesOrUpdates(t *testing.T) {
	resolver, ctx, client := setupProbeConfigResolverTest(t)
	ch := seedProbeConfigChannel(t, ctx, client)

	m := resolver.Mutation()
	cfg, err := m.SetModelProbeEnabled(ctx, SetModelProbeEnabledInput{
		DisplayModel:  "gpt-4o",
		ChannelID:     objects.GUID{Type: "Channel", ID: ch.ID},
		ActualModelID: "gpt-4o-2024-11-20",
		Enabled:       true,
	})
	require.NoError(t, err)
	require.True(t, cfg.ProbeEnabled)

	cfg2, err := m.SetModelProbeEnabled(ctx, SetModelProbeEnabledInput{
		DisplayModel:  "gpt-4o",
		ChannelID:     objects.GUID{Type: "Channel", ID: ch.ID},
		ActualModelID: "gpt-4o-2024-11-20",
		Enabled:       false,
	})
	require.NoError(t, err)
	require.False(t, cfg2.ProbeEnabled)
	require.Equal(t, cfg.ID, cfg2.ID, "same row updated, not duplicated")
}

func TestBatchSetChannelProbeEnabled_Mutation_UpsertsFromAssociations(t *testing.T) {
	resolver, ctx, client := setupProbeConfigResolverTest(t)
	ch := seedProbeConfigChannel(t, ctx, client)
	seedProbeConfigModel(t, ctx, client, "gpt-4o", ch.ID, "gpt-4o-2024-11-20")

	m := resolver.Mutation()
	cfgs, err := m.BatchSetChannelProbeEnabled(ctx, BatchSetChannelProbeEnabledInput{
		DisplayModel: "gpt-4o",
		ChannelID:    objects.GUID{Type: "Channel", ID: ch.ID},
		Enabled:      true,
	})
	require.NoError(t, err)
	require.Len(t, cfgs, 1)
	require.True(t, cfgs[0].ProbeEnabled)
}
