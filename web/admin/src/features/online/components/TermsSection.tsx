import { zodResolver } from '@hookform/resolvers/zod';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@arenile/shared/ui/collapsible';
import { FieldLegend, FieldSet } from '@arenile/shared/ui/field';
import { DialogFooter } from '@arenile/shared/ui/dialog';
import { Input } from '@arenile/shared/ui/input';
import { EmptyState, FormDialog, QueryState } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Textarea } from '@arenile/shared/ui/textarea';
import { ChevronDownIcon, FileTextIcon, PlusIcon } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../../establishment';
import { BooleanField, Field } from '../../configuration/components/fields';
import { FormAlert } from '../../configuration/components/FormAlert';
import { fieldAria } from '../../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../../configuration/lib/formErrors';
import { V } from '../../configuration/lib/validation';
import { usePublishTerms, useTermsVersions, type TermsVersion } from '../api';
import {
  nextTermsVersion,
  termsSchema,
  toTermsInput,
  type TermsValues,
} from '../pages/online.schema';
import { SectionCard } from './SectionCard';

function today(): string {
  const d = new Date();
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function PublishDialog({
  open,
  onOpenChange,
  establishmentId,
  locales,
  defaultLocale,
  versions,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  establishmentId: string;
  locales: string[];
  defaultLocale: string;
  versions: TermsVersion[];
}) {
  const { t } = useTranslation('online');
  const { t: tg } = useTranslation();
  const publish = usePublishTerms(establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const current = versions.find((v) => v.current) ?? versions[0];
  const form = useForm<TermsValues>({
    resolver: zodResolver(
      termsSchema.refine((v) => !!v.texts[defaultLocale]?.terms.trim(), {
        message: V.required,
        path: ['texts', defaultLocale, 'terms'],
      }),
    ),
    defaultValues: {
      version: nextTermsVersion(
        versions.map((v) => v.version),
        today(),
      ),
      make_current: true,
      texts: Object.fromEntries(
        locales.map((l) => [
          l,
          {
            terms: current?.texts[l]?.terms ?? '',
            privacy: current?.texts[l]?.privacy ?? '',
            marketing: current?.texts[l]?.marketing ?? '',
          },
        ]),
      ),
    },
  });
  const errors = form.formState.errors;
  const submit = form.handleSubmit(async (v) => {
    setAlert(null);
    try {
      await publish.mutateAsync(toTermsInput(v));
      toast.success(t('terms.published', { version: v.version }));
      onOpenChange(false);
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(form.getValues()) }));
    }
  });
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('terms.publishTitle')}
      description={t('terms.publishDescription')}
      wide
    >
      <form noValidate onSubmit={submit} className="space-y-5">
        <FormAlert state={alert} />
        <Field
          id="tv-version"
          label={t('terms.version')}
          error={errors.version}
          hint={t('terms.versionHint')}
        >
          <Input
            {...fieldAria('tv-version', errors.version, true)}
            className="font-mono"
            {...form.register('version')}
          />
        </Field>
        {locales.map((l) => {
          const e = errors.texts?.[l];
          return (
            <FieldSet key={l} className="gap-3 rounded-lg border p-4">
              <FieldLegend variant="label">
                {tg(`common.languages.${l}`, { defaultValue: l })}
                {l === defaultLocale ? ` · ${t('terms.defaultLocale')}` : ''}
              </FieldLegend>
              <Field id={`tv-${l}-terms`} label={t('terms.terms')} error={e?.terms}>
                <Textarea
                  {...fieldAria(`tv-${l}-terms`, e?.terms)}
                  rows={6}
                  {...form.register(`texts.${l}.terms`)}
                />
              </Field>
              <Field id={`tv-${l}-privacy`} label={t('terms.privacy')} error={e?.privacy}>
                <Textarea
                  {...fieldAria(`tv-${l}-privacy`, e?.privacy)}
                  rows={4}
                  {...form.register(`texts.${l}.privacy`)}
                />
              </Field>
              <Field
                id={`tv-${l}-marketing`}
                label={t('terms.marketing')}
                error={e?.marketing}
                hint={t('terms.marketingHint')}
              >
                <Textarea
                  {...fieldAria(`tv-${l}-marketing`, e?.marketing, true)}
                  rows={2}
                  {...form.register(`texts.${l}.marketing`)}
                />
              </Field>
            </FieldSet>
          );
        })}
        <BooleanField
          control={form.control}
          name="make_current"
          id="tv-make_current"
          label={t('terms.makeCurrent')}
          description={t('terms.makeCurrentHint')}
        />
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {tg('common.actions.cancel')}
          </Button>
          <Button type="submit" disabled={publish.isPending}>
            {publish.isPending && <Spinner />}
            {t('terms.publish')}
          </Button>
        </DialogFooter>
      </form>
    </FormDialog>
  );
}

/**
 * Versioned booking terms, privacy notice and marketing consent text (spec
 * 12.3, 23): immutable versions, the current one is required by the
 * checkout and each booking stores the version accepted.
 */
export function TermsSection({
  establishmentId,
  locales,
  defaultLocale,
  readOnly,
}: {
  establishmentId: string;
  locales: string[];
  defaultLocale: string;
  readOnly: boolean;
}) {
  const { t } = useTranslation('online');
  const { t: tg } = useTranslation();
  const query = useTermsVersions(establishmentId);
  const fmt = useInstantFormat();
  const [open, setOpen] = useState(false);
  const versions = query.data ?? [];
  return (
    <SectionCard
      id="ol-terms"
      icon={FileTextIcon}
      title={t('terms.title')}
      description={t('terms.description')}
      actions={
        !readOnly && query.data ? (
          <Button onClick={() => setOpen(true)}>
            <PlusIcon aria-hidden="true" />
            {t('terms.new')}
          </Button>
        ) : undefined
      }
    >
      {!query.data ? (
        <QueryState
          isLoading={query.isPending}
          error={query.error}
          onRetry={() => void query.refetch()}
        />
      ) : versions.length === 0 ? (
        <EmptyState
          icon={FileTextIcon}
          title={t('terms.empty')}
          description={t('terms.emptyHint')}
        />
      ) : (
        <ul className="divide-y rounded-lg border">
          {versions.map((v) => (
            <li key={v.id}>
              <Collapsible>
                <div className="flex flex-wrap items-center gap-3 p-3">
                  <span className="font-mono text-sm font-medium">{v.version}</span>
                  {v.current && <Badge>{t('terms.current')}</Badge>}
                  <span className="text-sm text-muted-foreground">
                    {t('terms.publishedAt', { date: fmt.dateTime(v.published_at) })}
                  </span>
                  <span className="text-sm text-muted-foreground">
                    {Object.keys(v.texts)
                      .map((l) => tg(`common.languages.${l}`, { defaultValue: l }))
                      .join(', ')}
                  </span>
                  <CollapsibleTrigger asChild>
                    <Button variant="ghost" size="sm" className="ml-auto">
                      <ChevronDownIcon aria-hidden="true" />
                      {t('terms.show')}
                    </Button>
                  </CollapsibleTrigger>
                </div>
                <CollapsibleContent className="px-3 pb-3">
                  <p className="max-h-60 overflow-y-auto rounded-md bg-muted/40 p-3 text-sm whitespace-pre-line">
                    {(v.texts[defaultLocale] ?? Object.values(v.texts)[0])?.terms}
                  </p>
                </CollapsibleContent>
              </Collapsible>
            </li>
          ))}
        </ul>
      )}
      {open && (
        <PublishDialog
          open={open}
          onOpenChange={setOpen}
          establishmentId={establishmentId}
          locales={locales}
          defaultLocale={defaultLocale}
          versions={versions}
        />
      )}
    </SectionCard>
  );
}
