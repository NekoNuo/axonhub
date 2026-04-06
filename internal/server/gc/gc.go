package gc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"entgo.io/ent/dialect"
	"github.com/zhenzou/executors"
	"go.uber.org/fx"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channelprobe"
	"github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/ent/requestexecution"
	"github.com/looplj/axonhub/internal/ent/schema/schematype"
	"github.com/looplj/axonhub/internal/ent/thread"
	"github.com/looplj/axonhub/internal/ent/trace"
	"github.com/looplj/axonhub/internal/ent/usagelog"
	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/server/biz"
)

// defaultBatchSize is the default batch size for cleanup operations
// This can be overridden for testing.
var defaultBatchSize = 500
var idleMaintenanceInterval = time.Minute

type Config struct {
	CRON          string `json:"cron" yaml:"cron" conf:"cron" validate:"required"`
	VacuumEnabled bool   `json:"vacuum_enabled" yaml:"vacuum_enabled" conf:"vacuum_enabled"`
	VacuumFull    bool   `json:"vacuum_full" yaml:"vacuum_full" conf:"vacuum_full"`
}

// Worker handles garbage collection and cleanup operations.
type Worker struct {
	SystemService         *biz.SystemService
	DataStorageService    *biz.DataStorageService
	Executor              executors.ScheduledExecutor
	Ent                   *ent.Client
	Config                Config
	CancelFunc            context.CancelFunc
	IdleCancelFunc        context.CancelFunc
	maintenanceMu         sync.Mutex
	lastIdleMaintenanceAt time.Time
	now                   func() time.Time
	latestWriteAtFunc     func(ctx context.Context) (time.Time, error)
	statsFunc             func(ctx context.Context) (dbMaintenanceStats, error)
	checkpointFunc        func(ctx context.Context) error
	vacuumFunc            func(ctx context.Context) error
}

type Params struct {
	fx.In

	Config             Config
	SystemService      *biz.SystemService
	DataStorageService *biz.DataStorageService
	Client             *ent.Client
}

// NewWorker creates a new GCService with daily cleanup scheduling.
func NewWorker(params Params) *Worker {
	return &Worker{
		SystemService:      params.SystemService,
		DataStorageService: params.DataStorageService,
		Executor:           executors.NewPoolScheduleExecutor(executors.WithMaxConcurrent(1)),
		Ent:                params.Client,
		Config:             params.Config,
	}
}

type dbMaintenanceStats struct {
	PageSize      int64
	PageCount     int64
	FreeListCount int64
}

func (s dbMaintenanceStats) dbSizeBytes() int64 {
	return s.PageSize * s.PageCount
}

func (w *Worker) shouldRunIdleMaintenance(settings biz.IdleDBMaintenance, stats dbMaintenanceStats) bool {
	if !settings.Enabled {
		return false
	}

	if settings.MinFreePages > 0 && stats.FreeListCount >= int64(settings.MinFreePages) {
		return true
	}

	if settings.MinDBSizeMB > 0 && stats.dbSizeBytes() >= int64(settings.MinDBSizeMB)*1024*1024 {
		return true
	}

	return false
}

func (w *Worker) canRunIdleMaintenance(now time.Time, settings biz.IdleDBMaintenance) bool {
	if !settings.Enabled {
		return false
	}

	w.maintenanceMu.Lock()
	lastRunAt := w.lastIdleMaintenanceAt
	w.maintenanceMu.Unlock()

	if settings.CooldownMinutes <= 0 {
		return true
	}

	if lastRunAt.IsZero() {
		return true
	}

	return now.Sub(lastRunAt) >= time.Duration(settings.CooldownMinutes)*time.Minute
}

func (w *Worker) latestRequestWriteAt(ctx context.Context) (time.Time, error) {
	var latest time.Time

	req, err := w.Ent.Request.Query().
		Order(ent.Desc(request.FieldUpdatedAt)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return time.Time{}, fmt.Errorf("failed to query latest request write: %w", err)
	}
	if err == nil && req.UpdatedAt.After(latest) {
		latest = req.UpdatedAt
	}

	exec, err := w.Ent.RequestExecution.Query().
		Order(ent.Desc(requestexecution.FieldUpdatedAt)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return time.Time{}, fmt.Errorf("failed to query latest request execution write: %w", err)
	}
	if err == nil && exec.UpdatedAt.After(latest) {
		latest = exec.UpdatedAt
	}

	usage, err := w.Ent.UsageLog.Query().
		Order(ent.Desc(usagelog.FieldUpdatedAt)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return time.Time{}, fmt.Errorf("failed to query latest usage log write: %w", err)
	}
	if err == nil && usage.UpdatedAt.After(latest) {
		latest = usage.UpdatedAt
	}

	return latest, nil
}

func (w *Worker) nowTime() time.Time {
	if w.now != nil {
		return w.now()
	}

	return time.Now()
}

func (w *Worker) latestRequestWriteTimestamp(ctx context.Context) (time.Time, error) {
	if w.latestWriteAtFunc != nil {
		return w.latestWriteAtFunc(ctx)
	}

	return w.latestRequestWriteAt(ctx)
}

func (w *Worker) maintenanceStats(ctx context.Context) (dbMaintenanceStats, error) {
	if w.statsFunc != nil {
		return w.statsFunc(ctx)
	}

	sqlDriver, ok := w.Ent.Driver().(*entsql.Driver)
	if !ok {
		return dbMaintenanceStats{}, fmt.Errorf("database driver is not *entsql.Driver")
	}

	if sqlDriver.Dialect() != dialect.SQLite {
		return dbMaintenanceStats{}, fmt.Errorf("idle maintenance only supports sqlite")
	}

	db := sqlDriver.DB()
	var stats dbMaintenanceStats

	if err := db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&stats.PageSize); err != nil {
		return dbMaintenanceStats{}, fmt.Errorf("failed to query page_size: %w", err)
	}

	if err := db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&stats.PageCount); err != nil {
		return dbMaintenanceStats{}, fmt.Errorf("failed to query page_count: %w", err)
	}

	if err := db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&stats.FreeListCount); err != nil {
		return dbMaintenanceStats{}, fmt.Errorf("failed to query freelist_count: %w", err)
	}

	return stats, nil
}

func (w *Worker) runCheckpointTruncate(ctx context.Context) error {
	if w.checkpointFunc != nil {
		return w.checkpointFunc(ctx)
	}

	sqlDriver, ok := w.Ent.Driver().(*entsql.Driver)
	if !ok {
		return fmt.Errorf("database driver is not *entsql.Driver")
	}

	if sqlDriver.Dialect() != dialect.SQLite {
		return fmt.Errorf("idle maintenance only supports sqlite")
	}

	var busy, walFrames, checkpointed int
	if err := sqlDriver.DB().QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &walFrames, &checkpointed); err != nil {
		return fmt.Errorf("failed to run wal checkpoint truncate: %w", err)
	}

	log.Debug(ctx, "SQLite WAL checkpoint completed",
		log.Int("busy", busy),
		log.Int("wal_frames", walFrames),
		log.Int("checkpointed_frames", checkpointed),
	)

	return nil
}

func (w *Worker) runVacuumNow(ctx context.Context) error {
	if w.vacuumFunc != nil {
		return w.vacuumFunc(ctx)
	}

	return w.runVacuum(ctx)
}

func (w *Worker) isIdleEnough(lastWriteAt, now time.Time, settings biz.IdleDBMaintenance) bool {
	if lastWriteAt.IsZero() || settings.IdleMinutes <= 0 {
		return true
	}

	return now.Sub(lastWriteAt) >= time.Duration(settings.IdleMinutes)*time.Minute
}

func (w *Worker) markIdleMaintenanceRun(at time.Time) {
	w.maintenanceMu.Lock()
	w.lastIdleMaintenanceAt = at
	w.maintenanceMu.Unlock()
}

func (w *Worker) supportsIdleMaintenance() bool {
	sqlDriver, ok := w.Ent.Driver().(*entsql.Driver)
	if !ok {
		return false
	}

	return sqlDriver.Dialect() == dialect.SQLite
}

// deleteInBatches deletes records in batches to avoid memory issues
// This function repeatedly executes the delete query until no more records are deleted.
func (w *Worker) deleteInBatches(ctx context.Context, deleteFunc func() (int, error)) (int, error) {
	totalDeleted := 0

	for {
		// Delete a batch of records
		deleted, err := deleteFunc()
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to delete batch: %w", err)
		}

		if deleted == 0 {
			// No more records to delete
			break
		}

		totalDeleted += deleted
		log.Debug(ctx, "Deleted batch of records", log.Int("batch_size", deleted), log.Int("total_deleted", totalDeleted))
	}

	return totalDeleted, nil
}

// getBatchSize returns the appropriate batch size for cleanup operations
// Returns 10 for test environment, 500 for production.
func (w *Worker) getBatchSize() int {
	// Check if running in test mode by checking context or environment
	// For now, use a default batch size that can be overridden via config if needed
	// In production, this should return 500
	// In tests, it can be overridden to 10
	return defaultBatchSize
}

func (w *Worker) Start(ctx context.Context) error {
	cancelFunc, err := w.Executor.ScheduleFuncAtCronRate(
		w.runCleanupWithSystemContext,
		executors.CRONRule{Expr: w.Config.CRON},
	)
	if err != nil {
		return err
	}

	idleCancelFunc, err := w.Executor.ScheduleFuncAtFixRate(
		w.runIdleMaintenanceWithSystemContext,
		idleMaintenanceInterval,
	)
	if err != nil {
		cancelFunc()
		return err
	}

	w.CancelFunc = cancelFunc
	w.IdleCancelFunc = idleCancelFunc

	log.Info(ctx, "GC worker started", log.String("cron", w.Config.CRON),
		log.Bool("cancel_func", w.CancelFunc != nil),
		log.Bool("idle_cancel_func", w.IdleCancelFunc != nil),
		log.Bool("ent", w.Ent != nil),
		log.Bool("executor", w.Executor != nil),
		log.Bool("system_service", w.SystemService != nil),
	)

	return nil
}

func (w *Worker) Stop(ctx context.Context) error {
	if w.CancelFunc != nil {
		w.CancelFunc()
	}

	if w.IdleCancelFunc != nil {
		w.IdleCancelFunc()
	}

	return w.Executor.Shutdown(ctx)
}

// runCleanup executes the cleanup process based on storage policy.
func (w *Worker) runCleanup(ctx context.Context, manual bool) {
	log.Info(ctx, "Starting automatic cleanup process")

	ctx = ent.NewContext(ctx, w.Ent)
	ctx = schematype.SkipSoftDelete(ctx)

	// Get storage policy
	policy, err := w.SystemService.StoragePolicy(ctx)
	if err != nil {
		log.Error(ctx, "Failed to get storage policy for cleanup", log.Cause(err))
		return
	}

	log.Debug(ctx, "Storage policy for cleanup", log.Any("policy", policy))

	// Execute cleanup for each resource type
	for _, option := range policy.CleanupOptions {
		if option.Enabled {
			switch option.ResourceType {
			case "requests":
				err := w.cleanupRequests(ctx, option.CleanupDays, manual)
				if err != nil {
					log.Error(ctx, "Failed to cleanup requests",
						log.String("resource", option.ResourceType),
						log.Cause(err))
				} else {
					log.Info(ctx, "Successfully cleaned up requests",
						log.String("resource", option.ResourceType),
						log.Int("cleanup_days", option.CleanupDays))
				}

				err = w.cleanupThreads(ctx, option.CleanupDays, manual)
				if err != nil {
					log.Error(ctx, "Failed to cleanup threads",
						log.String("resource", "threads"),
						log.Cause(err))
				} else {
					log.Info(ctx, "Successfully cleaned up threads",
						log.String("resource", "threads"),
						log.Int("cleanup_days", option.CleanupDays))
				}

				err = w.cleanupTraces(ctx, option.CleanupDays, manual)
				if err != nil {
					log.Error(ctx, "Failed to cleanup traces",
						log.String("resource", "traces"),
						log.Cause(err))
				} else {
					log.Info(ctx, "Successfully cleaned up traces",
						log.String("resource", "traces"),
						log.Int("cleanup_days", option.CleanupDays))
				}
			case "usage_logs":
				err := w.cleanupUsageLogs(ctx, option.CleanupDays, manual)
				if err != nil {
					log.Error(ctx, "Failed to cleanup usage logs",
						log.String("resource", option.ResourceType),
						log.Cause(err))
				} else {
					log.Info(ctx, "Successfully cleaned up usage logs",
						log.String("resource", option.ResourceType),
						log.Int("cleanup_days", option.CleanupDays))
				}
			default:
				log.Warn(ctx, "Unknown resource type for cleanup",
					log.String("resource", option.ResourceType))
			}
		}
	}

	// Always cleanup channel probe data older than 3 days
	err = w.cleanupChannelProbes(ctx, 3, manual)
	if err != nil {
		log.Error(ctx, "Failed to cleanup channel probes",
			log.Cause(err))
	} else {
		log.Info(ctx, "Successfully cleaned up channel probes",
			log.Int("cleanup_days", 3))
	}

	// Run VACUUM after cleanup to reclaim storage space (SQLite and PostgreSQL)
	if w.Config.VacuumEnabled {
		if err := w.runVacuum(ctx); err != nil {
			log.Error(ctx, "Failed to run VACUUM after cleanup",
				log.Cause(err))
		}
	}

	log.Info(ctx, "Automatic cleanup process completed")
}

func (w *Worker) runIdleMaintenanceCheck(ctx context.Context) {
	if !w.supportsIdleMaintenance() {
		return
	}

	ctx = ent.NewContext(ctx, w.Ent)
	ctx = schematype.SkipSoftDelete(ctx)

	policy, err := w.SystemService.StoragePolicy(ctx)
	if err != nil {
		log.Error(ctx, "Failed to get storage policy for idle maintenance", log.Cause(err))
		return
	}

	w.runIdleMaintenance(ctx, policy.IdleDBMaintenance)
}

func (w *Worker) runIdleMaintenance(ctx context.Context, settings biz.IdleDBMaintenance) {
	if !settings.Enabled {
		return
	}

	now := w.nowTime()
	if !w.canRunIdleMaintenance(now, settings) {
		return
	}

	lastWriteAt, err := w.latestRequestWriteTimestamp(ctx)
	if err != nil {
		log.Error(ctx, "Failed to determine latest request write time", log.Cause(err))
		return
	}

	if !w.isIdleEnough(lastWriteAt, now, settings) {
		return
	}

	if err := w.runCheckpointTruncate(ctx); err != nil {
		log.Error(ctx, "Failed to run idle WAL checkpoint", log.Cause(err))
		return
	}

	stats, err := w.maintenanceStats(ctx)
	if err != nil {
		log.Error(ctx, "Failed to collect idle maintenance stats", log.Cause(err))
		return
	}

	now = w.nowTime()
	lastWriteAt, err = w.latestRequestWriteTimestamp(ctx)
	if err != nil {
		log.Error(ctx, "Failed to recheck latest request write time", log.Cause(err))
		return
	}

	if !w.isIdleEnough(lastWriteAt, now, settings) {
		return
	}

	if !w.shouldRunIdleMaintenance(settings, stats) {
		w.markIdleMaintenanceRun(now)
		log.Debug(ctx, "Idle maintenance skipped VACUUM after checkpoint",
			log.Int64("page_count", stats.PageCount),
			log.Int64("free_list_count", stats.FreeListCount),
			log.Int64("db_size_bytes", stats.dbSizeBytes()),
		)
		return
	}

	if err := w.runVacuumNow(ctx); err != nil {
		log.Error(ctx, "Failed to run idle VACUUM", log.Cause(err))
		return
	}

	w.markIdleMaintenanceRun(now)
	log.Info(ctx, "Idle database maintenance completed",
		log.Int64("page_count", stats.PageCount),
		log.Int64("free_list_count", stats.FreeListCount),
		log.Int64("db_size_bytes", stats.dbSizeBytes()),
	)
}

// cleanupRequests deletes requests older than the specified number of days.
func (w *Worker) cleanupRequests(ctx context.Context, cleanupDays int, manual bool) error {
	if !manual && cleanupDays <= 0 {
		log.Debug(ctx, "No cleanup needed for requests")
		return nil // No cleanup needed
	}

	cutoffTime := time.Now().AddDate(0, 0, -cleanupDays)
	if manual && cleanupDays == 0 {
		cutoffTime = time.Now()
	}

	execResult, err := w.cleanupOldRequestExecutions(ctx, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to cleanup request executions: %w", err)
	}

	log.Debug(ctx, "Deleted old request executions",
		log.Int("deleted_executions_count", execResult),
		log.Time("cutoff_time", cutoffTime),
	)

	reqResult, err := w.cleanupOldRequestsRecords(ctx, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to cleanup requests: %w", err)
	}

	log.Debug(ctx, "Deleted old requests",
		log.Int("deleted_requests_count", reqResult),
		log.Time("cutoff_time", cutoffTime))

	return nil
}

func (w *Worker) cleanupOldRequestExecutions(ctx context.Context, cutoffTime time.Time) (int, error) {
	batchSize := w.getBatchSize()
	totalDeleted := 0
	cache := make(map[int]*ent.DataStorage)

	for {
		executions, err := w.Ent.RequestExecution.Query().
			Where(requestexecution.CreatedAtLT(cutoffTime)).
			Order(ent.Asc(requestexecution.FieldID)).
			Limit(batchSize).
			All(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to query old request executions: %w", err)
		}

		if len(executions) == 0 {
			break
		}

		ids := make([]int, len(executions))

		for i, exec := range executions {
			ids[i] = exec.ID
			w.cleanupExecutionExternalStorage(ctx, exec, cache)
		}

		if _, err := w.Ent.RequestExecution.Delete().
			Where(requestexecution.IDIn(ids...)).
			Exec(ctx); err != nil {
			return totalDeleted, fmt.Errorf("failed to delete request executions batch: %w", err)
		}

		log.Debug(ctx, "Deleted old request executions batch",
			log.Int("deleted_executions_count", len(ids)),
			log.Time("cutoff_time", cutoffTime),
		)

		totalDeleted += len(ids)
	}

	return totalDeleted, nil
}

func (w *Worker) cleanupOldRequestsRecords(ctx context.Context, cutoffTime time.Time) (int, error) {
	batchSize := w.getBatchSize()
	totalDeleted := 0
	cache := make(map[int]*ent.DataStorage)

	for {
		reqs, err := w.Ent.Request.Query().
			Where(request.CreatedAtLT(cutoffTime)).
			Order(ent.Asc(request.FieldID)).
			Limit(batchSize).
			All(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to query old requests: %w", err)
		}

		if len(reqs) == 0 {
			break
		}

		ids := make([]int, len(reqs))
		for i, req := range reqs {
			ids[i] = req.ID
			w.cleanupRequestExternalStorage(ctx, req, cache)
		}

		if _, err := w.Ent.Request.Delete().
			Where(request.IDIn(ids...)).
			Exec(ctx); err != nil {
			return totalDeleted, fmt.Errorf("failed to delete requests batch: %w", err)
		}

		totalDeleted += len(ids)
	}

	return totalDeleted, nil
}

func (w *Worker) cleanupExecutionExternalStorage(ctx context.Context, exec *ent.RequestExecution, cache map[int]*ent.DataStorage) {
	if exec == nil || exec.DataStorageID == 0 || w.DataStorageService == nil {
		return
	}

	ds, err := w.getDataStorageCached(ctx, exec.DataStorageID, cache)
	if err != nil {
		log.Warn(ctx, "Failed to load data storage for execution cleanup",
			log.Cause(err),
			log.Int("execution_id", exec.ID),
		)

		return
	}

	if ds == nil || ds.Primary {
		return
	}

	keys := []string{
		biz.GenerateExecutionRequestBodyKey(exec.ProjectID, exec.RequestID, exec.ID),
		biz.GenerateExecutionResponseBodyKey(exec.ProjectID, exec.RequestID, exec.ID),
		biz.GenerateExecutionResponseChunksKey(exec.ProjectID, exec.RequestID, exec.ID),
		biz.GenerateExecutionRequestDirKey(exec.ProjectID, exec.RequestID, exec.ID),
	}

	for _, key := range keys {
		if err := w.DataStorageService.DeleteData(ctx, ds, key); err != nil {
			log.Warn(ctx, "Failed to delete execution external data",
				log.Cause(err),
				log.Int("execution_id", exec.ID),
				log.String("key", key),
			)
		}
	}
}

func (w *Worker) cleanupRequestExternalStorage(ctx context.Context, req *ent.Request, cache map[int]*ent.DataStorage) {
	if req == nil || req.DataStorageID == 0 || w.DataStorageService == nil {
		return
	}

	ds, err := w.getDataStorageCached(ctx, req.DataStorageID, cache)
	if err != nil {
		log.Warn(ctx, "Failed to load data storage for request cleanup",
			log.Cause(err),
			log.Int("request_id", req.ID),
		)

		return
	}

	if ds == nil || ds.Primary {
		return
	}

	keys := []string{
		biz.GenerateRequestBodyKey(req.ProjectID, req.ID),
		biz.GenerateResponseBodyKey(req.ProjectID, req.ID),
		biz.GenerateResponseChunksKey(req.ProjectID, req.ID),
		biz.GenerateRequestExecutionsDirKey(req.ProjectID, req.ID),
		biz.GenerateRequestDirKey(req.ProjectID, req.ID),
	}

	for _, key := range keys {
		if err := w.DataStorageService.DeleteData(ctx, ds, key); err != nil {
			log.Warn(ctx, "Failed to delete request external data",
				log.Cause(err),
				log.Int("request_id", req.ID),
				log.String("key", key),
			)
		}
	}
}

func (w *Worker) getDataStorageCached(ctx context.Context, id int, cache map[int]*ent.DataStorage) (*ent.DataStorage, error) {
	if ds, ok := cache[id]; ok {
		return ds, nil
	}

	ds, err := w.DataStorageService.GetDataStorageByID(ctx, id)
	if err != nil {
		return nil, err
	}

	cache[id] = ds

	return ds, nil
}

// cleanupUsageLogs deletes usage logs older than the specified number of days.
func (w *Worker) cleanupUsageLogs(ctx context.Context, cleanupDays int, manual bool) error {
	if !manual && cleanupDays <= 0 {
		return nil // No cleanup needed
	}

	cutoffTime := time.Now().AddDate(0, 0, -cleanupDays)
	if manual && cleanupDays == 0 {
		cutoffTime = time.Now()
	}

	// Delete usage logs in batches
	result, err := w.deleteInBatches(ctx, func() (int, error) {
		return w.Ent.UsageLog.Delete().Where(usagelog.CreatedAtLT(cutoffTime)).Exec(ctx)
	})
	if err != nil {
		return fmt.Errorf("failed to delete old usage logs: %w", err)
	}

	log.Debug(ctx, "Cleaned up usage logs",
		log.Int("deleted_count", result),
		log.Time("cutoff_time", cutoffTime))

	return nil
}

// cleanupThreads deletes threads older than the specified number of days.
func (w *Worker) cleanupThreads(ctx context.Context, cleanupDays int, manual bool) error {
	if !manual && cleanupDays <= 0 {
		log.Debug(ctx, "No cleanup needed for threads")
		return nil // No cleanup needed
	}

	cutoffTime := time.Now().AddDate(0, 0, -cleanupDays)
	if manual && cleanupDays == 0 {
		cutoffTime = time.Now()
	}

	// Delete threads in batches
	result, err := w.deleteInBatches(ctx, func() (int, error) {
		return w.Ent.Thread.Delete().Where(thread.CreatedAtLT(cutoffTime)).Exec(ctx)
	})
	if err != nil {
		return fmt.Errorf("failed to delete old threads: %w", err)
	}

	log.Debug(ctx, "Cleaned up threads",
		log.Int("deleted_count", result),
		log.Time("cutoff_time", cutoffTime))

	return nil
}

// cleanupTraces deletes traces older than the specified number of days.
func (w *Worker) cleanupTraces(ctx context.Context, cleanupDays int, manual bool) error {
	if !manual && cleanupDays <= 0 {
		log.Debug(ctx, "No cleanup needed for traces")
		return nil // No cleanup needed
	}

	cutoffTime := time.Now().AddDate(0, 0, -cleanupDays)
	if manual && cleanupDays == 0 {
		cutoffTime = time.Now()
	}

	// Delete traces in batches
	result, err := w.deleteInBatches(ctx, func() (int, error) {
		return w.Ent.Trace.Delete().Where(trace.CreatedAtLT(cutoffTime)).Exec(ctx)
	})
	if err != nil {
		return fmt.Errorf("failed to delete old traces: %w", err)
	}

	log.Debug(ctx, "Cleaned up traces",
		log.Int("deleted_count", result),
		log.Time("cutoff_time", cutoffTime))

	return nil
}

// cleanupChannelProbes deletes channel probes older than the specified number of days.
func (w *Worker) cleanupChannelProbes(ctx context.Context, cleanupDays int, manual bool) error {
	if !manual && cleanupDays <= 0 {
		log.Debug(ctx, "No cleanup needed for channel probes")
		return nil // No cleanup needed
	}

	cutoffTime := time.Now().AddDate(0, 0, -cleanupDays)
	if manual && cleanupDays == 0 {
		cutoffTime = time.Now()
	}

	result, err := w.deleteInBatches(ctx, func() (int, error) {
		return w.Ent.ChannelProbe.Delete().Where(channelprobe.TimestampLT(cutoffTime.Unix())).Exec(ctx)
	})
	if err != nil {
		return fmt.Errorf("failed to delete old channel probes: %w", err)
	}

	log.Debug(ctx, "Cleaned up channel probes",
		log.Int("deleted_count", result),
		log.Time("cutoff_time", cutoffTime))

	return nil
}

// runVacuum executes VACUUM command on SQLite/PostgreSQL database to reclaim storage space.
// This should be called after cleanup operations to defragment the database file.
func (w *Worker) runVacuum(ctx context.Context) error {
	return w.executeVacuum(ctx, false)
}

func (w *Worker) executeVacuum(ctx context.Context, force bool) error {
	if !force && !w.Config.VacuumEnabled {
		log.Debug(ctx, "VACUUM is disabled, skipping")
		return nil
	}

	// Get the underlying SQL driver to check if it's SQLite
	dbDriver := w.Ent.Driver()
	if dbDriver == nil {
		return fmt.Errorf("failed to get database driver")
	}

	// Try to cast to *entsql.Driver to access underlying *sql.DB
	sqlDriver, ok := dbDriver.(*entsql.Driver)
	if !ok {
		log.Debug(ctx, "Database driver is not *entsql.Driver, skipping VACUUM")
		return nil
	}

	// Check if this is SQLite or PostgreSQL
	if sqlDriver.Dialect() != dialect.SQLite && sqlDriver.Dialect() != dialect.Postgres {
		log.Debug(ctx, "Database does not support VACUUM, skipping",
			log.String("dialect", sqlDriver.Dialect()))

		return nil
	}

	log.Info(ctx, "Starting database VACUUM operation",
		log.String("dialect", sqlDriver.Dialect()),
		log.Bool("vacuum_full", w.Config.VacuumFull))

	startTime := time.Now()

	// Execute VACUUM using raw SQL
	var vacuumSQL string
	if sqlDriver.Dialect() == dialect.Postgres && w.Config.VacuumFull {
		vacuumSQL = "VACUUM FULL"
	} else {
		vacuumSQL = "VACUUM"
	}

	_, err := sqlDriver.ExecContext(ctx, vacuumSQL, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to execute %s: %w", vacuumSQL, err)
	}

	duration := time.Since(startTime)
	log.Info(ctx, "Database VACUUM completed successfully",
		log.Duration("duration", duration),
		log.String("command", vacuumSQL))

	return nil
}

// RunVacuumNow manually triggers the VACUUM operation.
// This can be useful for testing or manual execution.
func (w *Worker) RunVacuumNow(ctx context.Context) error {
	return w.executeVacuum(ctx, true)
}

// RunCleanupNow manually triggers the cleanup process.
// This can be useful for testing or manual execution.
func (w *Worker) RunCleanupNow(ctx context.Context) error {
	w.runCleanup(ctx, true)
	return nil
}
