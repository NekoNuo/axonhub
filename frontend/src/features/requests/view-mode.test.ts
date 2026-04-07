import { describe, expect, it } from 'vitest';
import { getRequestLogsViewMeta, REQUEST_LOGS_VIEW_MODES } from './view-mode';

describe('request logs view mode', () => {
  it('defines request-only and all-related-log modes', () => {
    expect(REQUEST_LOGS_VIEW_MODES).toEqual(['requests', 'all']);
  });

  it('returns localized metadata for request-only mode', () => {
    expect(getRequestLogsViewMeta('requests')).toEqual({
      titleKey: 'requests.title',
      descriptionKey: 'requests.description',
      modeLabelKey: 'requests.viewMode.requests',
    });
  });

  it('returns localized metadata for all-related-logs mode', () => {
    expect(getRequestLogsViewMeta('all')).toEqual({
      titleKey: 'requests.title',
      descriptionKey: 'requests.descriptionAll',
      modeLabelKey: 'requests.viewMode.all',
    });
  });
});
