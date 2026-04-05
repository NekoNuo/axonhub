package biz

import (
	"context"
	"sync"
	"sync/atomic"
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

	_, err = client.Project.Create().
		SetName("default").
		SetDescription("default project").
		SetStatus("active").
		Save(ctx)
	require.NoError(t, err)

	now := time.Date(2026, 4, 5, 11, 0, 0, 0, time.UTC)

	req, err := client.Request.Create().
		SetProjectID(1).
		SetSource("api").
		SetModelID("gpt-4o").
		SetFormat("openai/chat_completions").
		SetRequestBody([]byte(`{}`)).
		SetStatus("completed").
		SetStream(false).
		SetClientIP("127.0.0.1").
		SetCreatedAt(now.Add(-10 * time.Minute)).
		SetUpdatedAt(now.Add(-10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(req.ID).
		SetProjectID(1).
		SetChannelID(channelEntity.ID).
		SetModelID("gpt-4o-2024-11-20").
		SetPromptTokens(1).
		SetCompletionTokens(1).
		SetTotalTokens(2).
		SetSource("api").
		SetFormat("openai/chat_completions").
		SetCreatedAt(now.Add(-10 * time.Minute)).
		SetUpdatedAt(now.Add(-10 * time.Minute)).
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

func TestModelHealthProbe_LimitsConcurrency(t *testing.T) {
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

	channelEntities := make([]*ent.Channel, 0, 4)
	enabledChannels := make([]*Channel, 0, 4)
	for i := 0; i < 4; i++ {
		channelEntity, createErr := client.Channel.Create().
			SetType("openai").
			SetBaseURL("https://api.openai.com/v1").
			SetName("OpenAI Channel " + string(rune('A'+i))).
			SetStatus("enabled").
			SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
			SetSupportedModels([]string{"gpt-4o-2024-11-20"}).
			SetDefaultTestModel("gpt-4o-2024-11-20").
			Save(ctx)
		require.NoError(t, createErr)
		channelEntities = append(channelEntities, channelEntity)
	}

	channelService := NewChannelServiceForTest(client)
	for _, channelEntity := range channelEntities {
		enabledChannel, buildErr := channelService.buildChannelWithTransformer(channelEntity)
		require.NoError(t, buildErr)
		enabledChannels = append(enabledChannels, enabledChannel)
	}
	channelService.SetEnabledChannelsForTest(enabledChannels)

	_, err = client.Project.Create().
		SetName("default").
		SetDescription("default project").
		SetStatus("active").
		Save(ctx)
	require.NoError(t, err)

	for i, channelEntity := range channelEntities {
		modelID := "gpt-4o-" + string(rune('a'+i))
		_, createErr := client.Model.Create().
			SetDeveloper("openai").
			SetModelID(modelID).
			SetType(model.TypeChat).
			SetName("GPT-4o " + string(rune('A'+i))).
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
		require.NoError(t, createErr)

		req, createErr := client.Request.Create().
			SetProjectID(1).
			SetSource("api").
			SetModelID(modelID).
			SetFormat("openai/chat_completions").
			SetRequestBody([]byte(`{}`)).
			SetStatus("completed").
			SetStream(false).
			SetClientIP("127.0.0.1").
			SetCreatedAt(time.Date(2026, 4, 5, 10, 50, 0, 0, time.UTC)).
			SetUpdatedAt(time.Date(2026, 4, 5, 10, 50, 0, 0, time.UTC)).
			Save(ctx)
		require.NoError(t, createErr)

		_, createErr = client.UsageLog.Create().
			SetRequestID(req.ID).
			SetProjectID(1).
			SetChannelID(channelEntity.ID).
			SetModelID("gpt-4o-2024-11-20").
			SetPromptTokens(1).
			SetCompletionTokens(1).
			SetTotalTokens(2).
			SetSource("api").
			SetFormat("openai/chat_completions").
			SetCreatedAt(time.Date(2026, 4, 5, 10, 50, 0, 0, time.UTC)).
			SetUpdatedAt(time.Date(2026, 4, 5, 10, 50, 0, 0, time.UTC)).
			Save(ctx)
		require.NoError(t, createErr)
	}

	probeService := &ChannelProbeService{
		AbstractService: &AbstractService{db: client},
		SystemService:   systemService,
		ChannelService:  channelService,
	}

	var active int32
	var maxActive int32
	var mu sync.Mutex
	release := make(chan struct{})
	calls := 0
	probeService.idleChannelModelProber = func(_ context.Context, _ *ent.Channel, _ string) (time.Duration, bool, error) {
		current := atomic.AddInt32(&active, 1)
		for {
			observed := atomic.LoadInt32(&maxActive)
			if current <= observed || atomic.CompareAndSwapInt32(&maxActive, observed, current) {
				break
			}
		}
		mu.Lock()
		calls++
		mu.Unlock()
		<-release
		atomic.AddInt32(&active, -1)
		return 50 * time.Millisecond, true, nil
	}

	done := make(chan struct{})
	go func() {
		probeService.runModelHealthProbe(ctx, time.Date(2026, 4, 5, 11, 0, 0, 0, time.UTC))
		close(done)
	}()

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&maxActive) >= 2
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(2), atomic.LoadInt32(&maxActive))

	close(release)
	<-done

	mu.Lock()
	assert.Equal(t, 4, calls)
	mu.Unlock()
}

func TestModelHealthProbe_ProbesRecentlyRequestedUnassociatedModel(t *testing.T) {
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
		SetSupportedModels([]string{"gpt-5-2"}).
		SetDefaultTestModel("gpt-5-2").
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

	now := time.Date(2026, 4, 5, 11, 0, 0, 0, time.UTC)
	req, err := client.Request.Create().
		SetProjectID(1).
		SetSource("api").
		SetModelID("gpt-5-2").
		SetFormat("openai/chat_completions").
		SetRequestBody([]byte(`{}`)).
		SetStatus("completed").
		SetStream(false).
		SetClientIP("127.0.0.1").
		SetCreatedAt(now.Add(-10 * time.Minute)).
		SetUpdatedAt(now.Add(-10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(req.ID).
		SetProjectID(1).
		SetChannelID(channelEntity.ID).
		SetModelID("gpt-5-2").
		SetPromptTokens(1).
		SetCompletionTokens(1).
		SetTotalTokens(2).
		SetSource("api").
		SetFormat("openai/chat_completions").
		SetCreatedAt(now.Add(-10 * time.Minute)).
		SetUpdatedAt(now.Add(-10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	probeService := &ChannelProbeService{
		AbstractService: &AbstractService{db: client},
		SystemService:   systemService,
		ChannelService:  channelService,
	}

	probeCalls := 0
	probeService.idleChannelModelProber = func(_ context.Context, ch *ent.Channel, modelID string) (time.Duration, bool, error) {
		probeCalls++
		require.Equal(t, channelEntity.ID, ch.ID)
		require.Equal(t, "gpt-5-2", modelID)
		return 10 * time.Millisecond, true, nil
	}

	probeService.runModelHealthProbe(ctx, now)

	require.Equal(t, 1, probeCalls)

	snapshot, err := client.ModelHealthSnapshot.Query().Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "gpt-5-2", snapshot.DisplayModel)
	require.Equal(t, "gpt-5-2", snapshot.ActualModelID)
	require.Equal(t, "discovered_recent_usage", snapshot.Source)
}

func TestModelHealthProbe_ProbesAllChannelsSupportingRecentlyRequestedUnassociatedModel(t *testing.T) {
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
		SetSupportedModels([]string{"gpt-5-2"}).
		SetDefaultTestModel("gpt-5-2").
		Save(ctx)
	require.NoError(t, err)

	channelB, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel B").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-5-2"}).
		SetDefaultTestModel("gpt-5-2").
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

	now := time.Date(2026, 4, 5, 11, 0, 0, 0, time.UTC)
	req, err := client.Request.Create().
		SetProjectID(1).
		SetSource("api").
		SetModelID("gpt-5-2").
		SetFormat("openai/chat_completions").
		SetRequestBody([]byte(`{}`)).
		SetStatus("completed").
		SetStream(false).
		SetClientIP("127.0.0.1").
		SetCreatedAt(now.Add(-10 * time.Minute)).
		SetUpdatedAt(now.Add(-10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(req.ID).
		SetProjectID(1).
		SetChannelID(channelA.ID).
		SetModelID("gpt-5-2").
		SetPromptTokens(1).
		SetCompletionTokens(1).
		SetTotalTokens(2).
		SetSource("api").
		SetFormat("openai/chat_completions").
		SetCreatedAt(now.Add(-10 * time.Minute)).
		SetUpdatedAt(now.Add(-10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	probeService := &ChannelProbeService{
		AbstractService: &AbstractService{db: client},
		SystemService:   systemService,
		ChannelService:  channelService,
	}

	called := map[int]int{}
	probeService.idleChannelModelProber = func(_ context.Context, ch *ent.Channel, modelID string) (time.Duration, bool, error) {
		called[ch.ID]++
		require.Equal(t, "gpt-5-2", modelID)
		return 10 * time.Millisecond, true, nil
	}

	probeService.runModelHealthProbe(ctx, now)

	require.Equal(t, 1, called[channelA.ID])
	require.Equal(t, 1, called[channelB.ID])

	snapshots, err := client.ModelHealthSnapshot.Query().
		Order(ent.Asc(modelhealthsnapshot.FieldChannelID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, snapshots, 2)
	require.Equal(t, channelA.ID, snapshots[0].ChannelID)
	require.Equal(t, channelB.ID, snapshots[1].ChannelID)
}

func TestModelHealthTargetResolver_ExpandsUnassociatedRecentModelToAllSupportingChannels(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))

	channelA, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel A").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-5-2"}).
		SetDefaultTestModel("gpt-5-2").
		Save(ctx)
	require.NoError(t, err)

	channelB, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel B").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-5-2"}).
		SetDefaultTestModel("gpt-5-2").
		Save(ctx)
	require.NoError(t, err)

	targets, err := NewModelHealthTargetResolver(client, nil).Resolve(ctx, []ModelHealthRecentUsage{
		{
			DisplayModel: "gpt-5-2",
			ActualModels: []ModelHealthRecentActualModel{
				{
					ActualModelID: "gpt-5-2",
					ChannelIDs:    []int{channelA.ID},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Equal(t, []int{channelA.ID, channelB.ID}, []int{targets[0].ChannelID, targets[1].ChannelID})
}

func TestModelHealthTargetResolver_UsesRecentActualModelsWhenProbeDisabled(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))

	channelA, err := client.Channel.Create().
		SetType("openai").
		SetBaseURL("https://api.openai.com/v1").
		SetName("OpenAI Channel A").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
		SetSupportedModels([]string{"gpt-5-2", "gpt-5-2-2026-04-01"}).
		SetDefaultTestModel("gpt-5-2").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.Model.Create().
		SetDeveloper("openai").
		SetModelID("gpt-5-2").
		SetType(model.TypeChat).
		SetName("GPT-5.2").
		SetIcon("openai").
		SetGroup("openai").
		SetStatus(model.StatusEnabled).
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{
			ProbeEnabled: false,
			Associations: []*objects.ModelAssociation{
				{
					Type: "channel_model",
					ChannelModel: &objects.ChannelModelAssociation{
						ChannelID: channelA.ID,
						ModelID:   "gpt-5-2",
					},
				},
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	targets, err := NewModelHealthTargetResolver(client, nil).Resolve(ctx, []ModelHealthRecentUsage{
		{
			DisplayModel: "gpt-5-2",
			ActualModels: []ModelHealthRecentActualModel{
				{
					ActualModelID: "gpt-5-2-2026-04-01",
					ChannelIDs:    []int{channelA.ID},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Contains(t, targets, ModelHealthProbeTarget{
		DisplayModel:  "gpt-5-2",
		ActualModelID: "gpt-5-2-2026-04-01",
		ChannelID:     channelA.ID,
		Sources:       []string{ModelHealthTargetSourceRecent},
		Source:        ModelHealthSourceDiscoveredRecentUsage,
	})
}

func TestGetEffectiveModelHealthFromDB_FallsBackToDiscoveredSnapshot(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))

	_, err := client.ModelHealthSnapshot.Create().
		SetDisplayModel("gpt-5-2").
		SetChannelID(1).
		SetActualModelID("gpt-5-2-2026-04-01").
		SetSource(ModelHealthSourceDiscoveredRecentUsage).
		SetIsHealthy(true).
		SetManualOverride(false).
		SetProbedAt(time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC).Unix()).
		Save(ctx)
	require.NoError(t, err)

	view, err := getEffectiveModelHealthFromDB(ctx, client, "gpt-5-2", 1, "gpt-5-2-2026-04-01")
	require.NoError(t, err)
	require.NotNil(t, view)
	require.True(t, view.IsHealthy)
	require.False(t, view.ManualOverride)
	require.Equal(t, ModelHealthSourceDiscoveredRecentUsage, view.Source)
}
