package gc

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zhenzou/executors"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/datastorage"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/internal/server/biz"
)

func TestWorker_ShouldRunIdleMaintenance(t *testing.T) {
	worker := &Worker{}

	settings := biz.IdleDBMaintenance{
		Enabled:         true,
		IdleMinutes:     15,
		MinFreePages:    100,
		MinDBSizeMB:     64,
		CooldownMinutes: 30,
	}

	t.Run("free pages threshold", func(t *testing.T) {
		shouldRun := worker.shouldRunIdleMaintenance(settings, dbMaintenanceStats{
			PageSize:      4096,
			PageCount:     1024,
			FreeListCount: 150,
		})
		require.True(t, shouldRun)
	})

	t.Run("db size threshold", func(t *testing.T) {
		shouldRun := worker.shouldRunIdleMaintenance(settings, dbMaintenanceStats{
			PageSize:      1024 * 1024,
			PageCount:     80,
			FreeListCount: 5,
		})
		require.True(t, shouldRun)
	})

	t.Run("disabled thresholds do not trigger", func(t *testing.T) {
		shouldRun := worker.shouldRunIdleMaintenance(settings, dbMaintenanceStats{
			PageSize:      4096,
			PageCount:     100,
			FreeListCount: 10,
		})
		require.False(t, shouldRun)
	})
}

func TestWorker_MaintenanceCooldown(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	worker := &Worker{
		lastIdleMaintenanceAt: now.Add(-10 * time.Minute),
	}

	settings := biz.IdleDBMaintenance{
		Enabled:         true,
		CooldownMinutes: 30,
	}

	require.False(t, worker.canRunIdleMaintenance(now, settings))

	worker.lastIdleMaintenanceAt = now.Add(-31 * time.Minute)
	require.True(t, worker.canRunIdleMaintenance(now, settings))
}

func TestWorker_LatestRequestWriteAt(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	base := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)

	_, err := client.Request.Create().
		SetProjectID(1).
		SetModelID("gpt-5.4").
		SetFormat("openai/chat_completions").
		SetRequestBody([]byte(`{}`)).
		SetStatus("completed").
		SetStream(false).
		SetClientIP("127.0.0.1").
		SetCreatedAt(base).
		SetUpdatedAt(base).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.RequestExecution.Create().
		SetProjectID(1).
		SetRequestID(1).
		SetChannelID(1).
		SetModelID("gpt-5.4").
		SetFormat("openai/chat_completions").
		SetRequestBody([]byte(`{}`)).
		SetStatus("completed").
		SetStream(false).
		SetCreatedAt(base.Add(5 * time.Minute)).
		SetUpdatedAt(base.Add(5 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetProjectID(1).
		SetRequestID(1).
		SetModelID("gpt-5.4").
		SetPromptTokens(1).
		SetCompletionTokens(1).
		SetTotalTokens(2).
		SetCreatedAt(base.Add(8 * time.Minute)).
		SetUpdatedAt(base.Add(8 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	worker := &Worker{Ent: client}

	lastWriteAt, err := worker.latestRequestWriteAt(ctx)
	require.NoError(t, err)
	require.Equal(t, base.Add(8*time.Minute), lastWriteAt)
}

func TestWorker_RunIdleMaintenance(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	settings := biz.IdleDBMaintenance{
		Enabled:         true,
		IdleMinutes:     15,
		MinFreePages:    100,
		MinDBSizeMB:     64,
		CooldownMinutes: 30,
	}

	t.Run("checkpoint then vacuum when thresholds match", func(t *testing.T) {
		checkpointCalls := 0
		vacuumCalls := 0

		worker := &Worker{
			now: func() time.Time { return now },
			latestWriteAtFunc: func(context.Context) (time.Time, error) {
				return now.Add(-20 * time.Minute), nil
			},
			statsFunc: func(context.Context) (dbMaintenanceStats, error) {
				return dbMaintenanceStats{
					PageSize:      4096,
					PageCount:     50000,
					FreeListCount: 200,
				}, nil
			},
			checkpointFunc: func(context.Context) error {
				checkpointCalls++
				return nil
			},
			vacuumFunc: func(context.Context) error {
				vacuumCalls++
				return nil
			},
		}

		worker.runIdleMaintenance(ctx, settings)

		require.Equal(t, 1, checkpointCalls)
		require.Equal(t, 1, vacuumCalls)
		require.Equal(t, now, worker.lastIdleMaintenanceAt)
	})

	t.Run("checkpoint only when thresholds do not match", func(t *testing.T) {
		checkpointCalls := 0
		vacuumCalls := 0

		worker := &Worker{
			now: func() time.Time { return now },
			latestWriteAtFunc: func(context.Context) (time.Time, error) {
				return now.Add(-20 * time.Minute), nil
			},
			statsFunc: func(context.Context) (dbMaintenanceStats, error) {
				return dbMaintenanceStats{
					PageSize:      4096,
					PageCount:     100,
					FreeListCount: 5,
				}, nil
			},
			checkpointFunc: func(context.Context) error {
				checkpointCalls++
				return nil
			},
			vacuumFunc: func(context.Context) error {
				vacuumCalls++
				return nil
			},
		}

		worker.runIdleMaintenance(ctx, settings)

		require.Equal(t, 1, checkpointCalls)
		require.Zero(t, vacuumCalls)
		require.Equal(t, now, worker.lastIdleMaintenanceAt)
	})
}

func TestWorker_getBatchSize(t *testing.T) {
	worker := &Worker{
		Ent:    nil,
		Config: Config{CRON: "0 0 * * *"},
	}

	// Test default batch size
	batchSize := worker.getBatchSize()
	if batchSize != defaultBatchSize {
		t.Errorf("Expected batch size %d, got %d", defaultBatchSize, batchSize)
	}

	// Test with overridden batch size
	originalBatchSize := defaultBatchSize
	defaultBatchSize = 20

	defer func() { defaultBatchSize = originalBatchSize }()

	batchSize = worker.getBatchSize()
	if batchSize != 20 {
		t.Errorf("Expected batch size 20, got %d", batchSize)
	}
}

func TestWorker_cleanupRequestExternalStorageDeletesFsArtifacts(t *testing.T) {
	worker, ctx, dataStorage, baseDir := setupWorkerWithFSStorage(t)

	req := &ent.Request{
		ID:            101,
		ProjectID:     202,
		DataStorageID: dataStorage.ID,
	}

	fileKeys := []string{
		biz.GenerateRequestBodyKey(req.ProjectID, req.ID),
		biz.GenerateResponseBodyKey(req.ProjectID, req.ID),
		biz.GenerateResponseChunksKey(req.ProjectID, req.ID),
	}

	dirKeys := []string{
		biz.GenerateRequestExecutionsDirKey(req.ProjectID, req.ID),
		biz.GenerateRequestDirKey(req.ProjectID, req.ID),
	}

	for _, key := range fileKeys {
		createFileForKey(t, baseDir, key)
	}

	for _, key := range dirKeys {
		createDirForKey(t, baseDir, key)
	}

	worker.cleanupRequestExternalStorage(ctx, req, make(map[int]*ent.DataStorage))

	for _, key := range append(fileKeys, dirKeys...) {
		assertRemoved(t, baseDir, key)
	}
}

func TestWorker_cleanupExecutionExternalStorageDeletesFsArtifacts(t *testing.T) {
	worker, ctx, dataStorage, baseDir := setupWorkerWithFSStorage(t)

	req := &ent.Request{
		ID:            303,
		ProjectID:     404,
		DataStorageID: dataStorage.ID,
	}

	exec := &ent.RequestExecution{
		ID:            505,
		RequestID:     req.ID,
		ProjectID:     req.ProjectID,
		DataStorageID: dataStorage.ID,
	}

	fileKeys := []string{
		biz.GenerateExecutionRequestBodyKey(exec.ProjectID, exec.RequestID, exec.ID),
		biz.GenerateExecutionResponseBodyKey(exec.ProjectID, exec.RequestID, exec.ID),
		biz.GenerateExecutionResponseChunksKey(exec.ProjectID, exec.RequestID, exec.ID),
	}

	dirKeys := []string{
		biz.GenerateExecutionRequestDirKey(exec.ProjectID, exec.RequestID, exec.ID),
	}

	for _, key := range fileKeys {
		createFileForKey(t, baseDir, key)
	}

	for _, key := range dirKeys {
		createDirForKey(t, baseDir, key)
	}

	worker.cleanupExecutionExternalStorage(ctx, exec, make(map[int]*ent.DataStorage))

	for _, key := range append(fileKeys, dirKeys...) {
		assertRemoved(t, baseDir, key)
	}
}

func setupWorkerWithFSStorage(t *testing.T) (*Worker, context.Context, *ent.DataStorage, string) {
	t.Helper()

	cacheConfig := xcache.Config{
		Mode: xcache.ModeMemory,
		Memory: xcache.MemoryConfig{
			Expiration:      5 * time.Minute,
			CleanupInterval: 10 * time.Minute,
		},
	}

	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=1")

	executor := executors.NewPoolScheduleExecutor(executors.WithMaxConcurrent(1))

	t.Cleanup(func() {
		_ = executor.Shutdown(context.Background())

		client.Close()
	})

	systemService := biz.NewSystemService(biz.SystemServiceParams{CacheConfig: cacheConfig})
	dataStorageService := biz.NewDataStorageService(biz.DataStorageServiceParams{
		SystemService: systemService,
		CacheConfig:   cacheConfig,
		Executor:      executor,
		Client:        client,
	})

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	dir := t.TempDir()
	dirCopy := dir
	settings := &objects.DataStorageSettings{Directory: &dirCopy}

	dataStorage, err := client.DataStorage.Create().
		SetName("fs-storage").
		SetDescription("test fs storage").
		SetPrimary(false).
		SetType(datastorage.TypeFs).
		SetSettings(settings).
		SetStatus(datastorage.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	worker := &Worker{
		DataStorageService: dataStorageService,
		Ent:                client,
	}

	return worker, ctx, dataStorage, dir
}

func createFileForKey(t *testing.T, baseDir, key string) {
	t.Helper()

	path := pathForKey(baseDir, key)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("test"), 0o644))
}

func createDirForKey(t *testing.T, baseDir, key string) {
	t.Helper()

	path := pathForKey(baseDir, key)
	require.NoError(t, os.MkdirAll(path, 0o755))
}

func assertRemoved(t *testing.T, baseDir, key string) {
	t.Helper()

	path := pathForKey(baseDir, key)
	_, err := os.Stat(path)
	require.ErrorIs(t, err, fs.ErrNotExist, "expected %s to be removed", key)
}

func pathForKey(baseDir, key string) string {
	rel := strings.TrimPrefix(key, "/")
	return filepath.Join(baseDir, filepath.FromSlash(rel))
}

func TestWorker_deleteInBatches(t *testing.T) {
	// Test that the deleteInBatches method works correctly
	// This test verifies the loop logic without needing a real database
	worker := &Worker{
		Ent:    nil,
		Config: Config{CRON: "0 0 * * *"},
	}

	// Simulate batch deletion - delete 3 times, with decreasing counts
	callCount := 0
	deleteFunc := func() (int, error) {
		callCount++
		if callCount == 1 {
			return 30, nil
		} else if callCount == 2 {
			return 15, nil
		} else {
			return 0, nil
		}
	}

	deleted, err := worker.deleteInBatches(context.Background(), deleteFunc)
	if err != nil {
		t.Fatalf("deleteInBatches failed: %v", err)
	}

	// Verify total deleted
	if deleted != 45 {
		t.Errorf("Expected to delete 45 records total, got %d", deleted)
	}

	// Verify it stopped after third call (when 0 was returned)
	if callCount != 3 {
		t.Errorf("Expected 3 delete calls, got %d", callCount)
	}
}

func TestWorker_cleanupWithZeroDays(t *testing.T) {
	worker := &Worker{
		Ent:    nil,
		Config: Config{CRON: "0 0 * * *"},
	}

	ctx := context.Background()

	// Test with 0 days - should not error
	err := worker.cleanupRequests(ctx, 0, false)
	if err != nil {
		t.Fatalf("cleanupRequests with 0 days failed: %v", err)
	}

	// Test with negative days - should not error
	err = worker.cleanupUsageLogs(ctx, -1, false)
	if err != nil {
		t.Fatalf("cleanupUsageLogs with negative days failed: %v", err)
	}
}
