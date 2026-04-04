package biz

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/ent/modelhealthhistory"
	"github.com/looplj/axonhub/internal/ent/modelhealthsnapshot"
)

func (svc *ChannelProbeService) runModelHealthProbe(ctx context.Context, now time.Time) {
	if svc.SystemService == nil {
		return
	}

	settings := svc.SystemService.ModelSettingsOrDefault(ctx)
	if settings == nil || !settings.EnableModelProbe || svc.idleChannelModelProber == nil || svc.ChannelService == nil {
		return
	}

	recentUsage, err := NewModelHealthRecentUsageAggregator(svc.db).Aggregate(ctx, now)
	if err != nil {
		return
	}

	modelService := &ModelService{
		AbstractService: &AbstractService{db: svc.db},
	}
	targetResolver := NewModelHealthTargetResolver(svc.db, modelService)

	targets, err := targetResolver.Resolve(ctx, recentUsage)
	if err != nil {
		return
	}

	for _, target := range targets {
		bizChannel := svc.ChannelService.GetEnabledChannel(target.ChannelID)
		if bizChannel == nil {
			continue
		}

		latency, healthy, probeErr := svc.idleChannelModelProber(ctx, bizChannel.Channel, target.ActualModelID)
		if probeErr != nil {
			healthy = false
		}

		if persistErr := svc.persistModelHealthResult(ctx, target, healthy, now.Unix(), false); persistErr != nil {
			continue
		}

		_ = latency
	}
}

func (svc *ChannelProbeService) persistModelHealthResult(
	ctx context.Context,
	target ModelHealthProbeTarget,
	healthy bool,
	probedAt int64,
	manualOverride bool,
) error {
	snapshot, err := svc.db.ModelHealthSnapshot.Query().
		Where(
			modelhealthsnapshot.DisplayModelEQ(target.DisplayModel),
			modelhealthsnapshot.ChannelIDEQ(target.ChannelID),
			modelhealthsnapshot.ActualModelIDEQ(target.ActualModelID),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("query model health snapshot: %w", err)
	}

	if snapshot == nil {
		_, err = svc.db.ModelHealthSnapshot.Create().
			SetDisplayModel(target.DisplayModel).
			SetChannelID(target.ChannelID).
			SetActualModelID(target.ActualModelID).
			SetIsHealthy(healthy).
			SetManualOverride(manualOverride).
			SetProbedAt(probedAt).
			Save(ctx)
	} else {
		_, err = snapshot.Update().
			SetIsHealthy(healthy).
			SetManualOverride(manualOverride).
			SetProbedAt(probedAt).
			Save(ctx)
	}
	if err != nil {
		return fmt.Errorf("upsert model health snapshot: %w", err)
	}

	_, err = svc.db.ModelHealthHistory.Create().
		SetDisplayModel(target.DisplayModel).
		SetChannelID(target.ChannelID).
		SetActualModelID(target.ActualModelID).
		SetIsHealthy(healthy).
		SetManualOverride(manualOverride).
		SetProbedAt(probedAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("create model health history: %w", err)
	}

	return nil
}

func (svc *ChannelProbeService) RunManualModelProbe(ctx context.Context, target ModelHealthProbeTarget, now time.Time) error {
	if svc.ChannelService == nil || svc.idleChannelModelProber == nil {
		return nil
	}

	bizChannel := svc.ChannelService.GetEnabledChannel(target.ChannelID)
	if bizChannel == nil {
		return nil
	}

	_, healthy, probeErr := svc.idleChannelModelProber(ctx, bizChannel.Channel, target.ActualModelID)
	if probeErr != nil {
		healthy = false
	}

	return svc.persistModelHealthResult(ctx, target, healthy, now.Unix(), true)
}

func (svc *ChannelProbeService) GetEffectiveModelHealth(ctx context.Context, target ModelHealthProbeTarget) (*ent.ModelHealthSnapshot, error) {
	snapshot, err := svc.db.ModelHealthSnapshot.Query().
		Where(
			modelhealthsnapshot.DisplayModelEQ(target.DisplayModel),
			modelhealthsnapshot.ChannelIDEQ(target.ChannelID),
			modelhealthsnapshot.ActualModelIDEQ(target.ActualModelID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("query effective model health snapshot: %w", err)
	}

	if !snapshot.ManualOverride {
		return snapshot, nil
	}

	latestAuto, autoErr := svc.db.ModelHealthHistory.Query().
		Where(
			modelhealthhistory.DisplayModelEQ(target.DisplayModel),
			modelhealthhistory.ChannelIDEQ(target.ChannelID),
			modelhealthhistory.ActualModelIDEQ(target.ActualModelID),
			modelhealthhistory.ManualOverrideEQ(false),
			modelhealthhistory.ProbedAtGT(snapshot.ProbedAt),
		).
		Order(ent.Desc(modelhealthhistory.FieldProbedAt)).
		First(ctx)
	if autoErr != nil {
		if ent.IsNotFound(autoErr) {
			return snapshot, nil
		}
		return nil, fmt.Errorf("query latest automatic model health history: %w", autoErr)
	}

	if latestAuto.ProbedAt > snapshot.ProbedAt {
		snapshot.IsHealthy = latestAuto.IsHealthy
		snapshot.ManualOverride = false
		snapshot.ProbedAt = latestAuto.ProbedAt
	}

	return snapshot, nil
}

func (svc *ChannelProbeService) runAutomaticModelHealthProbe(ctx context.Context) {
	svc.runModelHealthProbe(ctx, time.Now().UTC())
}

func (svc *ChannelProbeService) listProbeEnabledModels(ctx context.Context) ([]*ent.Model, error) {
	models, err := svc.db.Model.Query().
		Where(model.StatusEQ(model.StatusEnabled)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query enabled models: %w", err)
	}

	return lo.Filter(models, func(item *ent.Model, _ int) bool {
		return item.Settings != nil && item.Settings.ProbeEnabled
	}), nil
}
