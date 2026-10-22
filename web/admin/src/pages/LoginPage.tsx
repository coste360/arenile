import { useLogin } from '@arenile/shared/api';
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  Input,
  Label,
} from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
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
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { email: '', password: '', totp_code: '' },
  });

  const onSubmit = handleSubmit((values) => {
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
    <main className="flex min-h-screen items-center justify-center bg-slate-50 px-4 py-10">
      <div className="w-full max-w-sm space-y-4">
        <div className="flex justify-end">
          <OfflineIndicator />
        </div>
        <Card>
          <CardHeader>
            <p className="text-sm font-semibold tracking-wide text-[var(--ar-primary-on-surface)]">
              {t('common.appName')}
            </p>
            <h1 className="text-lg font-semibold leading-tight">{t('auth.loginTitle')}</h1>
            <CardDescription>{t('auth.loginSubtitle')}</CardDescription>
          </CardHeader>
          <CardContent>
            <form
              onSubmit={onSubmit}
              noValidate
              className="space-y-4"
              aria-label={t('auth.loginTitle')}
            >
              {errorKey && (
                <div
                  role="alert"
                  className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800"
                >
                  {t(errorKey)}
                </div>
              )}
              <div className="space-y-1.5">
                <Label htmlFor="email">{t('auth.email')}</Label>
                <Input
                  id="email"
                  type="email"
                  autoComplete="username"
                  inputMode="email"
                  aria-invalid={errors.email ? true : undefined}
                  aria-describedby={errors.email ? 'email-error' : undefined}
                  {...register('email')}
                />
                {errors.email?.message && (
                  <p id="email-error" className="text-sm text-red-700">
                    {t(errors.email.message)}
                  </p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="password">{t('auth.password')}</Label>
                <Input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  aria-invalid={errors.password ? true : undefined}
                  aria-describedby={errors.password ? 'password-error' : undefined}
                  {...register('password')}
                />
                {errors.password?.message && (
                  <p id="password-error" className="text-sm text-red-700">
                    {t(errors.password.message)}
                  </p>
                )}
              </div>
              {needsTotp && (
                <div className="space-y-1.5">
                  <Label htmlFor="totp_code">{t('auth.totpCode')}</Label>
                  <Input
                    id="totp_code"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={6}
                    aria-invalid={errors.totp_code ? true : undefined}
                    aria-describedby="totp-hint"
                    {...register('totp_code')}
                  />
                  <p id="totp-hint" className="text-sm text-slate-600">
                    {errors.totp_code?.message ? t(errors.totp_code.message) : t('auth.totpHint')}
                  </p>
                </div>
              )}
              <Button type="submit" className="w-full" disabled={login.isPending}>
                {login.isPending ? t('auth.submitting') : t('auth.submit')}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </main>
  );
}
