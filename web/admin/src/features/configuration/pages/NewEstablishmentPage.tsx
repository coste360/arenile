import { useEstablishments, useMe } from '@arenile/shared/api';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardFooter } from '@arenile/shared/ui/card';
import { FieldDescription, FieldSeparator } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@arenile/shared/ui/input-group';
import { Spinner } from '@arenile/shared/ui/spinner';
import { EmptyState } from '@arenile/shared/ui/patterns';
import { LockIcon } from 'lucide-react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useNavigate } from '@tanstack/react-router';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useCreateEstablishment } from '../api/establishment';
import { ConfigPage, QueryState } from '../components/ConfigPage';
import { Field, MultiCheck, SelectField } from '../components/fields';
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
        <EmptyState icon={LockIcon} title={t('new.ownerOnly')} />
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

  const languageOptions = SUPPORTED_LOCALES.map((l) => ({
    value: l,
    label: t(`common.languages.${l}`, { ns: 'translation' }),
  }));

  return (
    <form noValidate onSubmit={onSubmit} className="max-w-3xl">
      <Card>
        <CardContent className="space-y-6">
          <FormAlert state={alert} />
          {orgs.length > 1 && (
            <SelectField
              control={form.control}
              name="organization_id"
              id="new-org"
              label={t('new.fields.organization')}
              options={orgs.map((o, i) => ({ value: o, label: orgLabel(o, i) }))}
            />
          )}
          <div className="grid gap-5 sm:grid-cols-2">
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
              <InputGroup>
                <InputGroupAddon>
                  <span className="font-mono text-xs text-muted-foreground">/b/</span>
                </InputGroupAddon>
                <InputGroupInput
                  {...fieldAria('new-slug', errors.slug, true)}
                  {...slugField}
                  onChange={(e) => {
                    setSlugTouched(true);
                    void slugField.onChange(e);
                  }}
                />
              </InputGroup>
            </Field>
            <SelectField
              control={form.control}
              name="timezone"
              id="new-timezone"
              label={t('establishment.fields.timezone')}
              options={timezoneOptions('Europe/Rome').map((tz) => ({ value: tz, label: tz }))}
            />
            <SelectField
              control={form.control}
              name="default_locale"
              id="new-default_locale"
              label={t('establishment.fields.default_locale')}
              options={languageOptions}
            />
          </div>
          <MultiCheck
            control={form.control}
            name="enabled_locales"
            id="new-enabled_locales"
            legend={t('establishment.fields.enabled_locales')}
            options={languageOptions}
          />
          <FieldSeparator className="my-3 *:data-[slot=field-separator-content]:bg-card">
            {t('establishment.general.contacts')}
          </FieldSeparator>
          <div className="grid gap-5 sm:grid-cols-2">
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
        </CardContent>
        <CardFooter className="flex-wrap justify-between gap-3 border-t">
          <FieldDescription>{t('new.afterCreate')}</FieldDescription>
          <Button type="submit" disabled={create.isPending}>
            {create.isPending && <Spinner />}
            {t('new.submit')}
          </Button>
        </CardFooter>
      </Card>
    </form>
  );
}
