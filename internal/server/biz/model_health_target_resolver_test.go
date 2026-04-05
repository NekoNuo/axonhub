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

func TestModelHealthTargetResolver(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	modelSvc := &ModelService{AbstractService: &AbstractService{db: client}}

	channel1, err := client.Channel.Create().
		SetType("openai").
		SetName("OpenAI Channel 1").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"k1"}}).
		SetSupportedModels([]string{"gpt-4o-2024-11-20", "gpt-4.1"}).
		SetDefaultTestModel("gpt-4o-2024-11-20").
		Save(ctx)
	require.NoError(t, err)

	channel2, err := client.Channel.Create().
		SetType("openai").
		SetName("OpenAI Channel 2").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"k2"}}).
		SetSupportedModels([]string{"gpt-4o-2024-11-20"}).
		SetDefaultTestModel("gpt-4o-2024-11-20").
		Save(ctx)
	require.NoError(t, err)

	channel3, err := client.Channel.Create().
		SetType("anthropic").
		SetName("Anthropic Channel").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"k3"}}).
		SetSupportedModels([]string{"claude-3-7-sonnet"}).
		SetDefaultTestModel("claude-3-7-sonnet").
		Save(ctx)
	require.NoError(t, err)

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
						ChannelID: channel1.ID,
						ModelID:   "gpt-4o-2024-11-20",
					},
				},
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.Model.Create().
		SetDeveloper("anthropic").
		SetModelID("claude-sonnet").
		SetType(model.TypeChat).
		SetName("Claude Sonnet").
		SetIcon("anthropic").
		SetGroup("anthropic").
		SetStatus(model.StatusEnabled).
		SetModelCard(&objects.ModelCard{}).
		SetSettings(&objects.ModelSettings{
			ProbeEnabled: true,
			Associations: []*objects.ModelAssociation{
				{
					Type: "channel_model",
					ChannelModel: &objects.ChannelModelAssociation{
						ChannelID: channel3.ID,
						ModelID:   "claude-3-7-sonnet",
					},
				},
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	resolver := NewModelHealthTargetResolver(client, modelSvc)

	recent := []ModelHealthRecentUsage{
		{
			DisplayModel: "gpt-4o",
			ActualModels: []ModelHealthRecentActualModel{
				{ActualModelID: "gpt-4o-2024-11-20", ChannelIDs: []int{channel1.ID, channel2.ID}},
				{ActualModelID: "gpt-4.1", ChannelIDs: []int{channel1.ID}},
			},
		},
	}

	targets, err := resolver.Resolve(ctx, recent)
	require.NoError(t, err)

	require.Len(t, targets, 3)

	require.Equal(t, "gpt-4o", targets[0].DisplayModel)
	require.Equal(t, "gpt-4.1", targets[0].ActualModelID)
	require.Equal(t, channel1.ID, targets[0].ChannelID)
	require.ElementsMatch(t, []string{ModelHealthTargetSourceRecent}, targets[0].Sources)

	require.Equal(t, "gpt-4o", targets[1].DisplayModel)
	require.Equal(t, "gpt-4o-2024-11-20", targets[1].ActualModelID)
	require.Equal(t, channel1.ID, targets[1].ChannelID)
	require.ElementsMatch(t, []string{ModelHealthTargetSourceRecent}, targets[1].Sources)

	require.Equal(t, "gpt-4o", targets[2].DisplayModel)
	require.Equal(t, "gpt-4o-2024-11-20", targets[2].ActualModelID)
	require.Equal(t, channel2.ID, targets[2].ChannelID)
	require.ElementsMatch(t, []string{ModelHealthTargetSourceRecent}, targets[2].Sources)
}
