import { describe, expect, it } from 'vitest';
import { getModelHealthPointDetails } from './model-health-cell';

describe('ModelHealthCell', () => {
  const t = (key: string) => {
    switch (key) {
      case 'models.healthPage.healthy':
        return 'Healthy';
      case 'models.healthPage.unhealthy':
        return 'Unhealthy';
      case 'models.healthPage.manualTag':
        return 'Manual';
      case 'models.healthPage.autoTag':
        return 'Auto';
      default:
        return key;
    }
  };

  it('builds detailed tooltip content from history points', () => {
    const details = getModelHealthPointDetails(
      {
        displayModel: 'gpt-5.4',
        channelID: 'Q2hhbm5lbDox',
        actualModelID: 'gpt-5.4-2025-02-15',
        isHealthy: false,
        manualOverride: true,
        probedAt: 1712310300,
      },
      'zh-CN',
      t
    );

    expect(details.timestamp).toContain('17:45:00');
    expect(details.status).toBe('Unhealthy');
    expect(details.source).toBe('Manual');
    expect(details.actualModelID).toBe('gpt-5.4-2025-02-15');
  });
});
