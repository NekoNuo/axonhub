import { memo } from 'react';
import { Badge } from '@/components/ui/badge';
import type { ModelHealthSnapshot } from '../data/schema';

interface ModelHealthCellProps {
  snapshot: ModelHealthSnapshot;
}

export const ModelHealthCell = memo(({ snapshot }: ModelHealthCellProps) => {
  return (
    <div className='flex items-center gap-2'>
      <span className={`h-8 w-1.5 rounded-sm ${snapshot.isHealthy ? 'bg-green-500' : 'bg-red-500'}`} />
      <Badge variant={snapshot.isHealthy ? 'default' : 'destructive'}>
        {snapshot.isHealthy ? 'Healthy' : 'Unhealthy'}
      </Badge>
      {snapshot.manualOverride ? <Badge variant='outline'>Manual</Badge> : null}
    </div>
  );
});

ModelHealthCell.displayName = 'ModelHealthCell';
