package gc

import (
	"context"

	"github.com/looplj/axonhub/internal/authz"
)

func (w *Worker) runCleanupWithSystemContext(ctx context.Context) {
	ctx = authz.WithSystemBypass(ctx, "gc-cleanup")
	w.runCleanup(ctx, false)
}

func (w *Worker) runIdleMaintenanceWithSystemContext(ctx context.Context) {
	ctx = authz.WithSystemBypass(ctx, "gc-idle-maintenance")
	w.runIdleMaintenanceCheck(ctx)
}
