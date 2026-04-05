import { VisibilityState } from '@tanstack/react-table';

export const DEFAULT_COLUMN_VISIBILITY: VisibilityState = {
  baseURL: false,
  supportedModels: false,
  proxy: false,
  tags: false,
  channelPerformance: false,
};

export function getInitialColumnVisibility(stored: string | null): VisibilityState {
  if (!stored) {
    return DEFAULT_COLUMN_VISIBILITY;
  }

  try {
    return { ...DEFAULT_COLUMN_VISIBILITY, ...JSON.parse(stored) };
  } catch {
    return DEFAULT_COLUMN_VISIBILITY;
  }
}
