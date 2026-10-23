import { DEFAULT_ACCENT, DEFAULT_PRIMARY } from '@arenile/shared/theme';
import { zodResolver } from '@hookform/resolvers/zod';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Input } from '@arenile/shared/ui/input';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@arenile/shared/ui/input-group';
import { QueryState, ResponsiveTabsList } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Tabs, TabsContent } from '@arenile/shared/ui/tabs';
import { Textarea } from '@arenile/shared/ui/textarea';
import { Link } from '@tanstack/react-router';
import {
  CodeIcon,
  FileTextIcon,
  GlobeIcon,
  LayoutListIcon,
  PaletteIcon,
  ShieldIcon,
  WorkflowIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage } from '../configuration/components/ConfigPage';
import {
  BooleanField,
  Field,
  I18nTextField,
  MultiCheck,
  SelectField,
  SuffixInput,
} from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { ThemeWarnings } from '../configuration/components/ThemePreview';
import { fieldAria } from '../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import { useOnlineSettings, useUpdateOnlineSettings, type OnlineSettings } from './api';
import { CodeBlock } from './components/CodeBlock';
import { PublicPagePreview } from './components/PublicPagePreview';
import { SectionCard } from './components/SectionCard';
import { TermsSection } from './components/TermsSection';
import {
  FONT_FAMILIES,
  FORMULAS,
  RADII,
  SLOTS,
  onlineAliases,
  onlineSettingsSchema,
  themeOf,
  toOnlineInput,
  toOnlineValues,
  type OnlineSettingsValues,
} from './pages/online.schema';

const TABS = [
  { id: 'flow', icon: WorkflowIcon },
  { id: 'theme', icon: PaletteIcon },
  { id: 'terms', icon: FileTextIcon },
  { id: 'embed', icon: CodeIcon },
] as const;
type Tab = (typeof TABS)[number]['id'];

interface FormProps {
  settings: OnlineSettings;
  establishmentId: string;
  slug: string;
  locales: string[];
  defaultLocale: string;
  readOnly: boolean;
}

function OnlineSettingsForm({
  settings,
  establishmentId,
  slug,
  locales,
  defaultLocale,
  readOnly,
}: FormProps) {
  const { t } = useTranslation('online');
  const { t: tc } = useTranslation('configuration');
  const { t: tg } = useTranslation();
  const [tab, setTab] = useState<Tab>('flow');
  // Version the edit started from: background refetches never move it (412 on a stale save).
  const [version, setVersion] = useState(settings.version);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const update = useUpdateOnlineSettings(establishmentId);
  const query = useOnlineSettings(establishmentId);
  const form = useForm<OnlineSettingsValues>({
    resolver: zodResolver(onlineSettingsSchema),
    defaultValues: toOnlineValues(settings, locales),
  });
  const {
    control,
    register,
    setValue,
    formState: { errors, isDirty },
  } = form;
  const watched = useWatch({ control }) as OnlineSettingsValues;
  const previewTheme = themeOf(
    {
      primary_color: /^#[0-9a-f]{6}$/i.test(watched.primary_color ?? '')
        ? watched.primary_color
        : (settings.theme.primary_color ?? DEFAULT_PRIMARY),
      accent_color: /^#[0-9a-f]{6}$/i.test(watched.accent_color ?? '')
        ? watched.accent_color
        : (settings.theme.accent_color ?? DEFAULT_ACCENT),
      font_family: watched.font_family,
      radius: watched.radius,
      logo_url: /^https:\/\/\S+$/i.test(watched.logo_url ?? '') ? watched.logo_url : '',
      show_prices_on_map: watched.show_prices_on_map,
      hide_header_in_embed: watched.hide_header_in_embed,
      intro: watched.intro ?? {},
    },
    settings.theme,
  );

  const submit = form.handleSubmit(
    async (values) => {
      setAlert(null);
      try {
        const saved = await update.mutateAsync({
          version,
          body: toOnlineInput(values, query.data ?? settings),
        });
        setVersion(saved.version);
        form.reset(toOnlineValues(saved, locales));
        toast.success(t('saved'));
      } catch (err) {
        setAlert(
          problemToFormState(err, form.setError, {
            formKeys: Object.keys(form.getValues()),
            aliases: onlineAliases,
          }),
        );
      }
    },
    (errs) => {
      // Show the tab holding the first invalid field.
      const themeKeys = ['primary_color', 'accent_color', 'logo_url', 'intro'];
      setTab(Object.keys(errs).some((k) => themeKeys.includes(k)) ? 'theme' : 'flow');
    },
  );

  const reload = async () => {
    const res = await query.refetch();
    if (res.data) {
      setVersion(res.data.version);
      form.reset(toOnlineValues(res.data, locales));
      setAlert(null);
    }
  };

  const colorField = (name: 'primary_color' | 'accent_color') => (
    <Field id={`ol-${name}`} label={tc(`theme.fields.${name}`)} error={errors[name]}>
      <InputGroup data-disabled={readOnly || undefined}>
        <InputGroupAddon>
          <input
            type="color"
            aria-label={tc('theme.picker', { field: tc(`theme.fields.${name}`) })}
            disabled={readOnly}
            value={/^#[0-9a-f]{6}$/i.test(watched[name] ?? '') ? watched[name] : '#000000'}
            onChange={(e) =>
              setValue(name, e.target.value, { shouldDirty: true, shouldValidate: true })
            }
            className="size-6 cursor-pointer rounded-md border-0 bg-transparent p-0 disabled:cursor-not-allowed"
          />
        </InputGroupAddon>
        <InputGroupInput
          {...fieldAria(`ol-${name}`, errors[name])}
          disabled={readOnly}
          className="font-mono uppercase"
          maxLength={7}
          {...register(name)}
        />
      </InputGroup>
    </Field>
  );

  return (
    <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)} className="gap-6">
      <ResponsiveTabsList
        label={t('settings.tabsLabel')}
        value={tab}
        onValueChange={(v) => setTab(v as Tab)}
        options={TABS.map(({ id, icon }) => ({ value: id, icon, label: t(`settings.tabs.${id}`) }))}
      />

      <form noValidate onSubmit={submit} className="space-y-6" aria-label={t('settings.title')}>
        <FormAlert state={alert} onReload={() => void reload()} />

        <TabsContent value="flow" forceMount hidden={tab !== 'flow'} className="space-y-6">
          <SectionCard
            id="ol-booking"
            icon={GlobeIcon}
            title={t('settings.booking.title')}
            description={t('settings.booking.description')}
          >
            <div className="space-y-4">
              <BooleanField
                control={control}
                name="booking_enabled"
                id="ol-booking_enabled"
                variant="switch"
                disabled={readOnly}
                label={t('settings.booking.enabled')}
                description={t('settings.booking.enabledHint')}
              />
              <Field
                id="ol-origins"
                label={t('settings.booking.origins')}
                error={errors.allowed_embed_origins}
                hint={t('settings.booking.originsHint')}
              >
                <Textarea
                  {...fieldAria('ol-origins', errors.allowed_embed_origins, true)}
                  disabled={readOnly}
                  placeholder="https://www.lidoesempio.it"
                  rows={3}
                  className="font-mono text-sm"
                  {...register('allowed_embed_origins')}
                />
              </Field>
            </div>
          </SectionCard>

          <SectionCard
            id="ol-flow"
            icon={WorkflowIcon}
            title={t('settings.flow.title')}
            description={t('settings.flow.description')}
          >
            <div className="space-y-6">
              <MultiCheck
                control={control}
                name="formulas"
                id="ol-formulas"
                legend={t('settings.flow.formulas')}
                hint={t('settings.flow.formulasHint')}
                disabled={readOnly}
                options={FORMULAS.map((f) => ({ value: f, label: tc(`formulas.${f}`) }))}
              />
              <MultiCheck
                control={control}
                name="slots"
                id="ol-slots"
                legend={t('settings.flow.slots')}
                disabled={readOnly}
                options={SLOTS.map((s) => ({ value: s, label: t(`settings.flow.slot.${s}`) }))}
              />
              <div className="grid gap-4 sm:grid-cols-2">
                <Field
                  id="ol-max_days_ahead"
                  label={t('settings.flow.maxDaysAhead')}
                  error={errors.max_days_ahead}
                  hint={t('settings.flow.maxDaysAheadHint')}
                >
                  <SuffixInput
                    {...fieldAria('ol-max_days_ahead', errors.max_days_ahead, true)}
                    suffix={t('settings.flow.days')}
                    inputMode="numeric"
                    disabled={readOnly}
                    {...register('max_days_ahead')}
                  />
                </Field>
              </div>
              <div className="grid gap-3 md:grid-cols-3">
                <BooleanField
                  control={control}
                  name="show_rate_periods"
                  id="ol-show_rate_periods"
                  variant="switch"
                  disabled={readOnly}
                  label={t('settings.flow.showRatePeriods')}
                />
                <BooleanField
                  control={control}
                  name="promo_codes_enabled"
                  id="ol-promo"
                  variant="switch"
                  disabled={readOnly}
                  label={t('settings.flow.promoCodes')}
                />
                <BooleanField
                  control={control}
                  name="require_phone"
                  id="ol-require_phone"
                  variant="switch"
                  disabled={readOnly}
                  label={t('settings.flow.requirePhone')}
                />
              </div>
            </div>
          </SectionCard>

          <SectionCard
            id="ol-catalog"
            icon={LayoutListIcon}
            title={t('settings.offer.title')}
            description={t('settings.offer.description')}
          >
            <div className="flex flex-wrap gap-2">
              <Button asChild variant="outline">
                <Link to="/e/$establishmentId/config/zones" params={{ establishmentId }}>
                  {t('settings.offer.zones')}
                </Link>
              </Button>
              <Button asChild variant="outline">
                <Link to="/e/$establishmentId/config/catalog" params={{ establishmentId }}>
                  {t('settings.offer.catalog')}
                </Link>
              </Button>
              <Button asChild variant="outline">
                <Link to="/e/$establishmentId/policies" params={{ establishmentId }}>
                  <ShieldIcon aria-hidden="true" />
                  {t('settings.offer.policies')}
                </Link>
              </Button>
            </div>
          </SectionCard>
        </TabsContent>

        <TabsContent value="theme" forceMount hidden={tab !== 'theme'}>
          <SectionCard
            id="ol-theme"
            icon={PaletteIcon}
            title={tc('theme.title')}
            description={t('settings.themeDescription')}
          >
            <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_minmax(0,26rem)]">
              <div className="space-y-5">
                <div className="grid gap-5 sm:grid-cols-2">
                  {colorField('primary_color')}
                  {colorField('accent_color')}
                  <SelectField
                    control={control}
                    name="font_family"
                    id="ol-font_family"
                    label={tc('theme.fields.font_family')}
                    disabled={readOnly}
                    options={FONT_FAMILIES.map((f) => ({
                      value: f,
                      label: tc(`theme.fonts.${f}`),
                    }))}
                  />
                  <SelectField
                    control={control}
                    name="radius"
                    id="ol-radius"
                    label={tc('theme.fields.radius')}
                    disabled={readOnly}
                    options={RADII.map((r) => ({ value: r, label: tc(`theme.radii.${r}`) }))}
                  />
                  <Field
                    id="ol-logo_url"
                    label={tc('theme.fields.logo_url')}
                    error={errors.logo_url}
                    className="sm:col-span-2"
                  >
                    <Input
                      {...fieldAria('ol-logo_url', errors.logo_url)}
                      disabled={readOnly}
                      type="url"
                      placeholder="https://"
                      {...register('logo_url')}
                    />
                  </Field>
                </div>
                <ThemeWarnings theme={previewTheme} />
                <div className="grid gap-3">
                  <BooleanField
                    control={control}
                    name="show_prices_on_map"
                    id="ol-show_prices"
                    variant="switch"
                    disabled={readOnly}
                    label={tc('theme.fields.show_prices_on_map')}
                  />
                  <BooleanField
                    control={control}
                    name="hide_header_in_embed"
                    id="ol-hide_header"
                    variant="switch"
                    disabled={readOnly}
                    label={tc('theme.fields.hide_header_in_embed')}
                  />
                </div>
                <I18nTextField
                  control={control}
                  name="intro"
                  id="ol-intro"
                  label={tc('theme.fields.intro')}
                  locales={locales}
                  defaultLocale={defaultLocale}
                  disabled={readOnly}
                  multiline
                />
              </div>
              <PublicPagePreview slug={slug} theme={previewTheme} lang={defaultLocale} />
            </div>
          </SectionCard>
        </TabsContent>

        {!readOnly && (tab === 'flow' || tab === 'theme') && (
          <div className="sticky bottom-4 z-10 flex items-center justify-end gap-3 rounded-xl border bg-background/95 p-3 shadow-md backdrop-blur">
            {isDirty && (
              <span className="mr-auto text-sm text-muted-foreground">{tc('unsaved')}</span>
            )}
            <Button type="submit" disabled={update.isPending}>
              {update.isPending && <Spinner />}
              {tg('common.actions.save')}
            </Button>
          </div>
        )}
      </form>

      <TabsContent value="terms" forceMount hidden={tab !== 'terms'}>
        <TermsSection
          establishmentId={establishmentId}
          locales={locales}
          defaultLocale={defaultLocale}
          readOnly={readOnly}
        />
      </TabsContent>

      <TabsContent value="embed" forceMount hidden={tab !== 'embed'}>
        <SectionCard
          id="ol-embed"
          icon={CodeIcon}
          title={t('settings.embed.title')}
          description={t('settings.embed.description')}
        >
          <div className="space-y-5">
            <CodeBlock
              label={t('settings.embed.pageUrl')}
              value={settings.embed.page_url}
              language="URL"
            />
            <CodeBlock label={t('settings.embed.script')} value={settings.embed.script} />
            <CodeBlock label={t('settings.embed.iframe')} value={settings.embed.iframe} />
            <Alert>
              <ShieldIcon aria-hidden="true" />
              <AlertDescription>
                {settings.allowed_embed_origins.length > 0
                  ? t('settings.embed.originsList', {
                      origins: settings.allowed_embed_origins.join(', '),
                    })
                  : t('settings.embed.originsNone')}
              </AlertDescription>
            </Alert>
            <p className="text-sm text-muted-foreground">{t('settings.embed.events')}</p>
          </div>
        </SectionCard>
      </TabsContent>
    </Tabs>
  );
}

/**
 * "Pagina pubblica" (spec 12, 15.1): booking switch and allowed embed
 * origins (frame-ancestors), online flow options, theme editor with the
 * LIVE preview of the real page in preview mode, versioned terms and the
 * embed snippets (iframe and embed.js). API: GET/PUT online-settings
 * (If-Match), GET/POST terms-versions.
 */
export function OnlineSettingsPage() {
  const { t } = useTranslation('online');
  const ctx = useConfigContext();
  const query = useOnlineSettings(ctx.establishmentId);
  const est = ctx.establishment;
  const settings = query.data;
  return (
    <ConfigPage
      title={t('settings.title')}
      description={t('settings.description')}
      readOnly={!!est && !ctx.canConfigure}
    >
      {!settings || !est ? (
        <QueryState
          isLoading={query.isPending || ctx.isLoading}
          error={query.error ?? ctx.error}
          onRetry={() => void query.refetch()}
          rows={6}
        />
      ) : (
        <OnlineSettingsForm
          key={est.id}
          settings={settings}
          establishmentId={est.id}
          slug={est.slug}
          locales={ctx.locales}
          defaultLocale={ctx.defaultLocale}
          readOnly={!ctx.canConfigure}
        />
      )}
    </ConfigPage>
  );
}
