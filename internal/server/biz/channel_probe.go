package biz

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"entgo.io/ent/dialect"
	"github.com/samber/lo"
	"github.com/zhenzou/executors"
	"go.uber.org/fx"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/channelprobe"
	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/pkg/xtime"
	"github.com/looplj/axonhub/internal/scopes"
	"github.com/looplj/axonhub/internal/server/gql/qb"
	"github.com/looplj/axonhub/llm/httpclient"
)

const (
	activeProbeTimeout        = 8 * time.Second
	activeProbeMaxConcurrency = 5
)

// ChannelProbePoint represents a single probe data point for a channel.
type ChannelProbePoint struct {
	Timestamp             int64    `json:"timestamp"`
	TotalRequestCount     int      `json:"total_request_count"`
	SuccessRequestCount   int      `json:"success_request_count"`
	AvgTokensPerSecond    *float64 `json:"avg_tokens_per_second,omitempty"`
	AvgTimeToFirstTokenMs *float64 `json:"avg_time_to_first_token_ms,omitempty"`
	ActiveProbeLatencyMs  *float64 `json:"active_probe_latency_ms,omitempty"`
	ProbeModelLatencyMs   *float64 `json:"probe_model_latency_ms,omitempty"`
}

// ChannelProbeData represents probe data for a single channel.
type ChannelProbeData struct {
	ChannelID int                  `json:"channel_id"`
	Points    []*ChannelProbePoint `json:"points"`
}

type ChannelHealthSnapshot struct {
	ChannelID              int      `json:"channel_id"`
	ProbeHealthRecorded    bool     `json:"probe_health_recorded"`
	Alive                  bool     `json:"alive"`
	ModelsAlive            bool     `json:"models_alive"`
	ProbeModelAlive        bool     `json:"probe_model_alive"`
	ActiveProbeLatencyMs   *float64 `json:"active_probe_latency_ms,omitempty"`
	ProbeModelLatencyMs    *float64 `json:"probe_model_latency_ms,omitempty"`
	ProbeTimestamp         int64    `json:"probe_timestamp"`
	ObservedHealthRecorded bool     `json:"observed_health_recorded"`
	ObservedAlive          bool     `json:"observed_alive"`
	ObservedLatencyMs      *float64 `json:"observed_latency_ms,omitempty"`
	ObservedTimestamp      int64    `json:"observed_timestamp"`
}

// ChannelProbeServiceParams contains dependencies for ChannelProbeService.
type ChannelProbeServiceParams struct {
	fx.In

	Ent            *ent.Client
	SystemService  *SystemService
	ChannelService *ChannelService
	HttpClient     *httpclient.HttpClient
}

// ChannelProbeService handles channel probe operations.
type ChannelProbeService struct {
	*AbstractService

	SystemService          *SystemService
	ChannelService         *ChannelService
	Executor               executors.ScheduledExecutor
	mu                     sync.Mutex
	lastExecutionTime      time.Time
	modelFetcher           *ModelFetcher
	idleChannelProber      func(ctx context.Context, ch *ent.Channel) (time.Duration, bool, error)
	idleChannelModelProber func(ctx context.Context, ch *ent.Channel, modelID string) (time.Duration, bool, error)
}

// NewChannelProbeService creates a new ChannelProbeService.
func NewChannelProbeService(params ChannelProbeServiceParams) *ChannelProbeService {
	svc := &ChannelProbeService{
		AbstractService: &AbstractService{
			db: params.Ent,
		},
		SystemService:     params.SystemService,
		ChannelService:    params.ChannelService,
		Executor:          executors.NewPoolScheduleExecutor(executors.WithMaxConcurrent(1)),
		lastExecutionTime: time.Time{},
	}
	svc.modelFetcher = NewModelFetcher(params.HttpClient, params.ChannelService)
	svc.idleChannelProber = svc.probeIdleChannelByFetchModels

	return svc
}

func (svc *ChannelProbeService) SetIdleChannelModelProber(
	prober func(ctx context.Context, ch *ent.Channel, modelID string) (time.Duration, bool, error),
) {
	svc.idleChannelModelProber = prober
}

// Start starts the channel probe service with scheduled task.
func (svc *ChannelProbeService) Start(ctx context.Context) error {
	_, err := svc.Executor.ScheduleFuncAtCronRate(
		svc.runProbePeriodically,
		executors.CRONRule{Expr: "* * * * *"},
	)

	return err
}

// Stop stops the channel probe service.
func (svc *ChannelProbeService) Stop(ctx context.Context) error {
	return svc.Executor.Shutdown(ctx)
}

// shouldRunProbe determines if a probe should be executed based on frequency, current time, and last execution time.
// It returns true if the current aligned time is different from the last execution time.
// This is a pure function that does not depend on any external state.
func shouldRunProbe(frequency ProbeFrequency, now time.Time, lastExecution time.Time) bool {
	intervalMinutes := getIntervalMinutesFromFrequency(frequency)
	alignedTime := now.Truncate(time.Duration(intervalMinutes) * time.Minute)

	return !lastExecution.Equal(alignedTime)
}

// getIntervalMinutesFromFrequency returns the interval in minutes based on the probe frequency.
func getIntervalMinutesFromFrequency(frequency ProbeFrequency) int {
	switch frequency {
	case ProbeFrequency1Min:
		return 1
	case ProbeFrequency5Min:
		return 5
	case ProbeFrequency30Min:
		return 30
	case ProbeFrequency1Hour:
		return 60
	default:
		return 1
	}
}

type channelProbeStats struct {
	total                     int
	success                   int
	alive                     bool
	latencyMs                 *float64
	activeProbeLatencyMs      *float64
	activeProbeModelLatencyMs *float64
	modelsAlive               bool
	probeModelAlive           bool
	avgTokensPerSecond        *float64
	avgTimeToFirstTokenMs     *float64
}

// computeAllChannelProbeStats computes probe stats for all channels in a single batch query.
// Uses CTE with ROW_NUMBER to get only successful execution per request, includes all token types,
// and applies different TPS formulas for streaming vs non-streaming.
func (svc *ChannelProbeService) computeAllChannelProbeStats(
	ctx context.Context,
	channelIDs []int,
	startTime time.Time,
	endTime time.Time,
) (map[int]*channelProbeStats, error) {
	if len(channelIDs) == 0 {
		return nil, nil
	}

	// Use raw SQL query with CTE pattern (same as Task 1 FastestChannels)
	type probeResult struct {
		ChannelID              int   `json:"channel_id"`
		TotalCount             int   `json:"total_count"`
		SuccessCount           int   `json:"success_count"`
		TotalTokens            int64 `json:"total_tokens"`
		EffectiveLatencyMs     int64 `json:"effective_latency_ms"`
		TotalFirstTokenLatency int64 `json:"total_first_token_latency"`
		RequestCount           int   `json:"request_count"`
		StreamingRequestCount  int   `json:"streaming_request_count"`
	}

	dbDriver := svc.db.Driver()
	sqlDB, ok := dbDriver.(*entsql.Driver)
	if !ok {
		return nil, fmt.Errorf("failed to get underlying SQL driver")
	}

	// Detect dialect to use appropriate placeholder syntax
	// PostgreSQL uses $1, $2, etc. while SQLite uses ? placeholders
	dialectName := sqlDB.Dialect()
	useDollarPlaceholders := dialectName == dialect.Postgres

	// Build args slice for parameterized query
	args := make([]interface{}, 0, len(channelIDs)+2)
	args = append(args, startTime.UTC(), endTime.UTC())

	// Build channel ID filter with dialect-aware parameterized placeholders
	// Note: Placeholders start at $3 because $1 and $2 are reserved for startTime and endTime timestamps.
	// The args slice is constructed with timestamps first (lines 155-156), then channel IDs appended,
	// so placeholder numbering must match this ordering to bind values correctly.
	channelIDFilter := ""
	if len(channelIDs) > 0 {
		placeholders := make([]string, len(channelIDs))
		for i, id := range channelIDs {
			if useDollarPlaceholders {
				placeholders[i] = fmt.Sprintf("$%d", i+3) // $3, $4, etc. for PostgreSQL (offset by 2 for timestamps)
			} else {
				placeholders[i] = "?" // ? for SQLite
			}
			args = append(args, id)
		}
		channelIDFilter = fmt.Sprintf("AND se.channel_id IN (%s)", strings.Join(placeholders, ","))
	}

	queryMode := qb.ThroughputModeRowNumber
	if !useDollarPlaceholders {
		queryMode = qb.ThroughputModeMaxID
	}

	query := qb.BuildProbeStatsQuery(useDollarPlaceholders, channelIDFilter, queryMode)

	rows, err := sqlDB.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query channel probe stats: %w", err)
	}

	defer func() { _ = rows.Close() }()

	result := make(map[int]*channelProbeStats)
	for rows.Next() {
		var r probeResult
		if err := rows.Scan(
			&r.ChannelID,
			&r.TotalCount,
			&r.SuccessCount,
			&r.TotalTokens,
			&r.EffectiveLatencyMs,
			&r.TotalFirstTokenLatency,
			&r.RequestCount,
			&r.StreamingRequestCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan probe result: %w", err)
		}

		stats := &channelProbeStats{
			total:           r.TotalCount,
			success:         r.SuccessCount,
			alive:           r.SuccessCount > 0,
			modelsAlive:     r.SuccessCount > 0,
			probeModelAlive: r.SuccessCount > 0,
		}

		// Use average effective latency as health latency baseline.
		if r.EffectiveLatencyMs > 0 && r.RequestCount > 0 {
			avgLatencyMs := float64(r.EffectiveLatencyMs) / float64(r.RequestCount)
			stats.latencyMs = &avgLatencyMs
		}

		// Calculate avg tokens per second using effective latency
		// For streaming: tokens / ((latency - first_token_latency) / 1000)
		// For non-streaming: tokens / (latency / 1000)
		if r.TotalTokens > 0 && r.EffectiveLatencyMs > 0 {
			tps := float64(r.TotalTokens) / (float64(r.EffectiveLatencyMs) / 1000.0)
			stats.avgTokensPerSecond = &tps
		}

		// Calculate avg time to first token (only for streaming requests)
		if r.TotalFirstTokenLatency > 0 && r.StreamingRequestCount > 0 {
			avgTTFT := float64(r.TotalFirstTokenLatency) / float64(r.StreamingRequestCount)
			stats.avgTimeToFirstTokenMs = &avgTTFT
		}

		result[r.ChannelID] = stats
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating probe results: %w", err)
	}

	return result, nil
}

func (svc *ChannelProbeService) probeIdleChannelByFetchModels(ctx context.Context, ch *ent.Channel) (time.Duration, bool, error) {
	start := time.Now()

	if svc.modelFetcher == nil {
		return time.Since(start), false, fmt.Errorf("model fetcher is not initialized")
	}

	result, err := svc.modelFetcher.FetchModels(ctx, FetchModelsInput{
		ChannelType: ch.Type.String(),
		BaseURL:     ch.BaseURL,
		ChannelID:   lo.ToPtr(ch.ID),
	})
	if err != nil {
		return time.Since(start), false, err
	}

	if result == nil {
		return time.Since(start), false, fmt.Errorf("empty fetch models result")
	}

	if result.Error != nil {
		return time.Since(start), false, fmt.Errorf("fetch models returned error: %s", *result.Error)
	}

	return time.Since(start), true, nil
}

func preferredProbeLatency(stats *channelProbeStats) *float64 {
	if stats == nil {
		return nil
	}

	if stats.activeProbeModelLatencyMs != nil {
		return stats.activeProbeModelLatencyMs
	}

	return stats.activeProbeLatencyMs
}

func (svc *ChannelProbeService) fillIdleChannelProbeStats(
	ctx context.Context,
	channels []*ent.Channel,
	allStats map[int]*channelProbeStats,
) (int, int) {
	return svc.fillIdleChannelProbeStatsWithSettings(ctx, channels, allStats, ChannelProbeSetting{
		ActiveProbeIdleChannels: true,
	})
}

func (svc *ChannelProbeService) fillIdleChannelProbeStatsWithSettings(
	ctx context.Context,
	channels []*ent.Channel,
	allStats map[int]*channelProbeStats,
	setting ChannelProbeSetting,
) (int, int) {
	if svc.idleChannelProber == nil || len(channels) == 0 {
		return 0, 0
	}

	if allStats == nil {
		allStats = make(map[int]*channelProbeStats)
	}

	sem := make(chan struct{}, activeProbeMaxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	probedCount := 0
	successCount := 0

	for _, ch := range channels {
		ch := ch
		wg.Add(1)

		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			probeCtx, cancel := context.WithTimeout(ctx, activeProbeTimeout)
			defer cancel()

			latency, success, err := svc.idleChannelProber(probeCtx, ch)
			if err != nil {
				log.Warn(ctx, "Active probe for channel failed",
					log.Int("channel_id", ch.ID),
					log.String("channel_type", ch.Type.String()),
					log.Cause(err),
				)
			}

			stats := &channelProbeStats{
				total:       1,
				success:     0,
				alive:       false,
				modelsAlive: success,
			}

			latencyMs := float64(latency.Milliseconds())
			if latencyMs > 0 {
				stats.activeProbeLatencyMs = lo.ToPtr(latencyMs)
			}

			probeModelAlive := success
			probeModelLatencyMs := stats.activeProbeLatencyMs
			if setting.ProbeModelIdleChannels {
				if ch.DefaultTestModel != "" && svc.idleChannelModelProber != nil {
					modelLatency, modelSuccess, modelErr := svc.idleChannelModelProber(probeCtx, ch, ch.DefaultTestModel)
					if modelErr != nil {
						log.Warn(ctx, "Active model probe for channel failed",
							log.Int("channel_id", ch.ID),
							log.String("channel_type", ch.Type.String()),
							log.String("model_id", ch.DefaultTestModel),
							log.Cause(modelErr),
						)
					}

					probeModelAlive = modelSuccess
					modelLatencyMs := float64(modelLatency.Milliseconds())
					if modelLatencyMs > 0 {
						probeModelLatencyMs = lo.ToPtr(modelLatencyMs)
					} else {
						probeModelLatencyMs = nil
					}
				}
			}

			stats.probeModelAlive = probeModelAlive
			stats.alive = success && probeModelAlive
			if stats.alive {
				stats.success = 1
			}
			stats.activeProbeModelLatencyMs = probeModelLatencyMs
			stats.latencyMs = preferredProbeLatency(stats)

			mu.Lock()
			allStats[ch.ID] = stats
			probedCount++
			if stats.alive {
				successCount++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	return probedCount, successCount
}

// runProbe executes the probe task.
func (svc *ChannelProbeService) runProbe(ctx context.Context) {
	svc.runProbeWithMode(ctx, false)
}

func (svc *ChannelProbeService) runProbeWithMode(ctx context.Context, force bool) {
	// Check if probe is enabled
	setting := svc.SystemService.ChannelSettingOrDefault(ctx)
	if !setting.Probe.Enabled {
		log.Debug(ctx, "Channel probe is disabled, skipping")
		return
	}

	intervalMinutes := setting.Probe.GetIntervalMinutes()
	now := xtime.UTCNow()
	// Align current time to interval boundary
	alignedTime := now.Truncate(time.Duration(intervalMinutes) * time.Minute)
	timestamp := alignedTime.Unix()

	// Check if we should execute based on last execution time
	svc.mu.Lock()

	lastExecution := svc.lastExecutionTime
	if !force && !lastExecution.IsZero() && !shouldRunProbe(setting.Probe.Frequency, now, lastExecution) {
		// Already executed for this interval
		svc.mu.Unlock()
		log.Debug(ctx, "Skipping probe, already executed for this interval",
			log.Int64("timestamp", timestamp),
		)

		return
	}
	// Update last execution time
	svc.lastExecutionTime = alignedTime
	svc.mu.Unlock()

	ctx = ent.NewContext(ctx, svc.db)

	log.Debug(ctx, "Starting channel probe",
		log.Int("interval_minutes", intervalMinutes),
		log.Int64("timestamp", timestamp),
	)

	// Get all enabled channels
	channels, err := svc.db.Channel.Query().
		Where(channel.StatusEQ(channel.StatusEnabled)).
		Select(channel.FieldID, channel.FieldType, channel.FieldBaseURL, channel.FieldDefaultTestModel).
		All(ctx)
	if err != nil {
		log.Error(ctx, "Failed to query enabled channels", log.Cause(err))
		return
	}

	if len(channels) == 0 {
		log.Debug(ctx, "No enabled channels to probe")
		return
	}

	// Calculate time range based on frequency
	startTime := alignedTime.Add(-time.Duration(intervalMinutes) * time.Minute)

	// Extract channel IDs for batch query
	channelIDs := make([]int, len(channels))
	for i, ch := range channels {
		channelIDs[i] = ch.ID
	}

	// Batch compute all channel stats in 3 queries instead of N*4 queries
	allStats, err := svc.computeAllChannelProbeStats(ctx, channelIDs, startTime, alignedTime)
	if err != nil {
		log.Error(ctx, "Failed to compute channel probe stats", log.Cause(err))
		return
	}

	if setting.Probe.ActiveProbeIdleChannels {
		probedIdle, successIdle := svc.fillIdleChannelProbeStatsWithSettings(ctx, channels, allStats, setting.Probe)
		if probedIdle > 0 {
			log.Debug(ctx, "Completed active probe for channels",
				log.Int("probed_channels", probedIdle),
				log.Int("successful_probes", successIdle),
			)
		}
	}

	// Collect probe data for each channel
	var probes []*ent.ChannelProbeCreate

	for _, ch := range channels {
		stats, ok := allStats[ch.ID]
		if !ok || stats.total == 0 {
			continue
		}

		if svc.ChannelService != nil {
			svc.ChannelService.UpdateChannelProbeHealth(ch.ID, &ChannelProbeHealth{
				Alive:                stats.alive,
				ModelsAlive:          stats.modelsAlive,
				ProbeModelAlive:      stats.probeModelAlive,
				ActiveProbeLatencyMs: stats.activeProbeLatencyMs,
				ProbeModelLatencyMs:  stats.activeProbeModelLatencyMs,
				Timestamp:            timestamp,
			})
		}

		probes = append(probes, svc.db.ChannelProbe.Create().
			SetChannelID(ch.ID).
			SetTotalRequestCount(stats.total).
			SetSuccessRequestCount(stats.success).
			SetNillableAvgTokensPerSecond(stats.avgTokensPerSecond).
			SetNillableAvgTimeToFirstTokenMs(stats.avgTimeToFirstTokenMs).
			SetNillableActiveProbeLatencyMs(stats.activeProbeLatencyMs).
			SetNillableProbeModelLatencyMs(stats.activeProbeModelLatencyMs).
			SetTimestamp(timestamp),
		)
	}

	if len(probes) == 0 {
		log.Debug(ctx, "No probe data to store (all channels have 0 requests)")
		return
	}

	if force {
		if _, err := svc.db.ChannelProbe.Delete().
			Where(
				channelprobe.ChannelIDIn(channelIDs...),
				channelprobe.TimestampEQ(timestamp),
			).
			Exec(ctx); err != nil {
			log.Error(ctx, "Failed to replace existing channel probes for manual run", log.Cause(err))
			return
		}
	}

	// Bulk create probes
	if err := svc.db.ChannelProbe.CreateBulk(probes...).Exec(ctx); err != nil {
		log.Error(ctx, "Failed to create channel probes", log.Cause(err))
		return
	}

	log.Debug(ctx, "Channel probe completed",
		log.Int("channels_probed", len(probes)),
		log.Int64("timestamp", timestamp),
	)
}

// generateTimestamps generates a slice of Unix timestamps from startTime to endTime
// with the given interval in minutes.
func generateTimestamps(setting ChannelProbeSetting, currentTime time.Time) []int64 {
	intervalMinutes := setting.GetIntervalMinutes()
	rangeMinutes := setting.GetQueryRangeMinutes()
	endTime := currentTime.Truncate(time.Duration(intervalMinutes) * time.Minute)
	startTime := endTime.Add(-time.Duration(rangeMinutes) * time.Minute)

	var timestamps []int64
	for t := startTime.Unix(); t <= endTime.Unix(); t += int64(intervalMinutes * 60) {
		timestamps = append(timestamps, t)
	}

	return timestamps
}

// QueryChannelProbes queries probe data for multiple channels with time range alignment.
func (svc *ChannelProbeService) QueryChannelProbes(ctx context.Context, channelIDs []int) ([]*ChannelProbeData, error) {
	setting, err := authz.RunWithScopeDecision(ctx, scopes.ScopeReadChannels, func(ctx context.Context) (*SystemChannelSettings, error) {
		return svc.SystemService.ChannelSettingOrDefault(ctx), nil
	})
	if err != nil {
		return nil, err
	}
	rangeMinutes := setting.Probe.GetQueryRangeMinutes()
	intervalMinutes := setting.Probe.GetIntervalMinutes()
	now := xtime.UTCNow()
	// Align end time to interval boundary
	endTime := now.Truncate(time.Duration(intervalMinutes) * time.Minute)
	startTime := endTime.Add(-time.Duration(rangeMinutes) * time.Minute)

	// Query all probes for the given channels in the time range
	probes, err := svc.db.ChannelProbe.Query().
		Where(
			channelprobe.ChannelIDIn(channelIDs...),
			channelprobe.TimestampGTE(startTime.Unix()),
			channelprobe.TimestampLTE(endTime.Unix()),
		).
		Order(ent.Asc(channelprobe.FieldTimestamp)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	// Build a map of channel_id -> timestamp -> probe
	probeMap := make(map[int]map[int64]*ent.ChannelProbe)
	for _, p := range probes {
		if probeMap[p.ChannelID] == nil {
			probeMap[p.ChannelID] = make(map[int64]*ent.ChannelProbe)
		}

		probeMap[p.ChannelID][p.Timestamp] = p
	}

	// Generate all expected timestamps
	timestamps := generateTimestamps(setting.Probe, now)

	// Build result with aligned data (fill missing points with 0)
	result := make([]*ChannelProbeData, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		points := make([]*ChannelProbePoint, 0, len(timestamps))
		channelProbes := probeMap[channelID]

		for _, ts := range timestamps {
			if p, ok := channelProbes[ts]; ok {
				points = append(points, &ChannelProbePoint{
					Timestamp:             ts,
					TotalRequestCount:     p.TotalRequestCount,
					SuccessRequestCount:   p.SuccessRequestCount,
					AvgTokensPerSecond:    p.AvgTokensPerSecond,
					AvgTimeToFirstTokenMs: p.AvgTimeToFirstTokenMs,
					ActiveProbeLatencyMs:  p.ActiveProbeLatencyMs,
					ProbeModelLatencyMs:   p.ProbeModelLatencyMs,
				})
			} else {
				// Fill missing point with 0
				points = append(points, &ChannelProbePoint{
					Timestamp:           ts,
					TotalRequestCount:   0,
					SuccessRequestCount: 0,
				})
			}
		}

		result = append(result, &ChannelProbeData{
			ChannelID: channelID,
			Points:    points,
		})
	}

	return result, nil
}

// RunProbeNow manually triggers the probe task.
func (svc *ChannelProbeService) RunProbeNow(ctx context.Context) {
	svc.runProbeWithMode(ctx, true)
}

// GetProbesByChannelID returns probe data for a single channel.
func (svc *ChannelProbeService) GetProbesByChannelID(ctx context.Context, channelID int) ([]*ChannelProbePoint, error) {
	data, err := svc.QueryChannelProbes(ctx, []int{channelID})
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return []*ChannelProbePoint{}, nil
	}

	return data[0].Points, nil
}

// GetChannelProbeDataInput is the input for batch query.
type GetChannelProbeDataInput struct {
	ChannelIDs []int `json:"channel_ids"`
}

type GetChannelHealthSnapshotsInput struct {
	ChannelIDs []int `json:"channel_ids"`
}

func (svc *ChannelProbeService) QueryLatestChannelHealthSnapshots(_ context.Context, input GetChannelHealthSnapshotsInput) ([]*ChannelHealthSnapshot, error) {
	if svc.ChannelService == nil || len(input.ChannelIDs) == 0 {
		return []*ChannelHealthSnapshot{}, nil
	}

	snapshots := make([]*ChannelHealthSnapshot, 0, len(input.ChannelIDs))
	for _, channelID := range input.ChannelIDs {
		health, ok := svc.ChannelService.GetChannelProbeHealth(channelID)
		if !ok || health == nil {
			continue
		}

		snapshots = append(snapshots, &ChannelHealthSnapshot{
			ChannelID:              channelID,
			ProbeHealthRecorded:    health.ProbeHealthRecorded,
			Alive:                  health.Alive,
			ModelsAlive:            health.ModelsAlive,
			ProbeModelAlive:        health.ProbeModelAlive,
			ActiveProbeLatencyMs:   cloneFloat64Ptr(health.ActiveProbeLatencyMs),
			ProbeModelLatencyMs:    cloneFloat64Ptr(health.ProbeModelLatencyMs),
			ProbeTimestamp:         health.Timestamp,
			ObservedHealthRecorded: health.ObservedHealthRecorded,
			ObservedAlive:          health.ObservedAlive,
			ObservedLatencyMs:      cloneFloat64Ptr(health.ObservedLatencyMs),
			ObservedTimestamp:      health.ObservedTimestamp,
		})
	}

	return snapshots, nil
}

// BatchQueryChannelProbes is an alias for QueryChannelProbes for GraphQL.
func (svc *ChannelProbeService) BatchQueryChannelProbes(ctx context.Context, input GetChannelProbeDataInput) ([]*ChannelProbeData, error) {
	if len(input.ChannelIDs) == 0 {
		return []*ChannelProbeData{}, nil
	}

	return svc.QueryChannelProbes(ctx, input.ChannelIDs)
}
