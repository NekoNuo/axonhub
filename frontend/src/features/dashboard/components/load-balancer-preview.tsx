import { GitBranchIcon, RefreshCcwIcon, RouteIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { useLoadBalancerPreview } from '../data/dashboard';

export function LoadBalancerPreview() {
  const { t } = useTranslation();
  const { data, isLoading, error } = useLoadBalancerPreview();

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
                {t('dashboard.preview.strategy')}: <span className='text-foreground font-medium'>{data.strategy}</span>
              </span>
              <span className='flex items-center gap-1'>
                <RefreshCcwIcon className='h-3.5 w-3.5' />
                {t('dashboard.preview.autoRefresh')}
              </span>
            </div>
          </div>
        </CardHeader>
      </Card>

      <div className='grid gap-4 lg:grid-cols-3'>
        <Card className='hover-card'>
          <CardHeader>
            <CardTitle>{t('dashboard.preview.current')}</CardTitle>
          </CardHeader>
          <CardContent className='space-y-3 text-sm'>
            <PreviewRow label={t('dashboard.preview.primary')} value={data.summary.primaryChannelName || '-'} />
            <PreviewRow label={t('dashboard.preview.firstRetry')} value={data.summary.firstRetryChannelName || '-'} />
            <PreviewRow label={t('dashboard.preview.fallback')} value={data.summary.fallbackChannelName || '-'} />
          </CardContent>
        </Card>

        <Card className='hover-card lg:col-span-2'>
          <CardHeader>
            <CardTitle>{t('dashboard.preview.ranking')}</CardTitle>
            <CardDescription>{t('dashboard.preview.rankingDescription')}</CardDescription>
          </CardHeader>
          <CardContent className='space-y-3'>
            {data.candidates.map((candidate, index) => (
              <div key={`${candidate.channelName}-${index}`} className='flex items-center justify-between rounded-lg border px-3 py-2'>
                <div className='flex items-center gap-3'>
                  <div className='bg-primary/10 text-primary flex h-8 w-8 items-center justify-center rounded-md text-sm font-semibold'>
                    {index + 1}
                  </div>
                  <div className='font-medium'>{candidate.channelName}</div>
                </div>
                <GitBranchIcon className='text-muted-foreground h-4 w-4' />
              </div>
            ))}
          </CardContent>
        </Card>
      </div>

      <Card className='hover-card'>
        <CardHeader>
          <CardTitle>{t('dashboard.preview.timeline')}</CardTitle>
          <CardDescription>{t('dashboard.preview.timelineDescription')}</CardDescription>
        </CardHeader>
        <CardContent className='space-y-3'>
          {data.steps.map((step) => (
            <div key={`${step.attempt}-${step.channelName}`} className='flex items-start gap-3 rounded-lg border p-3'>
              <div className='bg-primary/10 flex h-9 w-9 items-center justify-center rounded-md'>
                <RouteIcon className='text-primary h-4 w-4' />
              </div>
              <div className='min-w-0 flex-1'>
                <div className='flex items-center justify-between gap-3'>
                  <div className='font-medium'>{t('dashboard.preview.attempt', { attempt: step.attempt })}</div>
                  <div className='text-muted-foreground text-xs'>{step.channelName}</div>
                </div>
                <div className='text-muted-foreground mt-1 text-sm'>
                  {step.waitMsAfterFailure > 0
                    ? t('dashboard.preview.retryAfter', { ms: step.waitMsAfterFailure })
                    : t('dashboard.preview.endOfChain')}
                </div>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>
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
