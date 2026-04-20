package biz

import (
	"context"
	"fmt"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/ent/modelprobeconfig"
)

// ModelProbeConfigService manages per-triple probe configuration rows that
// record whether (displayModel, channelID, actualModelID) should be probed
// automatically, its consecutive unhealthy-probe count, and whether it has
// been auto-disabled.
type ModelProbeConfigService struct {
	db *ent.Client
}

func NewModelProbeConfigService(db *ent.Client) *ModelProbeConfigService {
	return &ModelProbeConfigService{db: db}
}

// GetByDisplayModels returns config rows filtered by displayModel. Empty
// input returns every row.
func (s *ModelProbeConfigService) GetByDisplayModels(ctx context.Context, displayModels []string) ([]*ent.ModelProbeConfig, error) {
	q := s.db.ModelProbeConfig.Query()
	if len(displayModels) > 0 {
		q = q.Where(modelprobeconfig.DisplayModelIn(displayModels...))
	}

	return q.All(ctx)
}

// Get returns the config row for a triple, or nil if it does not exist.
func (s *ModelProbeConfigService) Get(ctx context.Context, displayModel string, channelID int, actualModelID string) (*ent.ModelProbeConfig, error) {
	cfg, err := s.db.ModelProbeConfig.Query().
		Where(
			modelprobeconfig.DisplayModelEQ(displayModel),
			modelprobeconfig.ChannelIDEQ(channelID),
			modelprobeconfig.ActualModelIDEQ(actualModelID),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}

	return cfg, err
}

// SetProbeEnabled upserts the triple's config. Enabling resets the
// consecutive-failure counter and clears auto_disabled_at so the triple
// gets a fresh window before any auto-disable can fire again. Disabling
// leaves counters intact — that's a user-intent disable, not a recovery.
func (s *ModelProbeConfigService) SetProbeEnabled(ctx context.Context, displayModel string, channelID int, actualModelID string, enabled bool) (*ent.ModelProbeConfig, error) {
	existing, err := s.Get(ctx, displayModel, channelID, actualModelID)
	if err != nil {
		return nil, fmt.Errorf("get probe config: %w", err)
	}

	if existing == nil {
		return s.db.ModelProbeConfig.Create().
			SetDisplayModel(displayModel).
			SetChannelID(channelID).
			SetActualModelID(actualModelID).
			SetProbeEnabled(enabled).
			Save(ctx)
	}

	upd := existing.Update().SetProbeEnabled(enabled)
	if enabled {
		upd = upd.SetConsecutiveFailures(0).ClearAutoDisabledAt()
	}

	return upd.Save(ctx)
}

// BatchSetChannelProbeEnabled upserts every triple under
// (displayModel, channelID) as resolved from the model's associations.
// Runs inside one transaction; on error nothing is committed.
func (s *ModelProbeConfigService) BatchSetChannelProbeEnabled(ctx context.Context, displayModel string, channelID int, enabled bool) ([]*ent.ModelProbeConfig, error) {
	actualModels, err := s.resolveChannelActualModels(ctx, displayModel, channelID)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.Tx(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]*ent.ModelProbeConfig, 0, len(actualModels))

	for _, am := range actualModels {
		cfg, err := upsertProbeConfigTx(ctx, tx, displayModel, channelID, am, enabled)
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("upsert (%s,%d,%s): %w", displayModel, channelID, am, err)
		}

		out = append(out, cfg)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return out, nil
}

// resolveChannelActualModels returns the distinct actualModelIDs a model's
// associations expose on a single channel. Used by BatchSetChannelProbeEnabled
// so channel-level UI toggles touch every triple the user sees in the tree.
func (s *ModelProbeConfigService) resolveChannelActualModels(ctx context.Context, displayModel string, channelID int) ([]string, error) {
	m, err := s.db.Model.Query().
		Where(model.ModelIDEQ(displayModel)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("load model %q: %w", displayModel, err)
	}

	if m.Settings == nil || len(m.Settings.Associations) == 0 {
		return nil, nil
	}

	targetChannel, err := s.db.Channel.Get(ctx, channelID)
	if ent.IsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("load channel %d: %w", channelID, err)
	}

	targets := resolveAssociatedModelHealthTargets(m, []*ent.Channel{targetChannel})
	seen := make(map[string]struct{}, len(targets))
	actuals := make([]string, 0, len(targets))

	for _, t := range targets {
		if t.ChannelID != channelID {
			continue
		}

		if _, ok := seen[t.ActualModelID]; ok {
			continue
		}

		seen[t.ActualModelID] = struct{}{}
		actuals = append(actuals, t.ActualModelID)
	}

	return actuals, nil
}

func upsertProbeConfigTx(ctx context.Context, tx *ent.Tx, displayModel string, channelID int, actualModelID string, enabled bool) (*ent.ModelProbeConfig, error) {
	existing, err := tx.ModelProbeConfig.Query().
		Where(
			modelprobeconfig.DisplayModelEQ(displayModel),
			modelprobeconfig.ChannelIDEQ(channelID),
			modelprobeconfig.ActualModelIDEQ(actualModelID),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}

	if ent.IsNotFound(err) {
		return tx.ModelProbeConfig.Create().
			SetDisplayModel(displayModel).
			SetChannelID(channelID).
			SetActualModelID(actualModelID).
			SetProbeEnabled(enabled).
			Save(ctx)
	}

	upd := existing.Update().SetProbeEnabled(enabled)
	if enabled {
		upd = upd.SetConsecutiveFailures(0).ClearAutoDisabledAt()
	}

	return upd.Save(ctx)
}

// probeConfigKey builds the canonical string key for a probe-config triple.
// Shared with the probe loop so filters and lookups agree on the key shape.
func probeConfigKey(displayModel string, channelID int, actualModelID string) string {
	return fmt.Sprintf("%s:%d:%s", displayModel, channelID, actualModelID)
}

// UpdateOnProbeResult adjusts the triple's failure counter after an automatic
// probe. Healthy → reset to 0. Unhealthy → increment, and auto-disable once
// the count hits the per-model threshold from
// Model.settings.probeAutoDisableAfterConsecutiveFailures. Threshold 0 or a
// missing/undefined model means never auto-disable. Manual probe results do
// not call this — the counter is only moved by automatic runs.
//
// If no config row exists for the triple, this function is a no-op: the
// automatic probe pipeline only reaches triples that are already enabled
// (the skip filter guarantees config existence there), and the discovered
// snapshot refresh path writes probe results for triples that must keep
// their "default disabled" stance until the operator opts in.
func (s *ModelProbeConfigService) UpdateOnProbeResult(ctx context.Context, displayModel string, channelID int, actualModelID string, healthy bool, probedAt int64) error {
	cfg, err := s.Get(ctx, displayModel, channelID, actualModelID)
	if err != nil {
		return err
	}

	if cfg == nil {
		return nil
	}

	if healthy {
		if cfg.ConsecutiveFailures == 0 {
			return nil
		}

		_, err := cfg.Update().SetConsecutiveFailures(0).Save(ctx)

		return err
	}

	next := cfg.ConsecutiveFailures + 1
	upd := cfg.Update().SetConsecutiveFailures(next)

	threshold, err := s.lookupAutoDisableThreshold(ctx, displayModel)
	if err == nil && threshold > 0 && next >= threshold {
		upd = upd.SetProbeEnabled(false).SetAutoDisabledAt(probedAt)
	}

	_, err = upd.Save(ctx)

	return err
}

func (s *ModelProbeConfigService) lookupAutoDisableThreshold(ctx context.Context, displayModel string) (int, error) {
	m, err := s.db.Model.Query().Where(model.ModelIDEQ(displayModel)).Only(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}

	if err != nil {
		return 0, err
	}

	if m.Settings == nil {
		return 0, nil
	}

	return m.Settings.ProbeAutoDisableAfterConsecutiveFailures, nil
}

// BackfillFromSnapshots ensures every pre-existing ModelHealthSnapshot has a
// corresponding ModelProbeConfig row with probe_enabled=true. Triples the
// user already explicitly disabled (or that have any config row at all) are
// left untouched. This is what makes upgrades invisible: after deploying
// the new code, existing probe behavior continues unchanged until operators
// choose to toggle triples.
func (s *ModelProbeConfigService) BackfillFromSnapshots(ctx context.Context) error {
	existing, err := s.db.ModelProbeConfig.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("list probe configs: %w", err)
	}

	have := make(map[string]struct{}, len(existing))
	for _, c := range existing {
		have[probeConfigKey(c.DisplayModel, c.ChannelID, c.ActualModelID)] = struct{}{}
	}

	snaps, err := s.db.ModelHealthSnapshot.Query().All(ctx)
	if err != nil {
		return fmt.Errorf("list snapshots: %w", err)
	}

	for _, sn := range snaps {
		if _, ok := have[probeConfigKey(sn.DisplayModel, sn.ChannelID, sn.ActualModelID)]; ok {
			continue
		}

		_, err := s.db.ModelProbeConfig.Create().
			SetDisplayModel(sn.DisplayModel).
			SetChannelID(sn.ChannelID).
			SetActualModelID(sn.ActualModelID).
			SetProbeEnabled(true).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("backfill (%s,%d,%s): %w", sn.DisplayModel, sn.ChannelID, sn.ActualModelID, err)
		}
	}

	return nil
}
