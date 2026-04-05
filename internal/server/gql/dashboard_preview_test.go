package gql

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/zhenzou/executors"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/internal/server/biz"
)

func TestBuildLoadBalancerPreviewSelectsHottestModelAndBuildsRetrySteps(t *testing.T) {
	ctx := authz.WithTestBypass(context.Background())
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	t.Cleanup(func() { client.Close() })
	ctx = ent.NewContext(ctx, client)

	systemService := biz.NewSystemService(biz.SystemServiceParams{
		CacheConfig: xcache.Config{Mode: xcache.ModeMemory},
		Ent:         client,
	})
	err := systemService.SetRetryPolicy(ctx, &biz.RetryPolicy{
		Enabled:                 true,
		MaxChannelRetries:       2,
		MaxSingleChannelRetries: 1,
		RetryDelayMs:            1000,
		LoadBalancerStrategy:    biz.LoadBalancerStrategyFailover,
	})
	require.NoError(t, err)

	channelService := biz.NewChannelService(biz.ChannelServiceParams{
		CacheConfig:   xcache.Config{Mode: xcache.ModeMemory},
		Executor:      executors.NewPoolScheduleExecutor(),
		Ent:           client,
		SystemService: systemService,
	})
	t.Cleanup(channelService.Stop)

	modelService := biz.NewModelService(biz.ModelServiceParams{Ent: client})

	ch1, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Primary").
		SetBaseURL("https://example.com/1").
		SetCredentials(objects.ChannelCredentials{APIKey: "k1"}).
		SetSupportedModels([]string{"gpt-5.4"}).
		SetDefaultTestModel("gpt-5.4").
		SetOrderingWeight(100).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	ch2, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Retry A").
		SetBaseURL("https://example.com/2").
		SetCredentials(objects.ChannelCredentials{APIKey: "k2"}).
		SetSupportedModels([]string{"gpt-5.4"}).
		SetDefaultTestModel("gpt-5.4").
		SetOrderingWeight(80).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	ch3, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Retry B").
		SetBaseURL("https://example.com/3").
		SetCredentials(objects.ChannelCredentials{APIKey: "k3"}).
		SetSupportedModels([]string{"gpt-5.4"}).
		SetDefaultTestModel("gpt-5.4").
		SetOrderingWeight(60).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	channelService.SetEnabledChannelsForTest([]*biz.Channel{
		{Channel: ch1},
		{Channel: ch2},
		{Channel: ch3},
	})

	_, err = client.Model.Create().
		SetDeveloper("openai").
		SetModelID("gpt-5.4").
		SetType(model.TypeChat).
		SetName("GPT-5.4").
		SetIcon("openai").
		SetGroup("openai").
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{}).
		SetStatus(model.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now()
	for range 8 {
		_, err = client.UsageLog.Create().
			SetRequestID(1).
			SetProjectID(1).
			SetChannelID(ch1.ID).
			SetModelID("gpt-5.4").
			SetCreatedAt(now).
			Save(ctx)
		require.NoError(t, err)
	}
	for range 3 {
		_, err = client.UsageLog.Create().
			SetRequestID(1).
			SetProjectID(1).
			SetChannelID(ch1.ID).
			SetModelID("claude-4.6").
			SetCreatedAt(now).
			Save(ctx)
		require.NoError(t, err)
	}

	preview, err := buildLoadBalancerPreview(ctx, &Resolver{
		client:         client,
		systemService:  systemService,
		channelService: channelService,
		modelService:   modelService,
	})
	require.NoError(t, err)
	require.NotNil(t, preview)
	require.Equal(t, "gpt-5.4", preview.ModelID)
	require.Equal(t, biz.LoadBalancerStrategyFailover, preview.ActiveStrategy)
	require.NotEmpty(t, preview.Strategies)
	failoverPreview := lo.FindOrElse(preview.Strategies, nil, func(item *loadBalancerPreviewStrategy) bool {
		return item.Strategy == biz.LoadBalancerStrategyFailover
	})
	require.NotNil(t, failoverPreview)
	require.Len(t, failoverPreview.Candidates, 3)
	require.Equal(t, "Primary", failoverPreview.Summary.PrimaryChannelName)
	require.Equal(t, "Retry A", failoverPreview.Summary.FirstRetryChannelName)
	require.Equal(t, "Retry B", failoverPreview.Summary.FallbackChannelName)
	require.Len(t, failoverPreview.Steps, 3)
	require.Equal(t, 1, failoverPreview.Steps[0].Attempt)
	require.Equal(t, "Primary", failoverPreview.Steps[0].ChannelName)
	require.Equal(t, 2, failoverPreview.Steps[1].Attempt)
	require.Equal(t, "Retry A", failoverPreview.Steps[1].ChannelName)
	require.Equal(t, 1000, failoverPreview.Steps[0].WaitMSAfterFailure)
	require.Equal(t, []string{"Primary", "Retry A", "Retry B"}, lo.Map(failoverPreview.Candidates, func(item *loadBalancerPreviewCandidate, _ int) string {
		return item.ChannelName
	}))
}

func TestBuildLoadBalancerPreviewIncludesStrategyComparisonsAndScores(t *testing.T) {
	ctx := authz.WithTestBypass(context.Background())
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	t.Cleanup(func() { client.Close() })
	ctx = ent.NewContext(ctx, client)

	systemService := biz.NewSystemService(biz.SystemServiceParams{
		CacheConfig: xcache.Config{Mode: xcache.ModeMemory},
		Ent:         client,
	})
	err := systemService.SetRetryPolicy(ctx, &biz.RetryPolicy{
		Enabled:                 true,
		MaxChannelRetries:       2,
		MaxSingleChannelRetries: 1,
		RetryDelayMs:            800,
		LoadBalancerStrategy:    biz.LoadBalancerStrategyAdaptive,
	})
	require.NoError(t, err)

	channelService := biz.NewChannelService(biz.ChannelServiceParams{
		CacheConfig:   xcache.Config{Mode: xcache.ModeMemory},
		Executor:      executors.NewPoolScheduleExecutor(),
		Ent:           client,
		SystemService: systemService,
	})
	t.Cleanup(channelService.Stop)

	modelService := biz.NewModelService(biz.ModelServiceParams{Ent: client})
	requestService := biz.NewRequestService(client, systemService, nil, nil)

	ch1, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("HA Primary").
		SetBaseURL("https://example.com/ha-primary").
		SetCredentials(objects.ChannelCredentials{APIKey: "ha1"}).
		SetSupportedModels([]string{"gpt-5.4"}).
		SetDefaultTestModel("gpt-5.4").
		SetOrderingWeight(100).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	ch2, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Balanced Retry").
		SetBaseURL("https://example.com/balanced").
		SetCredentials(objects.ChannelCredentials{APIKey: "ha2"}).
		SetSupportedModels([]string{"gpt-5.4"}).
		SetDefaultTestModel("gpt-5.4").
		SetOrderingWeight(70).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	channelService.SetEnabledChannelsForTest([]*biz.Channel{
		{Channel: ch1},
		{Channel: ch2},
	})

	_, err = client.Model.Create().
		SetDeveloper("openai").
		SetModelID("gpt-5.4").
		SetType(model.TypeChat).
		SetName("GPT-5.4").
		SetIcon("openai").
		SetGroup("openai").
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{}).
		SetStatus(model.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(11).
		SetProjectID(1).
		SetChannelID(ch1.ID).
		SetModelID("gpt-5.4").
		SetCreatedAt(time.Now()).
		Save(ctx)
	require.NoError(t, err)

	preview, err := buildLoadBalancerPreview(ctx, &Resolver{
		client:         client,
		systemService:  systemService,
		channelService: channelService,
		modelService:   modelService,
		requestService: requestService,
	})
	require.NoError(t, err)
	require.NotNil(t, preview)
	require.NotEmpty(t, preview.Strategies)
	require.Equal(t, biz.LoadBalancerStrategyAdaptive, preview.ActiveStrategy)

	foundAdaptive := false
	for _, strategy := range preview.Strategies {
		if strategy.Strategy == biz.LoadBalancerStrategyAdaptive {
			foundAdaptive = true
		}
		require.NotEmpty(t, strategy.Candidates)
		require.NotEmpty(t, strategy.Steps)
		require.NotZero(t, strategy.Candidates[0].TotalScore)
		require.NotEmpty(t, strategy.Candidates[0].ScoreBreakdown)
	}
	require.True(t, foundAdaptive)
}

func TestBuildLoadBalancerPreviewIncludesCandidateDiagnostics(t *testing.T) {
	ctx := authz.WithTestBypass(context.Background())
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	t.Cleanup(func() { client.Close() })
	ctx = ent.NewContext(ctx, client)

	systemService := biz.NewSystemService(biz.SystemServiceParams{
		CacheConfig: xcache.Config{Mode: xcache.ModeMemory},
		Ent:         client,
	})
	err := systemService.SetRetryPolicy(ctx, &biz.RetryPolicy{
		Enabled:              true,
		MaxChannelRetries:    1,
		RetryDelayMs:         500,
		LoadBalancerStrategy: biz.LoadBalancerStrategyLowLatency,
	})
	require.NoError(t, err)

	channelService := biz.NewChannelService(biz.ChannelServiceParams{
		CacheConfig:   xcache.Config{Mode: xcache.ModeMemory},
		Executor:      executors.NewPoolScheduleExecutor(),
		Ent:           client,
		SystemService: systemService,
	})
	t.Cleanup(channelService.Stop)

	modelService := biz.NewModelService(biz.ModelServiceParams{Ent: client})

	ch1, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Fast Healthy").
		SetBaseURL("https://example.com/fast").
		SetCredentials(objects.ChannelCredentials{APIKey: "fast"}).
		SetSupportedModels([]string{"gpt-5.4"}).
		SetDefaultTestModel("gpt-5.4").
		SetOrderingWeight(90).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	ch2, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Slow Degraded").
		SetBaseURL("https://example.com/slow").
		SetCredentials(objects.ChannelCredentials{APIKey: "slow"}).
		SetSupportedModels([]string{"gpt-5.4"}).
		SetDefaultTestModel("gpt-5.4").
		SetOrderingWeight(90).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	channelService.SetEnabledChannelsForTest([]*biz.Channel{
		{Channel: ch1},
		{Channel: ch2},
	})

	_, err = client.Model.Create().
		SetDeveloper("openai").
		SetModelID("gpt-5.4").
		SetType(model.TypeChat).
		SetName("GPT-5.4").
		SetIcon("openai").
		SetGroup("openai").
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{}).
		SetStatus(model.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(99).
		SetProjectID(1).
		SetChannelID(ch1.ID).
		SetModelID("gpt-5.4").
		SetCreatedAt(time.Now()).
		Save(ctx)
	require.NoError(t, err)

	fastLatency := 120.0
	slowLatency := 2400.0
	channelService.UpdateChannelProbeHealth(ch1.ID, &biz.ChannelProbeHealth{
		ProbeHealthRecorded:  true,
		Alive:                true,
		ModelsAlive:          true,
		ProbeModelAlive:      true,
		ProbeModelLatencyMs:  lo.ToPtr(fastLatency),
		ActiveProbeLatencyMs: lo.ToPtr(fastLatency),
		Timestamp:            time.Now().Unix(),
	})
	channelService.UpdateChannelProbeHealth(ch2.ID, &biz.ChannelProbeHealth{
		ProbeHealthRecorded:  true,
		Alive:                true,
		ModelsAlive:          true,
		ProbeModelAlive:      true,
		ProbeModelLatencyMs:  lo.ToPtr(slowLatency),
		ActiveProbeLatencyMs: lo.ToPtr(slowLatency),
		Timestamp:            time.Now().Unix(),
	})
	channelService.RecordPerformance(ctx, &biz.PerformanceRecord{
		ChannelID:          ch2.ID,
		StartTime:          time.Now().Add(-2 * time.Second),
		EndTime:            time.Now(),
		Success:            false,
		RequestCompleted:   true,
		ResponseStatusCode: 500,
	})

	preview, err := buildLoadBalancerPreview(ctx, &Resolver{
		client:         client,
		systemService:  systemService,
		channelService: channelService,
		modelService:   modelService,
	})
	require.NoError(t, err)

	lowLatencyPreview := lo.FindOrElse(preview.Strategies, nil, func(item *loadBalancerPreviewStrategy) bool {
		return item.Strategy == biz.LoadBalancerStrategyLowLatency
	})
	require.NotNil(t, lowLatencyPreview)
	require.Len(t, lowLatencyPreview.Candidates, 2)
	require.Equal(t, "Fast Healthy", lowLatencyPreview.Candidates[0].ChannelName)
	require.NotEmpty(t, lowLatencyPreview.Candidates[0].HealthStatus)
	require.NotNil(t, lowLatencyPreview.Candidates[0].LatencyMS)
	require.Equal(t, fastLatency, *lowLatencyPreview.Candidates[0].LatencyMS)
	require.Equal(t, int64(1), lowLatencyPreview.Candidates[1].RecentFailures)
	require.NotContains(t, lowLatencyPreview.Candidates[0].Reason, "zero_requests")
}

func TestBuildLoadBalancerPreviewHighAvailabilityUsesModelHealthAndFiltersUnhealthyChannels(t *testing.T) {
	ctx := authz.WithTestBypass(context.Background())
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	t.Cleanup(func() { client.Close() })
	ctx = ent.NewContext(ctx, client)

	systemService := biz.NewSystemService(biz.SystemServiceParams{
		CacheConfig: xcache.Config{Mode: xcache.ModeMemory},
		Ent:         client,
	})
	err := systemService.SetRetryPolicy(ctx, &biz.RetryPolicy{
		Enabled:              true,
		MaxChannelRetries:    2,
		RetryDelayMs:         500,
		LoadBalancerStrategy: biz.LoadBalancerStrategyHighAvailability,
	})
	require.NoError(t, err)

	channelService := biz.NewChannelService(biz.ChannelServiceParams{
		CacheConfig:   xcache.Config{Mode: xcache.ModeMemory},
		Executor:      executors.NewPoolScheduleExecutor(),
		Ent:           client,
		SystemService: systemService,
	})
	t.Cleanup(channelService.Stop)

	modelService := biz.NewModelService(biz.ModelServiceParams{Ent: client})

	ch1, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Healthy High Weight").
		SetBaseURL("https://example.com/ha-1").
		SetCredentials(objects.ChannelCredentials{APIKey: "ha-1"}).
		SetSupportedModels([]string{"gpt-5.4"}).
		SetDefaultTestModel("gpt-5.4").
		SetOrderingWeight(100).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	ch2, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Unhealthy Highest Weight").
		SetBaseURL("https://example.com/ha-2").
		SetCredentials(objects.ChannelCredentials{APIKey: "ha-2"}).
		SetSupportedModels([]string{"gpt-5.4"}).
		SetDefaultTestModel("gpt-5.4").
		SetOrderingWeight(200).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	ch3, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Healthy Lower Weight").
		SetBaseURL("https://example.com/ha-3").
		SetCredentials(objects.ChannelCredentials{APIKey: "ha-3"}).
		SetSupportedModels([]string{"gpt-5.4"}).
		SetDefaultTestModel("gpt-5.4").
		SetOrderingWeight(80).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	channelService.SetEnabledChannelsForTest([]*biz.Channel{
		{Channel: ch1},
		{Channel: ch2},
		{Channel: ch3},
	})

	_, err = client.Model.Create().
		SetDeveloper("openai").
		SetModelID("gpt-5.4").
		SetType(model.TypeChat).
		SetName("GPT-5.4").
		SetIcon("openai").
		SetGroup("openai").
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{ProbeEnabled: true}).
		SetStatus(model.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(501).
		SetProjectID(1).
		SetChannelID(ch1.ID).
		SetModelID("gpt-5.4").
		SetCreatedAt(time.Now()).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now().Unix()
	channelService.UpdateChannelProbeHealth(ch1.ID, &biz.ChannelProbeHealth{ProbeHealthRecorded: true, Alive: true, ModelsAlive: true, ProbeModelAlive: true, ProbeModelLatencyMs: lo.ToPtr(100.0), Timestamp: now})
	channelService.UpdateChannelProbeHealth(ch2.ID, &biz.ChannelProbeHealth{ProbeHealthRecorded: true, Alive: true, ModelsAlive: true, ProbeModelAlive: true, ProbeModelLatencyMs: lo.ToPtr(90.0), Timestamp: now})
	channelService.UpdateChannelProbeHealth(ch3.ID, &biz.ChannelProbeHealth{ProbeHealthRecorded: true, Alive: true, ModelsAlive: true, ProbeModelAlive: true, ProbeModelLatencyMs: lo.ToPtr(130.0), Timestamp: now})

	_, err = client.ModelHealthSnapshot.Create().
		SetDisplayModel("gpt-5.4").
		SetChannelID(ch1.ID).
		SetActualModelID("gpt-5.4").
		SetIsHealthy(true).
		SetManualOverride(false).
		SetProbedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ModelHealthSnapshot.Create().
		SetDisplayModel("gpt-5.4").
		SetChannelID(ch2.ID).
		SetActualModelID("gpt-5.4").
		SetIsHealthy(false).
		SetManualOverride(false).
		SetProbedAt(now).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ModelHealthSnapshot.Create().
		SetDisplayModel("gpt-5.4").
		SetChannelID(ch3.ID).
		SetActualModelID("gpt-5.4").
		SetIsHealthy(true).
		SetManualOverride(false).
		SetProbedAt(now).
		Save(ctx)
	require.NoError(t, err)

	preview, err := buildLoadBalancerPreview(ctx, &Resolver{
		client:         client,
		systemService:  systemService,
		channelService: channelService,
		modelService:   modelService,
	})
	require.NoError(t, err)
	require.NotNil(t, preview)

	haPreview := lo.FindOrElse(preview.Strategies, nil, func(item *loadBalancerPreviewStrategy) bool {
		return item.Strategy == biz.LoadBalancerStrategyHighAvailability
	})
	require.NotNil(t, haPreview)
	require.Equal(t, []string{"Healthy High Weight", "Healthy Lower Weight"}, lo.Map(haPreview.Candidates, func(item *loadBalancerPreviewCandidate, _ int) string {
		return item.ChannelName
	}))
	require.Equal(t, "Healthy High Weight", haPreview.Summary.PrimaryChannelName)
	require.Equal(t, "Healthy Lower Weight", haPreview.Summary.FirstRetryChannelName)
	require.Equal(t, "Healthy Lower Weight", haPreview.Summary.FallbackChannelName)
}
