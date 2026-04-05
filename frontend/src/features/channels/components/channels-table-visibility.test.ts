import { describe, expect, it } from 'vitest';
import { DEFAULT_COLUMN_VISIBILITY, getInitialColumnVisibility } from './channels-table-visibility';

describe('channels table visibility defaults', () => {
  it('returns defaults when storage is empty', () => {
    expect(getInitialColumnVisibility(null)).toEqual(DEFAULT_COLUMN_VISIBILITY);
  });

  it('merges stored visibility over defaults', () => {
    expect(getInitialColumnVisibility(JSON.stringify({ health: false, proxy: true }))).toEqual({
      ...DEFAULT_COLUMN_VISIBILITY,
      health: false,
      proxy: true,
    });
  });

  it('falls back to defaults when storage is invalid', () => {
    expect(getInitialColumnVisibility('{')).toEqual(DEFAULT_COLUMN_VISIBILITY);
  });
});
