package biz

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/pkg/xcache/live"
)

func TestChannelServiceStopStopsTokenProviders(t *testing.T) {
	stopped := false
	svc := &ChannelService{
		enabledChannelsCache: live.NewCache(live.Options[[]*Channel]{
			Name:            "channel_service_stop_test",
			InitialValue:    []*Channel{{stopTokenProvider: func() { stopped = true }}},
			RefreshInterval: time.Hour,
			RefreshFunc: func(_ context.Context, current []*Channel, lastUpdate time.Time) ([]*Channel, time.Time, bool, error) {
				return current, lastUpdate, false, nil
			},
		}),
	}

	svc.Stop()

	require.True(t, stopped)
}
