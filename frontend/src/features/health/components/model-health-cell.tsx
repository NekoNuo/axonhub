import { memo } from 'react';
import { format } from 'date-fns';
import { useTranslation } from 'react-i18next';
import { InteractiveTooltip } from '@/components/ui/interactive-tooltip';
import { cn } from '@/lib/utils';
import type { ModelHealthHistory, ModelHealthSnapshot } from '../data/schema';

interface ModelHealthCellProps {
  snapshot: ModelHealthSnapshot;
  history?: ModelHealthHistory[];
}

export const ModelHealthCell = memo(({ snapshot, history = [] }: ModelHealthCellProps) => {
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
          content={
            <div className='space-y-1 text-xs'>
              <div>{format(new Date(point.probedAt * 1000), 'MM-dd HH:mm')}</div>
              <div>{point.isHealthy ? 'Healthy' : 'Unhealthy'}</div>
              <div>{point.manualOverride ? t('models.healthPage.manualTag') : t('models.healthPage.autoTag')}</div>
            </div>
          }
        >
          <div className={cn('h-8 w-1.5 rounded-sm', point.isHealthy ? 'bg-green-500' : 'bg-red-500')} />
        </InteractiveTooltip>
      ))}
    </div>
  );
});

ModelHealthCell.displayName = 'ModelHealthCell';
