import { Outlet, createFileRoute } from '@tanstack/react-router';

function HealthRouteLayout() {
  return <Outlet />;
}

export const Route = createFileRoute('/_authenticated/health')({
  component: HealthRouteLayout,
});
