import type { UpdateModelSettingsInput } from '@/features/system/data/system';

export interface ModelProbeSettingsState {
  enableModelProbe: boolean;
  fallbackToChannelsOnModelNotFound: boolean;
  queryAllChannelModels: boolean;
}

export interface ModelProbeSwitchSettings {
  probeEnabled?: boolean;
}

export function buildModelSettingsInput(state: ModelProbeSettingsState): UpdateModelSettingsInput {
  return {
    enableModelProbe: state.enableModelProbe,
    fallbackToChannelsOnModelNotFound: state.fallbackToChannelsOnModelNotFound,
    queryAllChannelModels: state.queryAllChannelModels,
  };
}

export function getModelProbeSwitchState(settings?: ModelProbeSwitchSettings | null): boolean {
  return settings?.probeEnabled ?? false;
}

export function getModelProbeBadgeVariant(probeEnabled: boolean): 'default' | 'secondary' {
  return probeEnabled ? 'default' : 'secondary';
}
