import type { Establishment } from '@arenile/shared/api';
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
} from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import type { ReactNode } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useEstablishmentSection } from '../api/useEstablishmentSection';
import { EmbedSnippet } from '../components/EmbedSnippet';
import { Checkbox, CheckboxGroup, Field, Select, Textarea } from '../components/fields';
import { FormAlert, SavedNotice } from '../components/FormAlert';
import { I18nTextInput } from '../components/I18nTextInput';
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

function SectionCard({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <Card aria-labelledby={`${id}-title`} role="region">
      <CardHeader>
        <CardTitle id={`${id}-title`}>{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

function SaveRow({
  readOnly,
  pending,
  saved,
  label,
}: {
  readOnly: boolean;
  pending: boolean;
  saved: boolean;
  label?: string;
}) {
  const { t } = useTranslation();
  if (readOnly) return null;
  return (
    <div className="flex items-center gap-3 pt-2">
      <Button type="submit" disabled={pending}>
        {pending ? t('common.actions.saving') : (label ?? t('common.actions.save'))}
      </Button>
      <SavedNotice show={saved} />
    </div>
  );
}

// ---------------------------------------------------------------------------

export function GeneralSection({
  establishment,
  version,
  onVersion,
  readOnly,
  onSaved,
  submitLabel,
}: SectionProps) {
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
    toPatch: (v) => generalPatch(v),
    toValues: toGeneralValues,
    aliases: generalAliases,
  });
  const {
    register,
    formState: { errors },
  } = form;
  const tzs = timezoneOptions(establishment.timezone);
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
  const text = (name: TextName, opts: { type?: string; autoComplete?: string } = {}) => (
    <Field id={`gen-${name}`} label={t(`establishment.fields.${name}`)} error={errors[name]}>
      <Input
        {...fieldAria(`gen-${name}`, errors[name])}
        disabled={readOnly}
        type={opts.type}
        autoComplete={opts.autoComplete}
        {...register(name)}
      />
    </Field>
  );
  return (
    <SectionCard id="general" title={t('establishment.general.title')}>
      <form noValidate onSubmit={s.submit} className="space-y-4">
        <FormAlert state={s.alert} onReload={s.reload} />
        <div className="grid gap-4 sm:grid-cols-2">
          {text('name')}
          <Field
            id="gen-slug"
            label={t('establishment.fields.slug')}
            error={errors.slug}
            hint={t('establishment.general.slugHint')}
          >
            <Input
              {...fieldAria('gen-slug', errors.slug, true)}
              disabled={readOnly}
              {...register('slug')}
            />
          </Field>
          <Field
            id="gen-timezone"
            label={t('establishment.fields.timezone')}
            error={errors.timezone}
          >
            <Select
              {...fieldAria('gen-timezone', errors.timezone)}
              disabled={readOnly}
              {...register('timezone')}
            >
              {tzs.map((tz) => (
                <option key={tz} value={tz}>
                  {tz}
                </option>
              ))}
            </Select>
          </Field>
          <Field
            id="gen-default_locale"
            label={t('establishment.fields.default_locale')}
            error={errors.default_locale}
          >
            <Select
              {...fieldAria('gen-default_locale', errors.default_locale)}
              disabled={readOnly}
              {...register('default_locale')}
            >
              {SUPPORTED_LOCALES.map((l) => (
                <option key={l} value={l}>
                  {t(`common.languages.${l}`, { ns: 'translation' })}
                </option>
              ))}
            </Select>
          </Field>
        </div>
        <CheckboxGroup
          id="gen-enabled_locales"
          legend={t('establishment.fields.enabled_locales')}
          error={errors.enabled_locales as never}
        >
          {SUPPORTED_LOCALES.map((l) => (
            <Checkbox
              key={l}
              id={`gen-locale-${l}`}
              value={l}
              disabled={readOnly}
              label={t(`common.languages.${l}`, { ns: 'translation' })}
              {...register('enabled_locales')}
            />
          ))}
        </CheckboxGroup>
        <h3 className="pt-2 text-sm font-semibold text-slate-800">
          {t('establishment.general.contacts')}
        </h3>
        <div className="grid gap-4 sm:grid-cols-2">
          {text('contact_email', { type: 'email', autoComplete: 'email' })}
          {text('contact_phone', { type: 'tel', autoComplete: 'tel' })}
          {text('street', { autoComplete: 'street-address' })}
          {text('city', { autoComplete: 'address-level2' })}
          {text('province', { autoComplete: 'address-level1' })}
          {text('postal_code', { autoComplete: 'postal-code' })}
          {text('country', { autoComplete: 'country' })}
          {text('concession_ref')}
        </div>
        <p className="text-xs text-slate-600">
          {t('establishment.general.currency', { currency: establishment.currency })}
        </p>
        <SaveRow readOnly={readOnly} pending={s.pending} saved={s.saved} label={submitLabel} />
      </form>
    </SectionCard>
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
    formState: { errors },
  } = form;
  return (
    <SectionCard
      id="public-page"
      title={t('establishment.publicPage.title')}
      description={t('establishment.publicPage.description')}
    >
      <form noValidate onSubmit={s.submit} className="space-y-4">
        <FormAlert state={s.alert} onReload={s.reload} />
        <Checkbox
          id="pp-booking_enabled"
          disabled={readOnly}
          label={t('establishment.fields.booking_enabled')}
          description={t('establishment.publicPage.bookingHint')}
          {...register('booking_enabled')}
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
            {...register('allowed_embed_origins')}
          />
        </Field>
        <SaveRow readOnly={readOnly} pending={s.pending} saved={s.saved} label={submitLabel} />
      </form>
      <div className="mt-6 border-t border-slate-200 pt-4">
        <EmbedSnippet slug={establishment.slug} lang={establishment.default_locale} />
      </div>
    </SectionCard>
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
    formState: { errors },
  } = form;
  const values = {
    logo_url: useWatch({ control: form.control, name: 'logo_url' }),
    primary_color: useWatch({ control: form.control, name: 'primary_color' }),
    accent_color: useWatch({ control: form.control, name: 'accent_color' }),
    font_family: useWatch({ control: form.control, name: 'font_family' }),
    radius: useWatch({ control: form.control, name: 'radius' }),
    show_prices_on_map: useWatch({ control: form.control, name: 'show_prices_on_map' }),
    hide_header_in_embed: useWatch({ control: form.control, name: 'hide_header_in_embed' }),
    intro: useWatch({ control: form.control, name: 'intro' }),
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
      <div className="flex items-center gap-2">
        <input
          type="color"
          aria-label={t('theme.picker', { field: t(`theme.fields.${name}`) })}
          disabled={readOnly}
          value={/^#[0-9a-f]{6}$/i.test(values[name]) ? values[name] : '#000000'}
          onChange={(e) =>
            setValue(name, e.target.value, { shouldDirty: true, shouldValidate: true })
          }
          className="h-10 w-12 cursor-pointer rounded border border-slate-300 bg-white p-1 disabled:cursor-not-allowed"
        />
        <Input
          {...fieldAria(`th-${name}`, errors[name])}
          disabled={readOnly}
          className="font-mono"
          maxLength={7}
          {...register(name)}
        />
      </div>
    </Field>
  );
  return (
    <SectionCard id="theme" title={t('theme.title')} description={t('theme.description')}>
      <div className="grid gap-6 lg:grid-cols-[1fr_18rem]">
        <form noValidate onSubmit={s.submit} className="space-y-4">
          <FormAlert state={s.alert} onReload={s.reload} />
          <div className="grid gap-4 sm:grid-cols-2">
            {colorField('primary_color')}
            {colorField('accent_color')}
            <Field
              id="th-font_family"
              label={t('theme.fields.font_family')}
              error={errors.font_family}
            >
              <Select
                {...fieldAria('th-font_family', errors.font_family)}
                disabled={readOnly}
                {...register('font_family')}
              >
                {FONT_FAMILIES.map((f) => (
                  <option key={f} value={f}>
                    {t(`theme.fonts.${f}`)}
                  </option>
                ))}
              </Select>
            </Field>
            <Field id="th-radius" label={t('theme.fields.radius')} error={errors.radius}>
              <Select
                {...fieldAria('th-radius', errors.radius)}
                disabled={readOnly}
                {...register('radius')}
              >
                {RADII.map((r) => (
                  <option key={r} value={r}>
                    {t(`theme.radii.${r}`)}
                  </option>
                ))}
              </Select>
            </Field>
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
          <div className="space-y-2">
            <Checkbox
              id="th-show_prices"
              disabled={readOnly}
              label={t('theme.fields.show_prices_on_map')}
              {...register('show_prices_on_map')}
            />
            <Checkbox
              id="th-hide_header"
              disabled={readOnly}
              label={t('theme.fields.hide_header_in_embed')}
              {...register('hide_header_in_embed')}
            />
          </div>
          <I18nTextInput
            name="intro"
            idPrefix="th"
            label={t('theme.fields.intro')}
            locales={locales}
            defaultLocale={establishment.default_locale}
            register={register}
            errors={errors}
            disabled={readOnly}
            multiline
          />
          <SaveRow readOnly={readOnly} pending={s.pending} saved={s.saved} label={submitLabel} />
        </form>
        <ThemePreview
          theme={previewTheme}
          name={establishment.name}
          intro={values.intro?.[establishment.default_locale]}
        />
      </div>
    </SectionCard>
  );
}

// ---------------------------------------------------------------------------

export function PricingSettingsSection({
  establishment,
  version,
  onVersion,
  readOnly,
}: SectionProps) {
  const { t, i18n } = useTranslation('configuration');
  const locale = i18n.language || 'it';
  const toValues = (e: Establishment) => toPricingSettingsValues(e, locale);
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
    formState: { errors },
  } = form;
  return (
    <SectionCard
      id="pricing-settings"
      title={t('pricingSettings.title')}
      description={t('pricingSettings.description')}
    >
      <form noValidate onSubmit={s.submit} className="space-y-4">
        <FormAlert state={s.alert} onReload={s.reload} />
        <div className="grid gap-4 sm:grid-cols-3">
          <Field
            id="ps-half_day_factor"
            label={t('pricingSettings.fields.half_day_factor')}
            error={errors.half_day_factor}
            hint={t('pricingSettings.halfDayHint')}
          >
            <Input
              {...fieldAria('ps-half_day_factor', errors.half_day_factor, true)}
              inputMode="decimal"
              disabled={readOnly}
              {...register('half_day_factor')}
            />
          </Field>
          <Field
            id="ps-rounding"
            label={t('pricingSettings.fields.rounding')}
            error={errors.rounding}
          >
            <Select
              {...fieldAria('ps-rounding', errors.rounding)}
              disabled={readOnly}
              {...register('rounding')}
            >
              {ROUNDINGS.map((r) => (
                <option key={r} value={r}>
                  {t(`pricingSettings.roundings.${r}`)}
                </option>
              ))}
            </Select>
          </Field>
          <Field
            id="ps-vat"
            label={t('pricingSettings.fields.default_vat_rate')}
            error={errors.default_vat_rate}
            hint={t('pricingSettings.vatHint')}
          >
            <div className="flex items-center gap-2">
              <Input
                {...fieldAria('ps-vat', errors.default_vat_rate, true)}
                inputMode="decimal"
                disabled={readOnly}
                {...register('default_vat_rate')}
              />
              <span aria-hidden="true" className="text-sm text-slate-600">
                %
              </span>
            </div>
          </Field>
        </div>
        <SaveRow readOnly={readOnly} pending={s.pending} saved={s.saved} />
      </form>
    </SectionCard>
  );
}
