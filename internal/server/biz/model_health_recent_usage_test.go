package biz

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/usagelog"
	"github.com/looplj/axonhub/internal/objects"
)

func TestModelHealthRecentUsage(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)

	_, err := client.Project.Create().
		SetName("default").
		SetDescription("default project").
		SetStatus("active").
		Save(ctx)
	require.NoError(t, err)

	channelIDs := map[string]int{}
	for _, item := range []struct {
		key  string
		name string
	}{
		{key: "11", name: "channel-11"},
		{key: "12", name: "channel-12"},
		{key: "21", name: "channel-21"},
		{key: "99", name: "channel-99"},
	} {
		ch, createErr := client.Channel.Create().
			SetType("openai").
			SetName(item.name).
			SetStatus("enabled").
			SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key"}}).
			SetSupportedModels([]string{"placeholder"}).
			SetDefaultTestModel("placeholder").
			Save(ctx)
		require.NoError(t, createErr)
		channelIDs[item.key] = ch.ID
	}

	createRecentUsageFixture(t, ctx, client, now.Add(-10*time.Minute), channelIDs["11"], "gpt-4o", "gpt-4o-2024-11-20")
	createRecentUsageFixture(t, ctx, client, now.Add(-8*time.Minute), channelIDs["12"], "gpt-4o", "gpt-4o-2024-11-20")
	createRecentUsageFixture(t, ctx, client, now.Add(-5*time.Minute), channelIDs["11"], "gpt-4o", "gpt-4.1")
	createRecentUsageFixture(t, ctx, client, now.Add(-4*time.Minute), channelIDs["21"], "claude-sonnet", "claude-3-7-sonnet")
	createRecentUsageFixture(t, ctx, client, now.Add(-2*time.Hour), channelIDs["99"], "expired-model", "expired-actual")

	aggregator := NewModelHealthRecentUsageAggregator(client)

	result, err := aggregator.Aggregate(ctx, now)
	require.NoError(t, err)

	require.Len(t, result, 2)

	require.Equal(t, "claude-sonnet", result[0].DisplayModel)
	require.Equal(t, "gpt-4o", result[1].DisplayModel)

	require.Equal(t, []int{channelIDs["21"]}, result[0].ActualModels[0].ChannelIDs)
	require.Equal(t, "claude-3-7-sonnet", result[0].ActualModels[0].ActualModelID)

	require.Len(t, result[1].ActualModels, 2)
	require.Equal(t, "gpt-4.1", result[1].ActualModels[0].ActualModelID)
	require.Equal(t, []int{channelIDs["11"]}, result[1].ActualModels[0].ChannelIDs)
	require.Equal(t, "gpt-4o-2024-11-20", result[1].ActualModels[1].ActualModelID)
	require.Equal(t, []int{channelIDs["11"], channelIDs["12"]}, result[1].ActualModels[1].ChannelIDs)
}

func createRecentUsageFixture(t *testing.T, ctx context.Context, client *ent.Client, createdAt time.Time, channelID int, displayModel string, actualModel string) {
	t.Helper()

	req, err := client.Request.Create().
		SetProjectID(1).
		SetSource("api").
		SetModelID(displayModel).
		SetFormat("openai/chat_completions").
		SetRequestBody([]byte(`{}`)).
		SetStatus("completed").
		SetStream(false).
		SetClientIP("127.0.0.1").
		SetCreatedAt(createdAt).
		SetUpdatedAt(createdAt).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetRequestID(req.ID).
		SetProjectID(1).
		SetChannelID(channelID).
		SetModelID(actualModel).
		SetPromptTokens(1).
		SetCompletionTokens(1).
		SetTotalTokens(2).
		SetSource(usagelog.SourceAPI).
		SetFormat("openai/chat_completions").
		SetCreatedAt(createdAt).
		SetUpdatedAt(createdAt).
		Save(ctx)
	require.NoError(t, err)
}
