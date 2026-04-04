import { createFileRoute } from '@tanstack/react-router';
import ChannelHealthPage from '@/features/health/channel-health-page';

export const Route = createFileRoute('/_authenticated/health/channels')({
  component: ChannelHealthPage,
});
