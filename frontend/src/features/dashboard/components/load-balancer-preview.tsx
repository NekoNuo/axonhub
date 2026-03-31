import { useState } from 'react';
import { ChevronDownIcon, RefreshCcwIcon, RouteIcon, SigmaIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useLoadBalancerPreview } from '../data/dashboard';

export function LoadBalancerPreview() {
  const { t } = useTranslation();
  const { data, isLoading, error } = useLoadBalancerPreview();
  const [openStrategies, setOpenStrategies] = useState<Record<string, boolean>>({});

  if (isLoading) {
    return (
      <div className='grid gap-4 lg:grid-cols-3'>
        <Skeleton className='h-[220px]' />
        <Skeleton className='h-[220px]' />
        <Skeleton className='h-[220px]' />
      </div>
    );
  }

  if (error) {
    return (
      <div className='text-sm text-red-500'>
        {t('dashboard.preview.error')} {error.message}
      </div>
    );
  }

  if (!data) {
    return <div className='text-muted-foreground text-sm'>{t('dashboard.preview.empty')}</div>;
  }

  return (
    <div className='space-y-4'>
      <Card className='hover-card'>
        <CardHeader>
          <div className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
            <div>
              <CardTitle>{t('dashboard.preview.title')}</CardTitle>
              <CardDescription>{t('dashboard.preview.description')}</CardDescription>
            </div>
            <div className='text-muted-foreground flex flex-wrap items-center gap-3 text-sm'>
              <span>
                {t('dashboard.preview.model')}: <span className='text-foreground font-medium'>{data.modelId}</span>
              </span>
              <span>
                {t('dashboard.preview.strategy')}: <span className='text-foreground font-medium'>{data.activeStrategy}</span>
              </span>
              <span className='flex items-center gap-1'>
                <RefreshCcwIcon className='h-3.5 w-3.5' />
                {t('dashboard.preview.autoRefresh')}
              </span>
            </div>
          </div>
        </CardHeader>
      </Card>

      <div className='grid gap-4 xl:grid-cols-2'>
        {data.strategies.map((strategy) => {
          const isOpen = openStrategies[strategy.strategy] ?? strategy.strategy === data.activeStrategy;

          return (
            <Card key={strategy.strategy} className='hover-card'>
              <CardHeader>
                <button
                  type='button'
                  onClick={() => setOpenStrategies((prev) => ({ ...prev, [strategy.strategy]: !isOpen }))}
                  className='flex w-full items-center justify-between text-left'
                >
                  <div className='space-y-1'>
                    <CardTitle className='flex items-center gap-2'>
                      {strategy.strategy}
                      {strategy.strategy === data.activeStrategy && (
                        <span className='bg-primary/10 text-primary rounded px-2 py-0.5 text-xs'>{t('dashboard.preview.active')}</span>
                      )}
                    </CardTitle>
                    <CardDescription>
                      {t('dashboard.preview.primary')}: {strategy.summary.primaryChannelName || '-'}
                    </CardDescription>
                  </div>
                  <ChevronDownIcon className={`h-4 w-4 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
                </button>
              </CardHeader>
              {isOpen && (
                <CardContent className='space-y-4'>
                  <div className='grid gap-3 md:grid-cols-3'>
                    <PreviewRow label={t('dashboard.preview.primary')} value={strategy.summary.primaryChannelName || '-'} />
                    <PreviewRow label={t('dashboard.preview.firstRetry')} value={strategy.summary.firstRetryChannelName || '-'} />
                    <PreviewRow label={t('dashboard.preview.fallback')} value={strategy.summary.fallbackChannelName || '-'} />
                  </div>

                  <div className='space-y-3'>
                    <div className='text-sm font-medium'>{t('dashboard.preview.timeline')}</div>
                    {strategy.steps.map((step) => (
                      <div
                        key={`${strategy.strategy}-${step.attempt}-${step.channelName}`}
                        className='flex items-start gap-3 rounded-lg border p-3'
                      >
                        <div className='bg-primary/10 flex h-9 w-9 items-center justify-center rounded-md'>
                          <RouteIcon className='text-primary h-4 w-4' />
                        </div>
                        <div className='min-w-0 flex-1'>
                          <div className='flex items-center justify-between gap-3'>
                            <div className='font-medium'>{t('dashboard.preview.attempt', { attempt: step.attempt })}</div>
                            <div className='text-muted-foreground text-xs'>{step.channelName}</div>
                          </div>
                          <div className='text-muted-foreground mt-1 text-sm'>{step.reason}</div>
                          <div className='text-muted-foreground mt-1 text-xs'>
                            {step.waitMsAfterFailure > 0
                              ? t('dashboard.preview.retryAfter', { ms: step.waitMsAfterFailure })
                              : t('dashboard.preview.endOfChain')}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>

                  <div className='space-y-3'>
                    <div className='text-sm font-medium'>{t('dashboard.preview.ranking')}</div>
                    {strategy.candidates.map((candidate, index) => (
                      <div key={`${strategy.strategy}-${candidate.channelName}-${index}`} className='rounded-lg border p-3'>
                        <div className='flex items-center justify-between gap-3'>
                          <div>
                            <div className='flex items-center gap-3'>
                              <div className='bg-primary/10 text-primary flex h-8 w-8 items-center justify-center rounded-md text-sm font-semibold'>
                                {index + 1}
                              </div>
                              <div className='font-medium'>{candidate.channelName}</div>
                            </div>
                            <div className='text-muted-foreground mt-2 text-sm'>{candidate.reason}</div>
                          </div>
                          <div className='flex items-center gap-2 text-sm font-medium'>
                            <SigmaIcon className='text-muted-foreground h-4 w-4' />
                            {candidate.totalScore.toFixed(2)}
                          </div>
                        </div>
                        <div className='mt-3 grid gap-2'>
                          <div className='grid gap-2 md:grid-cols-3'>
                            <MetricPill
                              label={t('dashboard.preview.healthStatus')}
                              value={<StatusBadge status={candidate.healthStatus} />}
                            />
                            <MetricPill
                              label={t('dashboard.preview.latency')}
                              value={candidate.latencyMs != null ? `${candidate.latencyMs.toFixed(0)}ms` : '-'}
                            />
                            <MetricPill label={t('dashboard.preview.recentFailures')} value={String(candidate.recentFailures)} />
                          </div>
                          {candidate.scoreBreakdown.map((score) => (
                            <div
                              key={`${candidate.channelName}-${score.strategyName}`}
                              className='bg-muted/40 flex items-start justify-between rounded-md px-3 py-2 text-sm'
                            >
                              <div className='min-w-0 pr-3'>
                                <div className='font-medium'>{score.strategyName}</div>
                                <div className='text-muted-foreground text-xs'>{score.reason || t('dashboard.preview.noReason')}</div>
                              </div>
                              <div className='font-mono text-xs'>{score.score.toFixed(2)}</div>
                            </div>
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                </CardContent>
              )}
            </Card>
          );
        })}
      </div>
    </div>
  );
}

function PreviewRow({ label, value }: { label: string; value: string }) {
  return (
    <div className='flex items-center justify-between gap-3 rounded-lg border px-3 py-2'>
      <span className='text-muted-foreground'>{label}</span>
      <span className='font-medium'>{value}</span>
    </div>
  );
}

function MetricPill({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className='bg-muted/40 rounded-md px-3 py-2 text-sm'>
      <div className='text-muted-foreground text-xs'>{label}</div>
      <div className='font-medium'>{value}</div>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const colorClass =
    status === 'healthy' || status === 'observed-healthy'
      ? 'bg-emerald-500/12 text-emerald-700'
      : status === 'degraded'
        ? 'bg-amber-500/12 text-amber-700'
        : status === 'unhealthy' || status === 'observed-unhealthy'
          ? 'bg-rose-500/12 text-rose-700'
          : 'bg-muted text-muted-foreground';

  return <span className={`inline-flex rounded px-2 py-0.5 text-xs font-medium ${colorClass}`}>{status}</span>;
}
