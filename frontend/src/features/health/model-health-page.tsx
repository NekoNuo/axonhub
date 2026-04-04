import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Main } from '@/components/layout/main';
import { manualModelProbe, fetchModelHealthSnapshots } from './data/health';
import type { ManualModelProbeInput, ModelHealthSnapshot } from './data/schema';
import { ModelHealthCell } from './components/model-health-cell';

export interface ModelHealthGroup {
  displayModel: string;
  rows: ModelHealthSnapshot[];
}

export function buildModelHealthGroups(rows: ModelHealthSnapshot[]): ModelHealthGroup[] {
  const groups = new Map<string, ModelHealthSnapshot[]>();

  rows.forEach((row) => {
    if (!groups.has(row.displayModel)) {
      groups.set(row.displayModel, []);
    }
    groups.get(row.displayModel)!.push(row);
  });

  return Array.from(groups.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([displayModel, items]) => ({
      displayModel,
      rows: items.sort((a, b) => a.actualModelID.localeCompare(b.actualModelID) || a.channelID.localeCompare(b.channelID)),
    }));
}

export function applyManualProbeResult(rows: ModelHealthSnapshot[], input: ManualModelProbeInput): ModelHealthSnapshot[] {
  return rows.map((row) =>
    row.displayModel === input.displayModel && row.actualModelID === input.actualModelID && row.channelID === input.channelID
      ? {
          ...row,
          isHealthy: true,
          manualOverride: true,
          probedAt: Math.max(row.probedAt, Math.floor(Date.now() / 1000)),
        }
      : row
  );
}

export function ModelHealthPage() {
  const { t } = useTranslation();
  const [snapshots, setSnapshots] = useState<ModelHealthSnapshot[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [isProbingKey, setIsProbingKey] = useState<string | null>(null);

  useState(() => {
    void (async () => {
      setIsLoading(true);
      try {
        const data = await fetchModelHealthSnapshots({ input: {} });
        setSnapshots(data);
      } finally {
        setIsLoading(false);
      }
    })();
  });

  const groups = useMemo(() => buildModelHealthGroups(snapshots), [snapshots]);

  const handleManualProbe = async (input: ManualModelProbeInput) => {
    const key = `${input.displayModel}:${input.actualModelID}:${input.channelID}`;
    setIsProbingKey(key);
    try {
      await manualModelProbe(input);
      setSnapshots((current) => applyManualProbeResult(current, input));
    } finally {
      setIsProbingKey(null);
    }
  };

  return (
    <Main>
      <div className='space-y-6'>
        <div>
          <h1 className='text-2xl font-semibold'>{t('models.healthPage.title')}</h1>
          <p className='text-muted-foreground text-sm'>{t('models.healthPage.description')}</p>
        </div>

        {isLoading ? <div className='text-muted-foreground text-sm'>{t('models.healthPage.loading')}</div> : null}

        <div className='space-y-4'>
          {groups.map((group) => (
            <section key={group.displayModel} className='rounded-xl border p-4'>
              <h2 className='mb-3 text-lg font-medium'>{group.displayModel}</h2>
              <div className='space-y-3'>
                {group.rows.map((row) => {
                  const key = `${row.displayModel}:${row.actualModelID}:${row.channelID}`;
                  return (
                    <div key={key} className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                      <div className='space-y-1'>
                        <div className='font-medium'>{row.actualModelID}</div>
                        <div className='text-muted-foreground text-xs'>{row.channelID}</div>
                      </div>
                      <div className='flex items-center gap-3'>
                        <ModelHealthCell snapshot={row} />
                        <Button size='sm' variant='outline' onClick={() => handleManualProbe(row)} disabled={isProbingKey === key}>
                          {isProbingKey === key ? t('models.healthPage.probing') : t('models.healthPage.manualProbe')}
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
            </section>
          ))}
        </div>
      </div>
    </Main>
  );
}

export default ModelHealthPage;
