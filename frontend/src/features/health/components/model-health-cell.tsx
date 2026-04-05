import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import { InteractiveTooltip } from '@/components/ui/interactive-tooltip';
import { cn } from '@/lib/utils';
import { formatHealthTimestamp } from '../channel-health-format';
import type { ModelHealthHistory, ModelHealthSnapshot } from '../data/schema';

interface ModelHealthCellProps {
  snapshot: ModelHealthSnapshot;
  history?: ModelHealthHistory[];
  locale: string;
}

export function getModelHealthPointDetails(point: ModelHealthHistory, locale: string, t: (key: string) => string) {
  return {
    timestamp: formatHealthTimestamp(point.probedAt, locale),
    status: point.isHealthy ? t('models.healthPage.healthy') : t('models.healthPage.unhealthy'),
    source: point.manualOverride ? t('models.healthPage.manualTag') : t('models.healthPage.autoTag'),
    actualModelID: point.actualModelID,
  };
}

export const ModelHealthCell = memo(({ snapshot, history = [], locale }: ModelHealthCellProps) => {
  const { t } = useTranslation();
  const displayPoints = history.slice(0, 15).reverse();

  if (displayPoints.length === 0) {
    return <span className={cn('h-8 w-1.5 rounded-sm', snapshot.isHealthy ? 'bg-green-500' : 'bg-red-500')} />;
  }

  return (
    <div className='flex items-center gap-0.5'>
      {displayPoints.map((point) => (
        <InteractiveTooltip
          key={point.id ?? `${point.channelID}-${point.actualModelID}-${point.probedAt}`}
          content={(() => {
            const details = getModelHealthPointDetails(point, locale, t);
            return (
              <div className='space-y-1 text-xs'>
                <div>{details.timestamp}</div>
                <div>{details.status}</div>
                <div>{details.source}</div>
                <div className='max-w-72 break-all'>{details.actualModelID}</div>
              </div>
            );
          })()}
        >
          <div className={cn('h-8 w-1.5 rounded-sm', point.isHealthy ? 'bg-green-500' : 'bg-red-500')} />
        </InteractiveTooltip>
      ))}
    </div>
  );
});

ModelHealthCell.displayName = 'ModelHealthCell';
