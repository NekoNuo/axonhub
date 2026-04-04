import { createFileRoute } from '@tanstack/react-router';
import ModelHealthPage from '@/features/health/model-health-page';

export const Route = createFileRoute('/_authenticated/health/models')({
  component: ModelHealthPage,
});
