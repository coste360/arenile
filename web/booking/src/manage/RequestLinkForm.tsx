import { zodResolver } from '@hookform/resolvers/zod';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Field, FieldDescription, FieldError, FieldLabel } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { Spinner } from '@arenile/shared/ui/spinner';
import { MailCheckIcon } from 'lucide-react';
import { useId } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { useRequestManageLink } from '../api/public';
import { problemMessageKey } from '../flow/problems';
import { PUBLIC_NS } from '../i18n';

const schema = z.object({
  email: z.string().trim().min(1, 'who.validation.required').email('who.validation.email'),
  code: z
    .string()
    .trim()
    .min(3, 'who.validation.required')
    .max(32)
    .transform((v) => v.toUpperCase()),
});
type Values = z.input<typeof schema>;

/**
 * "Send me a new link" (spec 12.4): email + booking code. The server always
 * answers 202, so the page always shows the same neutral message (no
 * enumeration of bookings or emails).
 */
export function RequestLinkForm({ slug }: { slug: string }) {
  const { t } = useTranslation(PUBLIC_NS);
  const { t: tc } = useTranslation();
  const id = useId();
  const request = useRequestManageLink(slug);
  const form = useForm<Values, unknown, z.output<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { email: '', code: '' },
  });
  const { errors, isSubmitting } = form.formState;

  if (request.isSuccess) {
    return (
      <Alert role="status">
        <MailCheckIcon aria-hidden="true" />
        <AlertDescription>{t('manage.requestLinkSent')}</AlertDescription>
      </Alert>
    );
  }

  return (
    <form
      noValidate
      className="space-y-4"
      aria-labelledby={`${id}-title`}
      onSubmit={form.handleSubmit(async (v) => {
        try {
          await request.mutateAsync(v);
        } catch {
          // shown below
        }
      })}
    >
      <div className="space-y-1">
        <h2 id={`${id}-title`} className="text-lg font-semibold">
          {t('manage.requestLink')}
        </h2>
        <p className="text-sm text-muted-foreground">{t('manage.requestLinkHint')}</p>
      </div>
      <Field data-invalid={!!errors.email || undefined}>
        <FieldLabel htmlFor={`${id}-email`}>{t('who.email')}</FieldLabel>
        <Input
          id={`${id}-email`}
          type="email"
          autoComplete="email"
          className="h-11"
          aria-invalid={!!errors.email || undefined}
          aria-describedby={errors.email ? `${id}-email-error` : undefined}
          {...form.register('email')}
        />
        <FieldError id={`${id}-email-error`}>
          {errors.email?.message ? t(errors.email.message) : undefined}
        </FieldError>
      </Field>
      <Field data-invalid={!!errors.code || undefined}>
        <FieldLabel htmlFor={`${id}-code`}>{t('manage.bookingCode')}</FieldLabel>
        <Input
          id={`${id}-code`}
          autoComplete="off"
          autoCapitalize="characters"
          className="h-11 font-mono uppercase"
          aria-invalid={!!errors.code || undefined}
          aria-describedby={errors.code ? `${id}-code-error` : `${id}-code-hint`}
          {...form.register('code')}
        />
        <FieldDescription id={`${id}-code-hint`}>AR-1234</FieldDescription>
        <FieldError id={`${id}-code-error`}>
          {errors.code?.message ? t(errors.code.message) : undefined}
        </FieldError>
      </Field>
      {request.error && (
        <p role="alert" className="text-sm text-destructive">
          {tc(problemMessageKey(request.error))}
        </p>
      )}
      <Button type="submit" className="h-11" disabled={isSubmitting}>
        {isSubmitting && <Spinner />}
        {t('manage.send')}
      </Button>
    </form>
  );
}
