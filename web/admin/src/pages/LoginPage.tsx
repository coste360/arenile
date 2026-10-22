import { useLogin } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import { FieldRow, ThemeToggle } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { zodResolver } from '@hookform/resolvers/zod';
import { AlertCircleIcon, UmbrellaIcon } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { OfflineIndicator } from '../components/OfflineIndicator';
import { loginSchema, type LoginValues } from './login.schema';
import { loginErrorKey } from './loginErrors';

export interface LoginPageProps {
  /** Called after a successful login. */
  onSuccess: () => void;
}

export function LoginPage({ onSuccess }: LoginPageProps) {
  const { t } = useTranslation();
  const login = useLogin();
  const [needsTotp, setNeedsTotp] = useState(false);
  const form = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { email: '', password: '', totp_code: '' },
  });

  const onSubmit = form.handleSubmit((values) => {
    login.mutate(
      {
        email: values.email,
        password: values.password,
        ...(values.totp_code ? { totp_code: values.totp_code } : {}),
      },
      {
        onSuccess,
        onError: (err) => {
          if (err.type === 'totp_required' || err.type === 'invalid_totp') setNeedsTotp(true);
        },
      },
    );
  });

  const errorKey = login.error ? loginErrorKey(login.error) : null;

  return (
    <main className="relative flex min-h-svh flex-col items-center justify-center gap-6 bg-muted/40 p-6 md:p-10">
      <div className="absolute top-4 right-4 flex items-center gap-2">
        <OfflineIndicator />
        <ThemeToggle />
      </div>
      <div className="flex w-full max-w-sm flex-col gap-6">
        <div className="flex items-center gap-2 self-center">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <UmbrellaIcon aria-hidden="true" className="size-4" />
          </span>
          <span className="text-lg font-semibold tracking-tight">{t('common.appName')}</span>
        </div>
        <Card className="shadow-lg shadow-primary/5">
          <CardHeader className="text-center">
            <CardTitle>
              <h1 className="text-xl">{t('auth.loginTitle')}</h1>
            </CardTitle>
            <CardDescription>{t('auth.loginSubtitle')}</CardDescription>
          </CardHeader>
          <CardContent>
            <Form {...form}>
              <form
                onSubmit={onSubmit}
                noValidate
                className="grid gap-5"
                aria-label={t('auth.loginTitle')}
              >
                {errorKey && (
                  <Alert variant="destructive">
                    <AlertCircleIcon aria-hidden="true" />
                    <AlertDescription>{t(errorKey)}</AlertDescription>
                  </Alert>
                )}
                <FieldRow control={form.control} name="email" label={t('auth.email')}>
                  {(field) => (
                    <Input type="email" autoComplete="username" inputMode="email" {...field} />
                  )}
                </FieldRow>
                <FieldRow control={form.control} name="password" label={t('auth.password')}>
                  {(field) => <Input type="password" autoComplete="current-password" {...field} />}
                </FieldRow>
                {needsTotp && (
                  <FieldRow
                    control={form.control}
                    name="totp_code"
                    label={t('auth.totpCode')}
                    description={t('auth.totpHint')}
                  >
                    {(field) => (
                      <Input
                        inputMode="numeric"
                        autoComplete="one-time-code"
                        maxLength={6}
                        className="tracking-[0.3em] tabular-nums"
                        {...field}
                        value={field.value ?? ''}
                      />
                    )}
                  </FieldRow>
                )}
                <Button type="submit" className="w-full" disabled={login.isPending}>
                  {login.isPending && <Spinner />}
                  {login.isPending ? t('auth.submitting') : t('auth.submit')}
                </Button>
              </form>
            </Form>
          </CardContent>
        </Card>
        <p className="text-center text-xs text-balance text-muted-foreground">
          {t('admin.brandTagline')}
        </p>
      </div>
    </main>
  );
}
