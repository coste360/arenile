import { zodResolver } from '@hookform/resolvers/zod';
import { useMe } from '@arenile/shared/api';
import { Button } from '@arenile/shared/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import { Input } from '@arenile/shared/ui/input';
import { NativeSelect, NativeSelectOption } from '@arenile/shared/ui/native-select';
import { ConfirmDialog, QueryState, StatusBadge } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Textarea } from '@arenile/shared/ui/textarea';
import { BellRingIcon, BracesIcon, EyeIcon, MailIcon, RotateCcwIcon, SendIcon } from 'lucide-react';
import { useEffect, useId, useMemo, useRef, useState } from 'react';
import { useForm, useWatch, type UseFormReturn } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage } from '../configuration/components/ConfigPage';
import { BooleanField, Field } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { fieldAria } from '../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import {
  useEmailSettings,
  useEmailTemplates,
  usePreviewEmailTemplate,
  useResetEmailTemplate,
  useSendEmailTemplateTest,
  useUpdateEmailSettings,
  useUpdateEmailTemplate,
  type EmailLocale,
  type EmailSettings,
  type EmailTemplate,
  type EmailTemplateKind,
  type EmailTemplatePreview,
} from './api';
import { SectionCard } from './components/SectionCard';
import {
  EMAIL_LOCALES,
  LEAD_DAYS,
  LIMITS,
  TEMPLATE_KINDS,
  insertAt,
  remindersSchema,
  templateSchema,
  timeOptions,
  toRemindersInput,
  toRemindersValues,
  toTemplateValues,
  type RemindersValues,
  type TemplateField,
  type TemplateValues,
} from './pages/customerEmails.schema';

/** Delay before the live preview follows the typing. */
const PREVIEW_DEBOUNCE_MS = 400;

function RemindersCard({
  settings,
  establishmentId,
  readOnly,
}: {
  settings: EmailSettings;
  establishmentId: string;
  readOnly: boolean;
}) {
  const { t } = useTranslation('online');
  const { t: tg } = useTranslation();
  const [version, setVersion] = useState(settings.version);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const update = useUpdateEmailSettings(establishmentId);
  const query = useEmailSettings(establishmentId);
  const form = useForm<RemindersValues>({
    resolver: zodResolver(remindersSchema),
    defaultValues: toRemindersValues(settings),
  });
  const [dayBefore, balance, dayTime, balanceTime] = useWatch({
    control: form.control,
    name: ['day_before_reminder', 'balance_reminder', 'day_before_time', 'balance_time'],
  });
  const submit = form.handleSubmit(async (v) => {
    setAlert(null);
    // Latest settings: the sender fields may have been saved in the other page.
    const current = query.data ?? settings;
    try {
      const saved = await update.mutateAsync({ version, body: toRemindersInput(current, v) });
      setVersion(saved.version);
      form.reset(toRemindersValues(saved));
      toast.success(t('saved'));
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(v) }));
    }
  });
  const reload = async () => {
    const res = await query.refetch();
    if (res.data) {
      setVersion(res.data.version);
      form.reset(toRemindersValues(res.data));
      setAlert(null);
    }
  };
  return (
    <form noValidate onSubmit={submit}>
      <SectionCard
        id="mail-reminders"
        icon={BellRingIcon}
        title={t('customerEmails.reminders.title')}
        description={t('customerEmails.reminders.description')}
        footer={
          readOnly ? undefined : (
            <>
              {form.formState.isDirty && (
                <span className="mr-auto text-sm text-muted-foreground">{t('unsaved')}</span>
              )}
              <Button type="submit" disabled={update.isPending}>
                {update.isPending && <Spinner />}
                {tg('common.actions.save')}
              </Button>
            </>
          )
        }
      >
        <div className="space-y-4">
          <FormAlert state={alert} onReload={() => void reload()} />
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <div className="space-y-3">
              <BooleanField
                control={form.control}
                name="day_before_reminder"
                id="rem-day-before"
                variant="switch"
                disabled={readOnly}
                label={t('customerEmails.reminders.dayBefore')}
                description={t('customerEmails.reminders.dayBeforeHint', { time: dayTime })}
              />
              <Field id="rem-day-before-time" label={t('customerEmails.reminders.dayBeforeTime')}>
                <NativeSelect
                  id="rem-day-before-time"
                  className="w-full"
                  disabled={readOnly || !dayBefore}
                  {...form.register('day_before_time')}
                >
                  {timeOptions(settings.reminders.day_before_time).map((v) => (
                    <NativeSelectOption key={v} value={v}>
                      {v}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </Field>
            </div>
            <div className="space-y-3">
              <BooleanField
                control={form.control}
                name="balance_reminder"
                id="rem-balance"
                variant="switch"
                disabled={readOnly}
                label={t('customerEmails.reminders.balance')}
                description={t('customerEmails.reminders.balanceHint', { time: balanceTime })}
              />
              <div className="grid gap-3 sm:grid-cols-2">
                <Field id="rem-balance-time" label={t('customerEmails.reminders.balanceTime')}>
                  <NativeSelect
                    id="rem-balance-time"
                    className="w-full"
                    disabled={readOnly || !balance}
                    {...form.register('balance_time')}
                  >
                    {timeOptions(settings.reminders.balance_time).map((v) => (
                      <NativeSelectOption key={v} value={v}>
                        {v}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </Field>
                <Field id="rem-balance-lead" label={t('customerEmails.reminders.leadDays')}>
                  <NativeSelect
                    id="rem-balance-lead"
                    className="w-full"
                    disabled={readOnly || !balance}
                    {...form.register('balance_lead_days')}
                  >
                    {LEAD_DAYS.map((d) => (
                      <NativeSelectOption key={d} value={String(d)}>
                        {t('customerEmails.reminders.leadOption', { count: d })}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </Field>
              </div>
            </div>
          </div>
          <p className="text-sm text-muted-foreground">{t('customerEmails.reminders.rules')}</p>
        </div>
      </SectionCard>
    </form>
  );
}

/** Text control (Input or Textarea) with the placeholder picker target tracking. */
function TextField({
  form,
  name,
  label,
  readOnly,
  multiline,
  onFocusField,
  registerEl,
}: {
  form: UseFormReturn<TemplateValues>;
  name: TemplateField;
  label: string;
  readOnly: boolean;
  multiline: boolean;
  onFocusField: (name: TemplateField) => void;
  registerEl: (name: TemplateField, el: HTMLInputElement | HTMLTextAreaElement | null) => void;
}) {
  const { t } = useTranslation('online');
  const id = `tpl-${name}`;
  const error = form.formState.errors[name];
  const value = useWatch({ control: form.control, name }) ?? '';
  const reg = form.register(name);
  const common = {
    ...fieldAria(id, error, true),
    ...reg,
    ref: (el: HTMLInputElement | HTMLTextAreaElement | null) => {
      reg.ref(el);
      registerEl(name, el);
    },
    disabled: readOnly,
    onFocus: () => onFocusField(name),
  };
  return (
    <Field
      id={id}
      label={label}
      error={error}
      hint={t('customerEmails.editor.counter', { count: value.length, max: LIMITS[name] })}
    >
      {multiline ? (
        <Textarea rows={name === 'signature' ? 2 : 4} {...common} />
      ) : (
        <Input {...common} />
      )}
    </Field>
  );
}

function PreviewPanel({
  preview,
  pending,
  error,
}: {
  preview: EmailTemplatePreview | undefined;
  pending: boolean;
  error: boolean;
}) {
  const { t } = useTranslation('online');
  return (
    <section aria-labelledby="tpl-preview-title" className="min-w-0 space-y-2">
      <div className="flex items-center gap-2">
        <EyeIcon aria-hidden="true" className="size-4 text-muted-foreground" />
        <h3 id="tpl-preview-title" className="text-sm font-medium">
          {t('customerEmails.preview.title')}
        </h3>
        {pending && <Spinner className="size-4" />}
      </div>
      {error && <p className="text-sm text-destructive">{t('customerEmails.preview.invalid')}</p>}
      {preview && (
        <>
          <p className="truncate rounded-md border bg-muted px-3 py-2 text-sm">
            <span className="text-muted-foreground">{t('customerEmails.preview.subject')}: </span>
            <span data-testid="preview-subject">{preview.subject}</span>
          </p>
          <iframe
            title={t('customerEmails.preview.frame')}
            srcDoc={preview.html}
            sandbox=""
            className="h-128 w-full rounded-md border bg-background lg:h-160"
          />
        </>
      )}
      <p className="text-xs text-muted-foreground">{t('customerEmails.preview.sample')}</p>
    </section>
  );
}

function TemplateEditor({
  template,
  placeholders,
  establishmentId,
  readOnly,
  userEmail,
}: {
  template: EmailTemplate;
  placeholders: string[];
  establishmentId: string;
  readOnly: boolean;
  userEmail: string | undefined;
}) {
  const { t } = useTranslation('online');
  const { t: tg } = useTranslation();
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const update = useUpdateEmailTemplate(establishmentId);
  const reset = useResetEmailTemplate(establishmentId);
  const sendTest = useSendEmailTemplateTest(establishmentId);
  const preview = usePreviewEmailTemplate(establishmentId);
  const schema = useMemo(() => templateSchema(placeholders), [placeholders]);
  const form = useForm<TemplateValues>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: toTemplateValues(template),
  });
  const target = { kind: template.kind, locale: template.locale };
  const lastField = useRef<TemplateField>('opening');
  const elements = useRef<Partial<Record<TemplateField, HTMLInputElement | HTMLTextAreaElement>>>(
    {},
  );
  const values = useWatch({ control: form.control });

  // Live preview, debounced, only for texts the client accepts.
  const previewMutate = preview.mutate;
  const serialized = JSON.stringify(values);
  useEffect(() => {
    const body = JSON.parse(serialized) as TemplateValues;
    const parsed = schema.safeParse(body);
    if (!parsed.success) return;
    const timer = setTimeout(
      () => previewMutate({ kind: template.kind, locale: template.locale, body: parsed.data }),
      PREVIEW_DEBOUNCE_MS,
    );
    return () => clearTimeout(timer);
  }, [serialized, schema, previewMutate, template.kind, template.locale]);

  const insert = (name: string) => {
    const field = lastField.current;
    const el = elements.current[field];
    const current = form.getValues(field);
    const next = insertAt(
      current,
      `{${name}}`,
      el?.selectionStart ?? null,
      el?.selectionEnd ?? null,
    );
    form.setValue(field, next.value, { shouldDirty: true, shouldValidate: true });
    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(next.caret, next.caret);
    });
  };

  const save = form.handleSubmit(async (v) => {
    setAlert(null);
    try {
      const saved = await update.mutateAsync({ ...target, version: template.version, body: v });
      form.reset(toTemplateValues(saved));
      toast.success(t('saved'));
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(v) }));
    }
  });
  const test = form.handleSubmit(async (v) => {
    try {
      await sendTest.mutateAsync({ ...target, body: v });
      toast.success(t('customerEmails.editor.testQueued', { to: userEmail ?? '' }));
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(v) }));
    }
  });
  const restore = async () => {
    const saved = await reset.mutateAsync({ ...target, version: template.version });
    form.reset(toTemplateValues(saved));
    setAlert(null);
    toast.success(t('customerEmails.editor.restored'));
  };

  const fields: { name: TemplateField; multiline: boolean }[] = [
    { name: 'subject', multiline: false },
    { name: 'opening', multiline: true },
    { name: 'closing', multiline: true },
    { name: 'signature', multiline: true },
  ];

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <form noValidate onSubmit={save} className="min-w-0 space-y-4">
        <FormAlert state={alert} />
        <div className="flex flex-wrap items-center gap-2">
          <StatusBadge tone={template.customized ? 'info' : 'neutral'}>
            {t(
              template.customized
                ? 'customerEmails.editor.customized'
                : 'customerEmails.editor.default',
            )}
          </StatusBadge>
          {!readOnly && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button type="button" variant="outline" size="sm" className="ml-auto">
                  <BracesIcon aria-hidden="true" />
                  {t('customerEmails.editor.insert')}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuLabel>{t('customerEmails.editor.insertHint')}</DropdownMenuLabel>
                {placeholders.map((p) => (
                  <DropdownMenuItem key={p} onSelect={() => insert(p)}>
                    <span className="font-mono text-xs">{`{${p}}`}</span>
                    <span className="text-muted-foreground">
                      {t(`customerEmails.placeholders.${p}`, { defaultValue: p })}
                    </span>
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
        {fields.map((f) => (
          <TextField
            key={f.name}
            form={form}
            name={f.name}
            label={t(`customerEmails.editor.${f.name}`)}
            readOnly={readOnly}
            multiline={f.multiline}
            onFocusField={(n) => (lastField.current = n)}
            registerEl={(n, el) => {
              if (el) elements.current[n] = el;
            }}
          />
        ))}
        <p className="text-sm text-muted-foreground">
          {t('customerEmails.editor.fixed', {
            blocks: template.fixed_blocks.map((b) => t(`customerEmails.blocks.${b}`)).join(', '),
          })}
        </p>
        {!readOnly && (
          <div className="flex flex-wrap items-center gap-2">
            <Button type="submit" disabled={update.isPending}>
              {update.isPending && <Spinner />}
              {tg('common.actions.save')}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={sendTest.isPending || !userEmail}
              onClick={() => void test()}
              title={userEmail}
            >
              {sendTest.isPending ? <Spinner /> : <SendIcon aria-hidden="true" />}
              {t('customerEmails.editor.sendTest')}
            </Button>
            <ConfirmDialog
              title={t('customerEmails.editor.restoreTitle')}
              description={t('customerEmails.editor.restoreDescription')}
              confirmLabel={t('customerEmails.editor.restore')}
              destructive
              onConfirm={restore}
              errorText={() => tg('common.errors.generic')}
              trigger={
                <Button type="button" variant="ghost" disabled={!template.customized}>
                  <RotateCcwIcon aria-hidden="true" />
                  {t('customerEmails.editor.restore')}
                </Button>
              }
            />
            {form.formState.isDirty && (
              <span className="text-sm text-muted-foreground">{t('unsaved')}</span>
            )}
          </div>
        )}
      </form>
      <PreviewPanel preview={preview.data} pending={preview.isPending} error={preview.isError} />
    </div>
  );
}

function TemplatesCard({
  establishmentId,
  locales,
  defaultLocale,
  readOnly,
}: {
  establishmentId: string;
  locales: EmailLocale[];
  defaultLocale: EmailLocale;
  readOnly: boolean;
}) {
  const { t } = useTranslation('online');
  const query = useEmailTemplates(establishmentId);
  const me = useMe();
  const kindId = useId();
  const localeId = useId();
  const [kind, setKind] = useState<EmailTemplateKind>('booking_confirmed');
  const [locale, setLocale] = useState<EmailLocale>(defaultLocale);
  const template = query.data?.items.find((i) => i.kind === kind && i.locale === locale);
  return (
    <SectionCard
      id="mail-templates"
      icon={MailIcon}
      title={t('customerEmails.editor.title')}
      description={t('customerEmails.editor.description')}
    >
      {!query.data ? (
        <QueryState
          isLoading={query.isPending}
          error={query.error}
          onRetry={() => void query.refetch()}
          rows={4}
        />
      ) : (
        <div className="space-y-5">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field id={kindId} label={t('customerEmails.editor.kind')}>
              <NativeSelect
                id={kindId}
                className="w-full"
                value={kind}
                onChange={(e) => setKind(e.target.value as EmailTemplateKind)}
              >
                {TEMPLATE_KINDS.map((k) => (
                  <NativeSelectOption key={k} value={k}>
                    {t(`email.templates.${k}`)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </Field>
            <Field id={localeId} label={t('customerEmails.editor.locale')}>
              <NativeSelect
                id={localeId}
                className="w-full"
                value={locale}
                onChange={(e) => setLocale(e.target.value as EmailLocale)}
              >
                {locales.map((l) => (
                  <NativeSelectOption key={l} value={l}>
                    {t(`customerEmails.locales.${l}`)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </Field>
          </div>
          {template && (
            <TemplateEditor
              key={`${template.kind}/${template.locale}`}
              template={template}
              placeholders={query.data.placeholders}
              establishmentId={establishmentId}
              readOnly={readOnly}
              userEmail={me.data?.user?.email}
            />
          )}
        </div>
      )}
    </SectionCard>
  );
}

/**
 * "Email ai clienti" (ADR 0022): automatic reminders (toggles and send
 * times, saved in the email settings with If-Match) and the editable texts
 * of every customer email per language (subject, opening, closing,
 * signature) with placeholder picker, live preview on sample data, test to
 * the user's own address and restore default. API: GET/PUT email-settings,
 * GET email-templates, PUT/DELETE email-templates/{kind}/{locale}
 * (If-Match), POST .../preview and .../test.
 */
export function CustomerEmailsPage() {
  const { t } = useTranslation('online');
  const ctx = useConfigContext();
  const est = ctx.establishment;
  const settings = useEmailSettings(ctx.establishmentId);
  const readOnly = !ctx.canConfigure;
  const enabled = (est?.enabled_locales ?? []).filter((l): l is EmailLocale =>
    (EMAIL_LOCALES as readonly string[]).includes(l),
  );
  const locales = enabled.length > 0 ? enabled : [...EMAIL_LOCALES];
  const defaultLocale = locales.includes(est?.default_locale as EmailLocale)
    ? (est?.default_locale as EmailLocale)
    : locales[0]!;
  return (
    <ConfigPage
      title={t('customerEmails.title')}
      description={t('customerEmails.description')}
      readOnly={!!est && readOnly}
    >
      {!settings.data || !est ? (
        <QueryState
          isLoading={settings.isPending || ctx.isLoading}
          error={settings.error ?? ctx.error}
          onRetry={() => void settings.refetch()}
          rows={6}
        />
      ) : (
        <div className="space-y-6">
          <RemindersCard
            key={`rem-${est.id}`}
            settings={settings.data}
            establishmentId={est.id}
            readOnly={readOnly}
          />
          <TemplatesCard
            key={`tpl-${est.id}`}
            establishmentId={est.id}
            locales={locales}
            defaultLocale={defaultLocale}
            readOnly={readOnly}
          />
        </div>
      )}
    </ConfigPage>
  );
}
