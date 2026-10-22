import { useEstablishments, useMe } from '@arenile/shared/api';
import { Button, Card, CardContent, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useNavigate } from '@tanstack/react-router';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useCreateEstablishment } from '../api/establishment';
import { ConfigPage, QueryState } from '../components/ConfigPage';
import { Checkbox, CheckboxGroup, Field, Select } from '../components/fields';
import { FormAlert } from '../components/FormAlert';
import { fieldAria } from '../lib/aria';
import { problemToFormState, type FormAlertState } from '../lib/formErrors';
import { ownedOrganizations } from '../lib/permissions';
import { slugify } from '../lib/publicPage';
import { timezoneOptions } from '../lib/timezones';
import { SUPPORTED_LOCALES } from './establishment.schema';
import {
  newEstablishmentDefaults,
  newEstablishmentSchema,
  toEstablishmentCreate,
} from './newEstablishment.schema';

/** Creation of another establishment by an owner (spec 28 M1, multi-establishment). */
export function NewEstablishmentPage() {
  const { t } = useTranslation('configuration');
  const me = useMe();
  const establishments = useEstablishments({ limit: 200 });
  const orgs = ownedOrganizations(me.data);

  if (me.isPending) {
    return (
      <ConfigPage title={t('new.title')}>
        <QueryState isLoading error={null} />
      </ConfigPage>
    );
  }
  if (orgs.length === 0) {
    return (
      <ConfigPage title={t('new.title')}>
        <p role="alert" className="text-sm text-slate-700">
          {t('new.ownerOnly')}
        </p>
      </ConfigPage>
    );
  }
  const orgLabel = (orgId: string, index: number) => {
    const names = (establishments.data?.items ?? [])
      .filter((e) => e.organization_id === orgId)
      .map((e) => e.name);
    return names.length
      ? t('new.organizationWith', { index: index + 1, names: names.join(', ') })
      : t('new.organization', { index: index + 1 });
  };
  return (
    <ConfigPage title={t('new.title')} description={t('new.description')}>
      <NewEstablishmentForm orgs={orgs} orgLabel={orgLabel} />
    </ConfigPage>
  );
}

function NewEstablishmentForm({
  orgs,
  orgLabel,
}: {
  orgs: string[];
  orgLabel: (id: string, index: number) => string;
}) {
  const { t } = useTranslation('configuration');
  const navigate = useNavigate();
  const create = useCreateEstablishment();
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const [slugTouched, setSlugTouched] = useState(false);
  const form = useForm({
    resolver: zodResolver(newEstablishmentSchema),
    defaultValues: newEstablishmentDefaults(orgs[0] ?? ''),
  });
  const {
    register,
    setValue,
    formState: { errors },
  } = form;
  const nameField = register('name');
  const slugField = register('slug');

  const onSubmit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      const est = await create.mutateAsync(toEstablishmentCreate(values));
      void navigate({ to: '/e/$establishmentId/setup', params: { establishmentId: est.id } });
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(values) }));
    }
  });

  return (
    <Card>
      <CardContent className="pt-5">
        <form noValidate onSubmit={onSubmit} className="space-y-4">
          <FormAlert state={alert} />
          {orgs.length > 1 && (
            <Field id="new-org" label={t('new.fields.organization')} error={errors.organization_id}>
              <Select
                {...fieldAria('new-org', errors.organization_id)}
                {...register('organization_id')}
              >
                {orgs.map((o, i) => (
                  <option key={o} value={o}>
                    {orgLabel(o, i)}
                  </option>
                ))}
              </Select>
            </Field>
          )}
          <div className="grid gap-4 sm:grid-cols-2">
            <Field id="new-name" label={t('establishment.fields.name')} error={errors.name}>
              <Input
                {...fieldAria('new-name', errors.name)}
                autoFocus
                {...nameField}
                onChange={(e) => {
                  void nameField.onChange(e);
                  if (!slugTouched) setValue('slug', slugify(e.target.value));
                }}
              />
            </Field>
            <Field
              id="new-slug"
              label={t('establishment.fields.slug')}
              error={errors.slug}
              hint={t('establishment.general.slugHint')}
            >
              <Input
                {...fieldAria('new-slug', errors.slug, true)}
                {...slugField}
                onChange={(e) => {
                  setSlugTouched(true);
                  void slugField.onChange(e);
                }}
              />
            </Field>
            <Field
              id="new-timezone"
              label={t('establishment.fields.timezone')}
              error={errors.timezone}
            >
              <Select {...fieldAria('new-timezone', errors.timezone)} {...register('timezone')}>
                {timezoneOptions('Europe/Rome').map((tz) => (
                  <option key={tz} value={tz}>
                    {tz}
                  </option>
                ))}
              </Select>
            </Field>
            <Field
              id="new-default_locale"
              label={t('establishment.fields.default_locale')}
              error={errors.default_locale}
            >
              <Select
                {...fieldAria('new-default_locale', errors.default_locale)}
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
            id="new-enabled_locales"
            legend={t('establishment.fields.enabled_locales')}
            error={errors.enabled_locales as never}
          >
            {SUPPORTED_LOCALES.map((l) => (
              <Checkbox
                key={l}
                id={`new-locale-${l}`}
                value={l}
                label={t(`common.languages.${l}`, { ns: 'translation' })}
                {...register('enabled_locales')}
              />
            ))}
          </CheckboxGroup>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              id="new-contact_email"
              label={t('establishment.fields.contact_email')}
              error={errors.contact_email}
            >
              <Input
                {...fieldAria('new-contact_email', errors.contact_email)}
                type="email"
                {...register('contact_email')}
              />
            </Field>
            <Field
              id="new-contact_phone"
              label={t('establishment.fields.contact_phone')}
              error={errors.contact_phone}
            >
              <Input
                {...fieldAria('new-contact_phone', errors.contact_phone)}
                type="tel"
                {...register('contact_phone')}
              />
            </Field>
          </div>
          <p className="text-sm text-slate-600">{t('new.afterCreate')}</p>
          <Button type="submit" disabled={create.isPending}>
            {create.isPending ? t('common.actions.saving', { ns: 'translation' }) : t('new.submit')}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
