export const REQUEST_LOGS_VIEW_MODES = ['requests', 'all'] as const;

export type RequestLogsViewMode = (typeof REQUEST_LOGS_VIEW_MODES)[number];

export function isRequestLogsViewMode(value: string): value is RequestLogsViewMode {
  return REQUEST_LOGS_VIEW_MODES.includes(value as RequestLogsViewMode);
}

export function getRequestLogsViewMeta(mode: RequestLogsViewMode) {
  if (mode === 'all') {
    return {
      titleKey: 'requests.title',
      descriptionKey: 'requests.descriptionAll',
      modeLabelKey: 'requests.viewMode.all',
    };
  }

  return {
    titleKey: 'requests.title',
    descriptionKey: 'requests.description',
    modeLabelKey: 'requests.viewMode.requests',
  };
}
