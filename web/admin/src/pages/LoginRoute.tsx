import { getRouteApi, useNavigate } from '@tanstack/react-router';
import { LoginPage } from './LoginPage';

const route = getRouteApi('/login');

/** Login page bound to the router: returns to the requested page on success. */
export function LoginRoute() {
  const navigate = useNavigate();
  const { redirect: target } = route.useSearch();
  return (
    <LoginPage
      onSuccess={() => {
        void navigate({ to: target ?? '/oggi' });
      }}
    />
  );
}
