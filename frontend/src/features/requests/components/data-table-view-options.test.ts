import { describe, expect, it } from 'vitest';
import { getRequestsColumnLabelKey } from './data-table-view-options';

describe('getRequestsColumnLabelKey', () => {
  it('maps request path column to endpoint label', () => {
    expect(getRequestsColumnLabelKey('requestPath')).toBe('requests.columns.endpoint');
  });

  it('falls back to matching request column key for regular columns', () => {
    expect(getRequestsColumnLabelKey('userAgent')).toBe('requests.columns.userAgent');
  });
});
