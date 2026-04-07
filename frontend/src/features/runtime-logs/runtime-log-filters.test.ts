import { describe, expect, it } from 'vitest';
import { buildRuntimeLogWhereClause, DEFAULT_RUNTIME_LOG_LEVELS, getRuntimeLogLevels, RUNTIME_LOG_LEVELS_WITH_INFO } from './runtime-log-filters';

describe('runtime log filters', () => {
  it('defaults to warn and error levels', () => {
    expect(DEFAULT_RUNTIME_LOG_LEVELS).toEqual(['warn', 'error']);
    expect(getRuntimeLogLevels(false)).toEqual(['warn', 'error']);
  });

  it('adds info level when requested', () => {
    expect(RUNTIME_LOG_LEVELS_WITH_INFO).toEqual(['info', 'warn', 'error']);
    expect(getRuntimeLogLevels(true)).toEqual(['info', 'warn', 'error']);
  });

  it('builds a where clause with shared date range and level filters', () => {
    const where = buildRuntimeLogWhereClause(
      {
        from: new Date('2026-04-06T00:00:00.000Z'),
        to: new Date('2026-04-07T00:00:00.000Z'),
        startTime: { hh: '08', mm: '00', ss: '00' },
        endTime: { hh: '18', mm: '30', ss: '00' },
      },
      true
    );

    expect(where).toEqual({
      createdAtGTE: '2026-04-06T00:00:00.000Z',
      createdAtLTE: '2026-04-07T10:30:00.999Z',
      levelIn: ['info', 'warn', 'error'],
    });
  });
});
