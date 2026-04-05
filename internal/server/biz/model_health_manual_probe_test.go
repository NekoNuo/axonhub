package biz

import (
	"context"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/ent/modelhealthhistory"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
)

func TestModelHealthManualProbe(t *testing.T) {
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

	target := ModelHealthProbeTarget{
		DisplayModel:  "gpt-4o",
		ActualModelID: "gpt-4o-2024-11-20",
		ChannelID:     channelEntity.ID,
	}

	autoTime := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	err = probeService.persistModelHealthResult(ctx, target, false, autoTime.Unix(), false)
	require.NoError(t, err)

	probeService.idleChannelModelProber = func(_ context.Context, _ *ent.Channel, _ string) (time.Duration, bool, error) {
		return 50 * time.Millisecond, true, nil
	}

	manualTime := autoTime.Add(2 * time.Minute)
	err = probeService.RunManualModelProbe(ctx, target, manualTime)
	require.NoError(t, err)

	snapshot, err := client.ModelHealthSnapshot.Query().Only(ctx)
	require.NoError(t, err)
	require.True(t, snapshot.IsHealthy)
	require.True(t, snapshot.ManualOverride)
	require.Equal(t, manualTime.Unix(), snapshot.ProbedAt)

	effective, err := probeService.GetEffectiveModelHealth(ctx, target)
	require.NoError(t, err)
	require.NotNil(t, effective)
	require.True(t, effective.IsHealthy)
	require.True(t, effective.ManualOverride)

	autoRefreshTime := manualTime.Add(3 * time.Minute)
	err = probeService.persistModelHealthResult(ctx, target, false, autoRefreshTime.Unix(), false)
	require.NoError(t, err)

	effective, err = probeService.GetEffectiveModelHealth(ctx, target)
	require.NoError(t, err)
	require.NotNil(t, effective)
	require.False(t, effective.IsHealthy)
	require.False(t, effective.ManualOverride)
	require.Equal(t, autoRefreshTime.Unix(), effective.ProbedAt)

	history, err := client.ModelHealthHistory.Query().
		Order(ent.Asc(modelhealthhistory.FieldProbedAt)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, history, 3)
	require.Equal(t, []bool{false, true, false}, []bool{
		history[0].ManualOverride,
		history[1].ManualOverride,
		history[2].ManualOverride,
	})
}

func TestModelHealthManualProbe_PersistsAfterRequestContextTimeout(t *testing.T) {
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
	probeService.idleChannelModelProber = func(_ context.Context, _ *ent.Channel, _ string) (time.Duration, bool, error) {
		time.Sleep(20 * time.Millisecond)
		return 20 * time.Millisecond, true, nil
	}

	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Millisecond)
	defer cancel()

	target := ModelHealthProbeTarget{
		DisplayModel:  "gpt-4o",
		ActualModelID: "gpt-4o-2024-11-20",
		ChannelID:     channelEntity.ID,
	}

	err = probeService.RunManualModelProbe(requestCtx, target, time.Date(2026, 4, 5, 12, 10, 0, 0, time.UTC))
	require.NoError(t, err)

	snapshot, err := client.ModelHealthSnapshot.Query().Only(ctx)
	require.NoError(t, err)
	require.True(t, snapshot.IsHealthy)
	require.True(t, snapshot.ManualOverride)
}

func TestRefreshModelHealthFromRecentUsage_PersistsCurrentChannelImmediatelyAndProbesOthers(t *testing.T) {
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

	channelA, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel A").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-4.1-mini"}).
		SetDefaultTestModel("gpt-4.1-mini").
		Save(ctx)
	require.NoError(t, err)

	channelB, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel B").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-4.1-mini"}).
		SetDefaultTestModel("gpt-4.1-mini").
		Save(ctx)
	require.NoError(t, err)

	channelService := NewChannelServiceForTest(client)
	enabledA, err := channelService.buildChannelWithTransformer(channelA)
	require.NoError(t, err)
	enabledB, err := channelService.buildChannelWithTransformer(channelB)
	require.NoError(t, err)
	channelService.SetEnabledChannelsForTest([]*Channel{enabledA, enabledB})

	_, err = client.Project.Create().
		SetName("default").
		SetDescription("default project").
		SetStatus("active").
		Save(ctx)
	require.NoError(t, err)

	probeService := &ChannelProbeService{
		AbstractService: &AbstractService{db: client},
		SystemService:   systemService,
		ChannelService:  channelService,
	}

	var mu sync.Mutex
	called := make(map[int]int)
	probeService.idleChannelModelProber = func(_ context.Context, ch *ent.Channel, modelID string) (time.Duration, bool, error) {
		require.Equal(t, "gpt-4.1-mini", modelID)
		mu.Lock()
		called[ch.ID]++
		mu.Unlock()
		return 10 * time.Millisecond, ch.ID == channelB.ID, nil
	}

	req, err := client.Request.Create().
		SetProjectID(1).
		SetSource("api").
		SetModelID("gpt-4.1-mini").
		SetFormat("openai/chat_completions").
		SetRequestBody([]byte(`{}`)).
		SetStatus("completed").
		SetStream(false).
		SetClientIP("127.0.0.1").
		Save(ctx)
	require.NoError(t, err)

	reqExec, err := client.RequestExecution.Create().
		SetRequestID(req.ID).
		SetChannelID(channelA.ID).
		SetModelID("gpt-4.1-mini").
		SetStatus("completed").
		SetRequestBody([]byte(`{}`)).
		Save(ctx)
	require.NoError(t, err)

	now := time.Date(2026, 4, 5, 13, 0, 0, 0, time.UTC)
	currentTarget := ModelHealthProbeTarget{
		DisplayModel:  req.ModelID,
		ActualModelID: reqExec.ModelID,
		ChannelID:     reqExec.ChannelID,
		Source:        ModelHealthSourceDiscoveredRecentUsage,
		Sources:       []string{ModelHealthTargetSourceRecent},
	}
	err = probeService.persistModelHealthResult(ctx, currentTarget, true, now.Unix(), false)
	require.NoError(t, err)

	probeService.refreshModelHealthFromRecentUsage(ctx, req, reqExec, now.Unix(), currentTarget)

	snapshots, err := client.ModelHealthSnapshot.Query().
		Order(ent.Asc("channel_id")).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, snapshots, 2)
	require.Equal(t, channelA.ID, snapshots[0].ChannelID)
	require.True(t, snapshots[0].IsHealthy)
	require.Equal(t, channelB.ID, snapshots[1].ChannelID)
	require.True(t, snapshots[1].IsHealthy)
	require.Equal(t, ModelHealthSourceDiscoveredRecentUsage, snapshots[0].Source)
	require.Equal(t, ModelHealthSourceDiscoveredRecentUsage, snapshots[1].Source)

	mu.Lock()
	require.Equal(t, 0, called[channelA.ID])
	require.Equal(t, 1, called[channelB.ID])
	mu.Unlock()
}

func TestRefreshModelHealthFromUsage_SkipsWhenHealthAlreadyExists(t *testing.T) {
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
		SetSupportedModels([]string{"gpt-4.1-mini"}).
		SetDefaultTestModel("gpt-4.1-mini").
		Save(ctx)
	require.NoError(t, err)

	channelService := NewChannelServiceForTest(client)
	enabledChannel, err := channelService.buildChannelWithTransformer(channelEntity)
	require.NoError(t, err)
	channelService.SetEnabledChannelsForTest([]*Channel{enabledChannel})

	_, err = client.Project.Create().
		SetName("default").
		SetDescription("default project").
		SetStatus("active").
		Save(ctx)
	require.NoError(t, err)

	probeService := &ChannelProbeService{
		AbstractService: &AbstractService{db: client},
		SystemService:   systemService,
		ChannelService:  channelService,
	}

	called := 0
	probeService.idleChannelModelProber = func(_ context.Context, _ *ent.Channel, _ string) (time.Duration, bool, error) {
		called++
		return 10 * time.Millisecond, true, nil
	}

	req, err := client.Request.Create().
		SetProjectID(1).
		SetSource("api").
		SetModelID("gpt-4.1-mini").
		SetFormat("openai/chat_completions").
		SetRequestBody([]byte(`{}`)).
		SetStatus("completed").
		SetStream(false).
		SetClientIP("127.0.0.1").
		Save(ctx)
	require.NoError(t, err)

	reqExec, err := client.RequestExecution.Create().
		SetRequestID(req.ID).
		SetChannelID(channelEntity.ID).
		SetModelID("gpt-4.1-mini").
		SetStatus("completed").
		SetRequestBody([]byte(`{}`)).
		Save(ctx)
	require.NoError(t, err)

	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	err = probeService.persistModelHealthResult(ctx, ModelHealthProbeTarget{
		DisplayModel:  req.ModelID,
		ActualModelID: reqExec.ModelID,
		ChannelID:     reqExec.ChannelID,
		Source:        ModelHealthSourceDiscoveredRecentUsage,
	}, true, now.Add(-time.Minute).Unix(), false)
	require.NoError(t, err)

	probeService.RefreshModelHealthFromUsage(ctx, req, reqExec, now)

	time.Sleep(50 * time.Millisecond)

	require.Equal(t, 0, called)

	history, err := client.ModelHealthHistory.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, history, 1)
}
