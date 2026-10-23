import { ApiProblem } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import {
  BrandLogo,
  FieldRow,
  MadeByCoste360,
  QueryState,
  ThemeToggle,
} from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { zodResolver } from '@hookform/resolvers/zod';
import { Link, useLocation, useNavigate } from '@tanstack/react-router';
import { AlertCircleIcon, CheckCircle2Icon } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { useForm, type Path } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useDocumentTitle } from '@arenile/shared/hooks/use-document-title';
import { useCompleteFirstRun, useFirstRunStatus } from './api';
import { firstRunSchema, tokenFromHash, type FirstRunValues } from './firstRun.schema';

const FORM_FIELDS = ['organization_name', 'name', 'email', 'password'] as const;

/** Shell of the page: same look as the login page, outside the app shell. */
function FirstRunShell({ children }: { children: ReactNode }) {
  return (
    <main className="relative flex min-h-svh flex-col items-center justify-center gap-6 bg-muted/40 p-6 md:p-10">
      <div className="absolute top-4 right-4">
        <ThemeToggle />
      </div>
      <div className="flex w-full max-w-md flex-col gap-6">
        <BrandLogo className="h-20 self-center" />
        {children}
        <MadeByCoste360 className="self-center" />
      </div>
    </main>
  );
}

/**
 * First start of the desktop edition (spec 29.1, "Primo avvio"): the
 * launcher opens /admin/first-run#token=<one-time token>. The token is read
 * from the fragment (never sent in a URL, never logged) and removed from
 * the address bar; the form creates the owner and the organization, then
 * the browser is logged in and continues to the guided setup (spec 15.2).
 */
export function FirstRunPage() {
  const { t } = useTranslation('firstRun');
  // The page title already names the product ("Benvenuto in Arenile"): the tab shows just "Arenile".
  useDocumentTitle();
  const location = useLocation();
  const navigate = useNavigate();
  const [token] = useState(() => tokenFromHash(location.hash));
  const status = useFirstRunStatus();
  const complete = useCompleteFirstRun();
  const form = useForm<FirstRunValues>({
    resolver: zodResolver(firstRunSchema),
    defaultValues: {
      organization_name: '',
      name: '',
      email: '',
      password: '',
      password_confirm: '',
    },
  });

  // Drop the token from the address bar (history, bookmarks, screenshots).
  useEffect(() => {
    if (location.hash) void navigate({ to: '/first-run', replace: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- once, on mount
  }, []);

  const onSubmit = form.handleSubmit((values) => {
    complete.mutate(
      {
        setup_token: token,
        organization_name: values.organization_name,
        name: values.name,
        email: values.email,
        password: values.password,
      },
      {
        onSuccess: () => void navigate({ to: '/establishments/new' }),
        onError: (err) => {
          for (const fe of err.fieldErrors) {
            const field = FORM_FIELDS.find((f) => f === fe.field);
            if (field)
              form.setError(field as Path<FirstRunValues>, { type: 'server', message: fe.message });
          }
        },
      },
    );
  });

  if (!status.data) {
    return (
      <FirstRunShell>
        <QueryState
          isLoading={status.isPending}
          error={status.error}
          onRetry={() => void status.refetch()}
        />
      </FirstRunShell>
    );
  }

  if (!status.data.required) {
    return (
      <FirstRunShell>
        <Alert>
          <CheckCircle2Icon aria-hidden="true" />
          <AlertTitle>{t('done.title')}</AlertTitle>
          <AlertDescription>
            <p>{t('done.description')}</p>
            <Button asChild variant="link" className="h-auto px-0">
              <Link to="/login">{t('done.login')}</Link>
            </Button>
          </AlertDescription>
        </Alert>
      </FirstRunShell>
    );
  }

  const error = complete.error;
  const alertKey = !token
    ? 'errors.noToken'
    : error instanceof ApiProblem
      ? error.type === 'setup_token_invalid'
        ? 'errors.tokenInvalid'
        : error.type === 'setup_completed'
          ? 'errors.completed'
          : error.type === 'validation_failed'
            ? null
            : error.isNetworkError
              ? 'errors.network'
              : 'errors.generic'
      : null;

  return (
    <FirstRunShell>
      <Card className="shadow-lg shadow-primary/5">
        <CardHeader className="text-center">
          <CardTitle>
            <h1 className="text-xl">{t('title')}</h1>
          </CardTitle>
          <CardDescription>{t('description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={onSubmit} noValidate className="grid gap-5" aria-label={t('title')}>
              {alertKey && (
                <Alert variant="destructive">
                  <AlertCircleIcon aria-hidden="true" />
                  <AlertDescription>{t(alertKey)}</AlertDescription>
                </Alert>
              )}
              <FieldRow
                control={form.control}
                name="organization_name"
                label={t('fields.organization')}
                description={t('fields.organizationHint')}
              >
                {(field) => <Input autoComplete="organization" {...field} />}
              </FieldRow>
              <FieldRow control={form.control} name="name" label={t('fields.name')}>
                {(field) => <Input autoComplete="name" {...field} />}
              </FieldRow>
              <FieldRow
                control={form.control}
                name="email"
                label={t('fields.email')}
                description={t('fields.emailHint')}
              >
                {(field) => (
                  <Input type="email" autoComplete="username" inputMode="email" {...field} />
                )}
              </FieldRow>
              <FieldRow
                control={form.control}
                name="password"
                label={t('fields.password')}
                description={t('fields.passwordHint')}
              >
                {(field) => <Input type="password" autoComplete="new-password" {...field} />}
              </FieldRow>
              <FieldRow
                control={form.control}
                name="password_confirm"
                label={t('fields.passwordConfirm')}
              >
                {(field) => <Input type="password" autoComplete="new-password" {...field} />}
              </FieldRow>
              <Button type="submit" className="w-full" disabled={!token || complete.isPending}>
                {complete.isPending && <Spinner />}
                {complete.isPending ? t('submitting') : t('submit')}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>
      <p className="text-center text-xs text-balance text-muted-foreground">{t('privacy')}</p>
    </FirstRunShell>
  );
}
