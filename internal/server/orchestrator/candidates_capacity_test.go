package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
)

type staticCandidatesSelector struct {
	candidates []*ChannelModelsCandidate
}

func (s *staticCandidatesSelector) Select(_ctx context.Context, _req *llm.Request) ([]*ChannelModelsCandidate, error) {
	return s.candidates, nil
}

func TestChannelCapacitySelector_RPMExceededSwitchesChannel(t *testing.T) {
	ctx, client := setupTest(t)

	ch1, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("limited-rpm").
		SetBaseURL("https://api.openai.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "k1"}).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetStatus(channel.StatusEnabled).
		SetSettings(&objects.ChannelSettings{RPM: 1}).
		Save(ctx)
	require.NoError(t, err)

	ch2, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("normal").
		SetBaseURL("https://api.openai.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "k2"}).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	channelService := biz.NewChannelServiceForTest(client)
	channelService.IncrementChannelSelection(ch1.ID)

	base := &staticCandidatesSelector{
		candidates: []*ChannelModelsCandidate{
			{Channel: &biz.Channel{Channel: ch1}, Models: []biz.ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4"}}},
			{Channel: &biz.Channel{Channel: ch2}, Models: []biz.ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4"}}},
		},
	}

	selector := WithChannelCapacitySelector(base, channelService, NewDefaultConnectionTracker(10))
	result, err := selector.Select(ctx, &llm.Request{Model: "gpt-4"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, ch2.ID, result[0].Channel.ID)
}

func TestChannelCapacitySelector_ConcurrencyExceededSwitchesChannel(t *testing.T) {
	ctx, client := setupTest(t)

	ch1, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("limited-concurrency").
		SetBaseURL("https://api.openai.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "k1"}).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetStatus(channel.StatusEnabled).
		SetSettings(&objects.ChannelSettings{Concurrency: 1}).
		Save(ctx)
	require.NoError(t, err)

	ch2, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("normal").
		SetBaseURL("https://api.openai.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "k2"}).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	tracker := NewDefaultConnectionTracker(10)
	tracker.IncrementConnection(ch1.ID)

	base := &staticCandidatesSelector{
		candidates: []*ChannelModelsCandidate{
			{Channel: &biz.Channel{Channel: ch1}, Models: []biz.ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4"}}},
			{Channel: &biz.Channel{Channel: ch2}, Models: []biz.ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4"}}},
		},
	}

	selector := WithChannelCapacitySelector(base, nil, tracker)
	result, err := selector.Select(ctx, &llm.Request{Model: "gpt-4"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, ch2.ID, result[0].Channel.ID)
}

func TestChannelCapacitySelector_AllExceededFallsBackToOriginal(t *testing.T) {
	ctx, client := setupTest(t)

	ch1, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("limited-rpm-1").
		SetBaseURL("https://api.openai.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "k1"}).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetStatus(channel.StatusEnabled).
		SetSettings(&objects.ChannelSettings{RPM: 1}).
		Save(ctx)
	require.NoError(t, err)

	ch2, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("limited-rpm-2").
		SetBaseURL("https://api.openai.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "k2"}).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetStatus(channel.StatusEnabled).
		SetSettings(&objects.ChannelSettings{RPM: 1}).
		Save(ctx)
	require.NoError(t, err)

	channelService := biz.NewChannelServiceForTest(client)
	channelService.IncrementChannelSelection(ch1.ID)
	channelService.IncrementChannelSelection(ch2.ID)

	base := &staticCandidatesSelector{
		candidates: []*ChannelModelsCandidate{
			{Channel: &biz.Channel{Channel: ch1}, Models: []biz.ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4"}}},
			{Channel: &biz.Channel{Channel: ch2}, Models: []biz.ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4"}}},
		},
	}

	selector := WithChannelCapacitySelector(base, channelService, nil)
	result, err := selector.Select(ctx, &llm.Request{Model: "gpt-4"})
	require.NoError(t, err)
	require.Len(t, result, 2)
}
