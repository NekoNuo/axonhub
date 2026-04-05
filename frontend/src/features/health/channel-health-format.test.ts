import { describe, expect, it } from 'vitest';
import { formatHealthTimestamp } from './channel-health-format';

describe('channel health timestamp formatting', () => {
  it('formats unix seconds into readable local time', () => {
    expect(formatHealthTimestamp(1775350800, 'en-US')).toMatch(/2026/);
  });

  it('returns dash for missing timestamps', () => {
    expect(formatHealthTimestamp(0, 'en-US')).toBe('-');
  });
});
