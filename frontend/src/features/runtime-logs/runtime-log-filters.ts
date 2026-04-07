import { buildDateRangeWhereClause, type DateTimeRangeValue } from '@/utils/date-range';

export const DEFAULT_RUNTIME_LOG_LEVELS = ['warn', 'error'] as const;
export const RUNTIME_LOG_LEVELS_WITH_INFO = ['info', ...DEFAULT_RUNTIME_LOG_LEVELS] as const;

export function getRuntimeLogLevels(includeInfo: boolean) {
  return includeInfo ? [...RUNTIME_LOG_LEVELS_WITH_INFO] : [...DEFAULT_RUNTIME_LOG_LEVELS];
}

export function buildRuntimeLogWhereClause(dateRange: DateTimeRangeValue | undefined, includeInfo: boolean) {
  return {
    ...buildDateRangeWhereClause(dateRange),
    levelIn: getRuntimeLogLevels(includeInfo),
  };
}
