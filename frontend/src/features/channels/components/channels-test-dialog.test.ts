import { describe, expect, it } from 'vitest';
import { buildBatchTestSuccessMessage, buildSingleModelTestSuccessMessage } from './channels-test-dialog';

describe('ChannelsTestDialog', () => {
  it('builds single model success message', () => {
    const t = (key: string, vars?: Record<string, unknown>) => {
      if (key === 'channels.dialogs.test.singleSuccess') {
        return `模型 ${vars?.model} 测试成功（耗时 ${vars?.latency} 秒）`;
      }
      return key;
    };

    expect(buildSingleModelTestSuccessMessage(t, 'gpt-5.4', 1.23)).toBe('模型 gpt-5.4 测试成功（耗时 1.23 秒）');
  });

  it('builds batch success message', () => {
    const t = (key: string, vars?: Record<string, unknown>) => {
      if (key === 'channels.dialogs.test.batchSuccess') {
        return `已完成 ${vars?.count} 个模型测试，成功 ${vars?.success} 个，失败 ${vars?.failed} 个`;
      }
      return key;
    };

    expect(buildBatchTestSuccessMessage(t, 5, 3, 2)).toBe('已完成 5 个模型测试，成功 3 个，失败 2 个');
  });
});
