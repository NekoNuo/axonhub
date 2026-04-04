package biz

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/ent/modelhealthhistory"
	"github.com/looplj/axonhub/internal/ent/modelhealthsnapshot"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
)

func TestModelHealthProbe(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))

	systemService := NewSystemService(SystemServiceParams{Ent: client, CacheConfig: xcache.Config{Mode: xcache.ModeMemory}})
	err := systemService.SetModelSettings(ctx, SystemModelSettings{
		EnableModelProbe:                  true,
		FallbackToChannelsOnModelNotFound: true,
		QueryAllChannelModels:             true,
	})
	require.NoError(t, err)

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

	channelService := NewChannelServiceForTest(client)
	enabledChannel, err := channelService.buildChannelWithTransformer(channelEntity)
	require.NoError(t, err)
	channelService.SetEnabledChannelsForTest([]*Channel{enabledChannel})

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

	probeService := &ChannelProbeService{
		AbstractService: &AbstractService{db: client},
		SystemService:   systemService,
		ChannelService:  channelService,
	}

	modelProbeCalls := 0
	probeService.idleChannelModelProber = func(_ context.Context, ch *ent.Channel, modelID string) (time.Duration, bool, error) {
		modelProbeCalls++
		require.Equal(t, channelEntity.ID, ch.ID)
		require.Equal(t, "gpt-4o-2024-11-20", modelID)
		return 120 * time.Millisecond, true, nil
	}

	now := time.Date(2026, 4, 5, 11, 0, 0, 0, time.UTC)
	probeService.runModelHealthProbe(ctx, now)

	require.Equal(t, 1, modelProbeCalls)

	snapshot, err := client.ModelHealthSnapshot.Query().Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "gpt-4o", snapshot.DisplayModel)
	require.Equal(t, channelEntity.ID, snapshot.ChannelID)
	require.Equal(t, "gpt-4o-2024-11-20", snapshot.ActualModelID)
	require.True(t, snapshot.IsHealthy)
	require.False(t, snapshot.ManualOverride)
	require.Equal(t, now.Unix(), snapshot.ProbedAt)

	history, err := client.ModelHealthHistory.Query().
		Order(ent.Asc(modelhealthhistory.FieldProbedAt)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.True(t, history[0].IsHealthy)
	require.False(t, history[0].ManualOverride)

	_, err = client.ModelHealthSnapshot.UpdateOneID(snapshot.ID).
		SetIsHealthy(false).
		SetManualOverride(true).
		Save(ctx)
	require.NoError(t, err)

	probeService.idleChannelModelProber = func(_ context.Context, _ *ent.Channel, _ string) (time.Duration, bool, error) {
		return 80 * time.Millisecond, true, nil
	}

	next := now.Add(5 * time.Minute)
	probeService.runModelHealthProbe(ctx, next)

	updatedSnapshot, err := client.ModelHealthSnapshot.Query().
		Where(modelhealthsnapshot.IDEQ(snapshot.ID)).
		Only(ctx)
	require.NoError(t, err)
	require.True(t, updatedSnapshot.IsHealthy)
	require.False(t, updatedSnapshot.ManualOverride)
	require.Equal(t, next.Unix(), updatedSnapshot.ProbedAt)

	history, err = client.ModelHealthHistory.Query().
		Order(ent.Asc(modelhealthhistory.FieldProbedAt)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, history, 2)
	assert.Equal(t, []bool{false, false}, []bool{history[0].ManualOverride, history[1].ManualOverride})
	assert.Equal(t, []int64{now.Unix(), next.Unix()}, []int64{history[0].ProbedAt, history[1].ProbedAt})
}
