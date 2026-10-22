import type { Establishment } from '@arenile/shared/api';
import { Button } from '@arenile/shared/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@arenile/shared/ui/card';
import { FieldDescription, FieldSeparator } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@arenile/shared/ui/input-group';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Textarea } from '@arenile/shared/ui/textarea';
import { zodResolver } from '@hookform/resolvers/zod';
import type { LucideIcon } from 'lucide-react';
import {
  Building2Icon,
  CodeIcon,
  GlobeIcon,
  LanguagesIcon,
  PaletteIcon,
  ReceiptTextIcon,
} from 'lucide-react';
import type { ReactNode } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useEstablishmentSection } from '../api/useEstablishmentSection';
import { EmbedSnippet } from '../components/EmbedSnippet';
import {
  BooleanField,
  Field,
  I18nTextField,
  MultiCheck,
  SelectField,
  SuffixInput,
} from '../components/fields';
import { FormAlert } from '../components/FormAlert';
import { ThemePreview, ThemeWarnings } from '../components/ThemePreview';
import { fieldAria } from '../lib/aria';
import { orderedLocales } from '../lib/i18nText';
import { timezoneOptions } from '../lib/timezones';
import {
  FONT_FAMILIES,
  RADII,
  ROUNDINGS,
  SUPPORTED_LOCALES,
  generalAliases,
  generalPatch,
  generalSchema,
  pricingSettingsAliases,
  pricingSettingsPatch,
  pricingSettingsSchema,
  publicPagePatch,
  publicPageSchema,
  themeAliases,
  themePatch,
  themeSchema,
  toGeneralValues,
  toPricingSettingsValues,
  toPublicPageValues,
  toThemeValues,
  type GeneralValues,
} from './establishment.schema';

export interface SectionProps {
  establishment: Establishment;
  /** Version the page is editing from (If-Match). */
  version: number;
  onVersion: (version: number) => void;
  readOnly: boolean;
  /** Called after a successful save (the wizard advances). */
  onSaved?: () => void;
  submitLabel?: string;
}

/** Card with an accessible region name; the save row goes in the footer. */
function SectionCard({
  id,
  icon: Icon,
  title,
  description,
  children,
  footer,
}: {
  id: string;
  icon: LucideIcon;
  title: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
}) {
  return (
    <Card aria-labelledby={`${id}-title`} role="region">
      <CardHeader className="flex flex-row items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary-on-surface">
          <Icon aria-hidden="true" className="size-4" />
        </span>
        <div className="space-y-1.5">
          <CardTitle id={`${id}-title`}>{title}</CardTitle>
          {description && <CardDescription>{description}</CardDescription>}
        </div>
      </CardHeader>
      <CardContent>{children}</CardContent>
      {footer}
    </Card>
  );
}

function SaveRow({
  readOnly,
  pending,
  dirty,
  label,
}: {
  readOnly: boolean;
  pending: boolean;
  dirty?: boolean;
  label?: string;
}) {
  const { t } = useTranslation();
  if (readOnly) return null;
  return (
    <CardFooter className="justify-end gap-3 border-t">
      {dirty && (
        <span className="mr-auto text-sm text-muted-foreground">
          {t('unsaved', { ns: 'configuration' })}
        </span>
      )}
      <Button type="submit" disabled={pending}>
        {pending && <Spinner />}
        {label ?? t('common.actions.save')}
      </Button>
    </CardFooter>
  );
}

// ---------------------------------------------------------------------------

type GeneralPart = 'all' | 'details' | 'languages';
const LANGUAGE_KEYS = ['default_locale', 'enabled_locales'] as const;

/** Only the fields shown by `part` are sent, so two tabs never overwrite each other. */
function partialGeneralPatch(v: GeneralValues, part: GeneralPart) {
  const patch = generalPatch(v);
  if (part === 'all') return patch;
  const out: Record<string, unknown> = {};
  for (const [k, value] of Object.entries(patch)) {
    const isLanguage = (LANGUAGE_KEYS as readonly string[]).includes(k);
    if (isLanguage === (part === 'languages')) out[k] = value;
  }
  return out as typeof patch;
}

export function GeneralSection({
  establishment,
  version,
  onVersion,
  readOnly,
  onSaved,
  submitLabel,
  part = 'all',
}: SectionProps & { part?: GeneralPart }) {
  const { t } = useTranslation('configuration');
  const form = useForm({
    resolver: zodResolver(generalSchema),
    defaultValues: toGeneralValues(establishment),
  });
  const s = useEstablishmentSection({
    establishmentId: establishment.id,
    form,
    version,
    onVersion,
    onSaved,
    toPatch: (v) => partialGeneralPatch(v, part),
    toValues: toGeneralValues,
    aliases: generalAliases,
  });
  const {
    register,
    control,
    formState: { errors, isDirty },
  } = form;
  const tzs = timezoneOptions(establishment.timezone);
  const languageOptions = SUPPORTED_LOCALES.map((l) => ({
    value: l,
    label: t(`common.languages.${l}`, { ns: 'translation' }),
  }));
  type TextName =
    | 'name'
    | 'contact_email'
    | 'contact_phone'
    | 'street'
    | 'city'
    | 'province'
    | 'postal_code'
    | 'country'
    | 'concession_ref';
  const text = (
    name: TextName,
    opts: { type?: string; autoComplete?: string; className?: string } = {},
  ) => (
    <Field
      id={`gen-${name}`}
      label={t(`establishment.fields.${name}`)}
      error={errors[name]}
      className={opts.className}
    >
      <Input
        {...fieldAria(`gen-${name}`, errors[name])}
        disabled={readOnly}
        type={opts.type}
        autoComplete={opts.autoComplete}
        {...register(name)}
      />
    </Field>
  );
  const details = (
    <>
      <div className="grid gap-5 sm:grid-cols-2">
        {text('name')}
        <Field
          id="gen-slug"
          label={t('establishment.fields.slug')}
          error={errors.slug}
          hint={t('establishment.general.slugHint')}
        >
          <InputGroup data-disabled={readOnly || undefined}>
            <InputGroupAddon>
              <span className="font-mono text-xs text-muted-foreground">/b/</span>
            </InputGroupAddon>
            <InputGroupInput
              {...fieldAria('gen-slug', errors.slug, true)}
              disabled={readOnly}
              {...register('slug')}
            />
          </InputGroup>
        </Field>
        <SelectField
          control={control}
          name="timezone"
          id="gen-timezone"
          label={t('establishment.fields.timezone')}
          disabled={readOnly}
          options={tzs.map((tz) => ({ value: tz, label: tz }))}
        />
      </div>
      <FieldSeparator className="my-3 *:data-[slot=field-separator-content]:bg-card">
        {t('establishment.general.contacts')}
      </FieldSeparator>
      <div className="grid gap-5 sm:grid-cols-2">
        {text('contact_email', { type: 'email', autoComplete: 'email' })}
        {text('contact_phone', { type: 'tel', autoComplete: 'tel' })}
        {text('street', { autoComplete: 'street-address', className: 'sm:col-span-2' })}
        {text('city', { autoComplete: 'address-level2' })}
        {text('province', { autoComplete: 'address-level1' })}
        {text('postal_code', { autoComplete: 'postal-code' })}
        {text('country', { autoComplete: 'country' })}
        {text('concession_ref', { className: 'sm:col-span-2' })}
      </div>
      <FieldDescription>
        {t('establishment.general.currency', { currency: establishment.currency })}
      </FieldDescription>
    </>
  );
  const languages = (
    <div className="grid gap-5">
      <SelectField
        control={control}
        name="default_locale"
        id="gen-default_locale"
        label={t('establishment.fields.default_locale')}
        disabled={readOnly}
        options={languageOptions}
        className="sm:max-w-xs"
      />
      <MultiCheck
        control={control}
        name="enabled_locales"
        id="gen-enabled_locales"
        legend={t('establishment.fields.enabled_locales')}
        disabled={readOnly}
        hint={t('establishment.general.languagesHint')}
        options={languageOptions}
      />
    </div>
  );
  const meta = {
    all: { id: 'general', icon: Building2Icon, title: t('establishment.general.title') },
    details: { id: 'general', icon: Building2Icon, title: t('establishment.general.titleDetails') },
    languages: {
      id: 'languages',
      icon: LanguagesIcon,
      title: t('establishment.general.languages'),
    },
  }[part];
  return (
    <form noValidate onSubmit={s.submit}>
      <SectionCard
        {...meta}
        footer={
          <SaveRow readOnly={readOnly} pending={s.pending} dirty={isDirty} label={submitLabel} />
        }
      >
        <div className="space-y-6">
          <FormAlert state={s.alert} onReload={s.reload} />
          {part !== 'languages' && details}
          {part === 'all' && (
            <FieldSeparator className="my-3 *:data-[slot=field-separator-content]:bg-card">
              {t('establishment.general.languages')}
            </FieldSeparator>
          )}
          {part !== 'details' && languages}
        </div>
      </SectionCard>
    </form>
  );
}

// ---------------------------------------------------------------------------

export function PublicPageSection({
  establishment,
  version,
  onVersion,
  readOnly,
  onSaved,
  submitLabel,
}: SectionProps) {
  const { t } = useTranslation('configuration');
  const form = useForm({
    resolver: zodResolver(publicPageSchema),
    defaultValues: toPublicPageValues(establishment),
  });
  const s = useEstablishmentSection({
    establishmentId: establishment.id,
    form,
    version,
    onVersion,
    onSaved,
    toPatch: (v) => publicPagePatch(v),
    toValues: toPublicPageValues,
  });
  const {
    register,
    formState: { errors, isDirty },
  } = form;
  return (
    <div className="space-y-6">
      <form noValidate onSubmit={s.submit}>
        <SectionCard
          id="public-page"
          icon={GlobeIcon}
          title={t('establishment.publicPage.title')}
          description={t('establishment.publicPage.description')}
          footer={
            <SaveRow readOnly={readOnly} pending={s.pending} dirty={isDirty} label={submitLabel} />
          }
        >
          <div className="space-y-5">
            <FormAlert state={s.alert} onReload={s.reload} />
            <BooleanField
              control={form.control}
              name="booking_enabled"
              id="pp-booking_enabled"
              variant="switch"
              disabled={readOnly}
              label={t('establishment.fields.booking_enabled')}
              description={t('establishment.publicPage.bookingHint')}
            />
            <Field
              id="pp-origins"
              label={t('establishment.fields.allowed_embed_origins')}
              error={errors.allowed_embed_origins}
              hint={t('establishment.publicPage.originsHint')}
            >
              <Textarea
                {...fieldAria('pp-origins', errors.allowed_embed_origins, true)}
                disabled={readOnly}
                placeholder="https://www.lidoesempio.it"
                rows={3}
                className="font-mono text-sm"
                {...register('allowed_embed_origins')}
              />
            </Field>
          </div>
        </SectionCard>
      </form>
      <SectionCard
        id="embed"
        icon={CodeIcon}
        title={t('embed.title')}
        description={t('embed.description')}
      >
        <EmbedSnippet slug={establishment.slug} lang={establishment.default_locale} />
      </SectionCard>
    </div>
  );
}

// ---------------------------------------------------------------------------

export function ThemeSection({
  establishment,
  version,
  onVersion,
  readOnly,
  onSaved,
  submitLabel,
}: SectionProps) {
  const { t } = useTranslation('configuration');
  const form = useForm({
    resolver: zodResolver(themeSchema),
    defaultValues: toThemeValues(establishment),
  });
  const s = useEstablishmentSection({
    establishmentId: establishment.id,
    form,
    version,
    onVersion,
    onSaved,
    toPatch: themePatch,
    toValues: toThemeValues,
    aliases: themeAliases,
  });
  const {
    register,
    setValue,
    control,
    formState: { errors, isDirty },
  } = form;
  const values = {
    logo_url: useWatch({ control, name: 'logo_url' }),
    primary_color: useWatch({ control, name: 'primary_color' }),
    accent_color: useWatch({ control, name: 'accent_color' }),
    font_family: useWatch({ control, name: 'font_family' }),
    radius: useWatch({ control, name: 'radius' }),
    show_prices_on_map: useWatch({ control, name: 'show_prices_on_map' }),
    hide_header_in_embed: useWatch({ control, name: 'hide_header_in_embed' }),
    intro: useWatch({ control, name: 'intro' }),
  };
  const locales = orderedLocales(establishment.default_locale, establishment.enabled_locales);
  const previewTheme = {
    logo_url: values.logo_url || undefined,
    primary_color: values.primary_color,
    accent_color: values.accent_color,
    font_family: values.font_family,
    radius: values.radius,
    show_prices_on_map: values.show_prices_on_map,
    hide_header_in_embed: values.hide_header_in_embed,
  };
  const colorField = (name: 'primary_color' | 'accent_color') => (
    <Field id={`th-${name}`} label={t(`theme.fields.${name}`)} error={errors[name]}>
      <InputGroup data-disabled={readOnly || undefined}>
        <InputGroupAddon>
          <input
            type="color"
            aria-label={t('theme.picker', { field: t(`theme.fields.${name}`) })}
            disabled={readOnly}
            value={/^#[0-9a-f]{6}$/i.test(values[name]) ? values[name] : '#000000'}
            onChange={(e) =>
              setValue(name, e.target.value, { shouldDirty: true, shouldValidate: true })
            }
            className="size-6 cursor-pointer rounded-md border-0 bg-transparent p-0 disabled:cursor-not-allowed"
          />
        </InputGroupAddon>
        <InputGroupInput
          {...fieldAria(`th-${name}`, errors[name])}
          disabled={readOnly}
          className="font-mono uppercase"
          maxLength={7}
          {...register(name)}
        />
      </InputGroup>
    </Field>
  );
  return (
    <form noValidate onSubmit={s.submit}>
      <SectionCard
        id="theme"
        icon={PaletteIcon}
        title={t('theme.title')}
        description={t('theme.description')}
        footer={
          <SaveRow readOnly={readOnly} pending={s.pending} dirty={isDirty} label={submitLabel} />
        }
      >
        <div className="grid gap-8 lg:grid-cols-[1fr_20rem]">
          <div className="space-y-5">
            <FormAlert state={s.alert} onReload={s.reload} />
            <div className="grid gap-5 sm:grid-cols-2">
              {colorField('primary_color')}
              {colorField('accent_color')}
              <SelectField
                control={control}
                name="font_family"
                id="th-font_family"
                label={t('theme.fields.font_family')}
                disabled={readOnly}
                options={FONT_FAMILIES.map((f) => ({ value: f, label: t(`theme.fonts.${f}`) }))}
              />
              <SelectField
                control={control}
                name="radius"
                id="th-radius"
                label={t('theme.fields.radius')}
                disabled={readOnly}
                options={RADII.map((r) => ({ value: r, label: t(`theme.radii.${r}`) }))}
              />
              <Field
                id="th-logo_url"
                label={t('theme.fields.logo_url')}
                error={errors.logo_url}
                className="sm:col-span-2"
              >
                <Input
                  {...fieldAria('th-logo_url', errors.logo_url)}
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
                id="th-show_prices"
                variant="switch"
                disabled={readOnly}
                label={t('theme.fields.show_prices_on_map')}
              />
              <BooleanField
                control={control}
                name="hide_header_in_embed"
                id="th-hide_header"
                variant="switch"
                disabled={readOnly}
                label={t('theme.fields.hide_header_in_embed')}
              />
            </div>
            <I18nTextField
              control={control}
              name="intro"
              id="th-intro"
              label={t('theme.fields.intro')}
              locales={locales}
              defaultLocale={establishment.default_locale}
              disabled={readOnly}
              multiline
            />
          </div>
          <ThemePreview
            theme={previewTheme}
            name={establishment.name}
            intro={values.intro?.[establishment.default_locale]}
          />
        </div>
      </SectionCard>
    </form>
  );
}

// ---------------------------------------------------------------------------

export function PricingSettingsSection({
  establishment,
  version,
  onVersion,
  readOnly,
}: SectionProps) {
  const { t } = useTranslation('configuration');
  const toValues = (e: Establishment) => toPricingSettingsValues(e);
  const form = useForm({
    resolver: zodResolver(pricingSettingsSchema),
    defaultValues: toValues(establishment),
  });
  const s = useEstablishmentSection({
    establishmentId: establishment.id,
    form,
    version,
    onVersion,
    toPatch: pricingSettingsPatch,
    toValues,
    aliases: pricingSettingsAliases,
  });
  const {
    register,
    control,
    formState: { errors, isDirty },
  } = form;
  return (
    <form noValidate onSubmit={s.submit}>
      <SectionCard
        id="pricing-settings"
        icon={ReceiptTextIcon}
        title={t('pricingSettings.title')}
        description={t('pricingSettings.description')}
        footer={<SaveRow readOnly={readOnly} pending={s.pending} dirty={isDirty} />}
      >
        <div className="space-y-5">
          <FormAlert state={s.alert} onReload={s.reload} />
          <div className="grid gap-5 sm:grid-cols-3">
            <Field
              id="ps-half_day_factor"
              label={t('pricingSettings.fields.half_day_factor')}
              error={errors.half_day_factor}
              hint={t('pricingSettings.halfDayHint')}
            >
              <SuffixInput
                {...fieldAria('ps-half_day_factor', errors.half_day_factor, true)}
                suffix="×"
                inputMode="decimal"
                disabled={readOnly}
                {...register('half_day_factor')}
              />
            </Field>
            <SelectField
              control={control}
              name="rounding"
              id="ps-rounding"
              label={t('pricingSettings.fields.rounding')}
              disabled={readOnly}
              options={ROUNDINGS.map((r) => ({
                value: r,
                label: t(`pricingSettings.roundings.${r}`),
              }))}
            />
            <Field
              id="ps-vat"
              label={t('pricingSettings.fields.default_vat_rate')}
              error={errors.default_vat_rate}
              hint={t('pricingSettings.vatHint')}
            >
              <SuffixInput
                {...fieldAria('ps-vat', errors.default_vat_rate, true)}
                suffix="%"
                inputMode="decimal"
                disabled={readOnly}
                {...register('default_vat_rate')}
              />
            </Field>
          </div>
        </div>
      </SectionCard>
    </form>
  );
}
