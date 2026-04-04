import { describe, expect, it } from 'vitest';
import { buildModelSettingsInput, getModelProbeBadgeVariant, getModelProbeSwitchState } from './model-probe-settings';

describe('Task 9 Model Probe Settings UI', () => {
  it('includes global model-probe toggle in settings mutation payload', () => {
    expect(
      buildModelSettingsInput({
        enableModelProbe: true,
        fallbackToChannelsOnModelNotFound: false,
        queryAllChannelModels: true,
      })
    ).toEqual({
      enableModelProbe: true,
      fallbackToChannelsOnModelNotFound: false,
      queryAllChannelModels: true,
    });
  });

  it('derives per-model probe-enabled switch state', () => {
    expect(getModelProbeSwitchState({ probeEnabled: true })).toBe(true);
    expect(getModelProbeSwitchState({ probeEnabled: false })).toBe(false);
    expect(getModelProbeSwitchState({})).toBe(false);
  });

  it('maps per-model probe-enabled state to display variant', () => {
    expect(getModelProbeBadgeVariant(true)).toBe('default');
    expect(getModelProbeBadgeVariant(false)).toBe('secondary');
  });
});
