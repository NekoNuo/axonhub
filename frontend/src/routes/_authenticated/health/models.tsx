import { createFileRoute } from '@tanstack/react-router';

function ModelHealthRoutePage() {
  return <div>Model health page</div>;
}

export const Route = createFileRoute('/_authenticated/health/models')({
  component: ModelHealthRoutePage,
});
