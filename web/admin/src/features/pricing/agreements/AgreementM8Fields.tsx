import {
  Button,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@arenile/shared/ui';
import { MoneyInput, MultiSelect, OptionSelect, type Option } from '@arenile/shared/ui/patterns';
import { CopyIcon, PlusIcon, Trash2Icon } from 'lucide-react';
import type { ReactNode } from 'react';
import { useFieldArray, useWatch, type Control, type Path } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { seasons as seasonsResource } from '../../configuration/api/resources';
import { customerCategories, partners } from '../../partners/api';
import type { Agreement } from '../api';
import { EditorSection, MultiToggle, SingleToggle, SwitchRow, TextRow } from '../kit';
import type { PricingTabProps } from '../RulesTab';
import type { AgreementValues } from '../rules.schema';
import { AGREEMENT_CHANNELS, REGISTER_CATEGORIES, TARGETS } from './agreement.schema';

type C = Control<AgreementValues>;

function Row({
  control,
  name,
  label,
  description,
  className,
  children,
}: {
  control: C;
  name: Path<AgreementValues>;
  label: ReactNode;
  description?: ReactNode;
  className?: string;
  children: (field: { value: unknown; onChange: (v: unknown) => void }) => ReactNode;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem className={className}>
          <FormLabel>{label}</FormLabel>
          <FormControl>{children(field)}</FormControl>
          {description && <FormDescription>{description}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

/**
 * M8 sections of the agreement editor (spec 30.2.2): recipients, fixed price
 * per pack, scope and seasons, quota, requirement and channels with the
 * agreement link. Rendered after the economic condition in Prezzi → Sconti.
 */
export function AgreementM8Fields({
  control,
  item,
  ...p
}: Pick<PricingTabProps, 'establishmentId' | 'packs' | 'extras' | 'zones' | 'name'> & {
  control: C;
  item?: Agreement;
}) {
  const { t } = useTranslation('partners');
  const partnerList = partners.useList(p.establishmentId);
  const categoryList = customerCategories.useList(p.establishmentId);
  const seasonList = seasonsResource.useList(p.establishmentId);
  const target = useWatch({ control, name: 'target' });
  const reserved = useWatch({ control, name: 'quota_reserved' });
  const prices = useFieldArray({ control, name: 'pack_prices' });

  const packOptions: Option[] = p.packs.map((x) => ({
    value: x.id,
    label: p.name(x.name) || x.code,
  }));
  const extraOptions: Option[] = p.extras.map((x) => ({
    value: x.id,
    label: p.name(x.name) || x.code,
  }));
  const zoneOptions: Option[] = p.zones.map((z) => ({ value: z.id, label: z.name }));
  const seasonOptions: Option[] = [...(seasonList.data ?? [])]
    .sort((a, b) => b.year - a.year)
    .map((s) => ({ value: s.id, label: String(s.year) }));
  const partnerOptions: Option[] = (partnerList.data ?? [])
    .filter((x) => x.active || x.id === item?.partner_id)
    .map((x) => ({ value: x.id, label: x.name }));
  const categoryOptions: Option[] = (categoryList.data ?? [])
    .filter((x) => x.active || x.id === item?.customer_category_id)
    .map((x) => ({ value: x.id, label: x.name }));
  const numeric = { inputMode: 'numeric' as const, className: 'max-w-32' };

  return (
    <>
      <EditorSection
        title={t('agreementForm.recipients')}
        description={t('agreementForm.recipientsHelp')}
      >
        <Row control={control} name="target" label={t('agreementForm.target')}>
          {(f) => (
            <SingleToggle
              value={f.value as string}
              onChange={f.onChange}
              label={t('agreementForm.target')}
              options={TARGETS.map((x) => ({ value: x, label: t(`targets.${x}`) }))}
            />
          )}
        </Row>
        <div className="grid gap-4 sm:grid-cols-2">
          {target === 'partner' && (
            <Row control={control} name="partner_id" label={t('agreementForm.partner')}>
              {(f) => (
                <OptionSelect
                  value={f.value as string}
                  onChange={f.onChange}
                  placeholder={
                    partnerOptions.length ? t('agreementForm.pick') : t('agreementForm.noPartners')
                  }
                  options={partnerOptions}
                />
              )}
            </Row>
          )}
          {target === 'customer_category' && (
            <Row control={control} name="customer_category_id" label={t('agreementForm.category')}>
              {(f) => (
                <OptionSelect
                  value={f.value as string}
                  onChange={f.onChange}
                  placeholder={
                    categoryOptions.length
                      ? t('agreementForm.pick')
                      : t('agreementForm.noCategories')
                  }
                  options={categoryOptions}
                />
              )}
            </Row>
          )}
          <Row
            control={control}
            name="register_category"
            label={t('agreementForm.registerCategory')}
            description={t('agreementForm.registerCategoryHelp')}
          >
            {(f) => (
              <OptionSelect
                value={f.value as string}
                onChange={f.onChange}
                options={REGISTER_CATEGORIES.map((r) => ({ value: r, label: t(`register.${r}`) }))}
              />
            )}
          </Row>
        </div>
      </EditorSection>

      <EditorSection
        title={t('agreementForm.packPrices')}
        description={t('agreementForm.packPricesHelp')}
      >
        {prices.fields.length > 0 && (
          <ul className="space-y-3">
            {prices.fields.map((row, i) => (
              <li key={row.id} className="flex flex-wrap items-start gap-2">
                <Row
                  control={control}
                  name={`pack_prices.${i}.pack_id`}
                  label={<span className="sr-only">{t('agreementForm.pack')}</span>}
                  className="min-w-0 flex-1 basis-48"
                >
                  {(f) => (
                    <OptionSelect
                      value={f.value as string}
                      onChange={f.onChange}
                      placeholder={t('agreementForm.pack')}
                      options={packOptions}
                    />
                  )}
                </Row>
                <Row
                  control={control}
                  name={`pack_prices.${i}.amount`}
                  label={<span className="sr-only">{t('agreementForm.packAmount')}</span>}
                  className="w-36"
                >
                  {(f) => (
                    <MoneyInput
                      value={f.value as number | null}
                      onChange={(c) => f.onChange(c)}
                      aria-label={t('agreementForm.packAmount')}
                    />
                  )}
                </Row>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="mt-2"
                  aria-label={t('agreementForm.removePackPrice')}
                  onClick={() => prices.remove(i)}
                >
                  <Trash2Icon aria-hidden="true" />
                </Button>
              </li>
            ))}
          </ul>
        )}
        <Button
          variant="outline"
          onClick={() => prices.append({ pack_id: '', amount: null })}
          disabled={packOptions.length === 0}
        >
          <PlusIcon aria-hidden="true" />
          {t('agreementForm.addPackPrice')}
        </Button>
      </EditorSection>

      <EditorSection title={t('agreementForm.scope')} description={t('agreementForm.scopeHelp')}>
        <div className="grid gap-4 sm:grid-cols-2">
          <Row control={control} name="scope_pack_ids" label={t('agreementForm.scopePacks')}>
            {(f) => (
              <MultiSelect
                value={f.value as string[]}
                onChange={f.onChange}
                options={packOptions}
                placeholder={t('agreementForm.all')}
              />
            )}
          </Row>
          <Row control={control} name="season_ids" label={t('agreementForm.seasons')}>
            {(f) => (
              <MultiSelect
                value={f.value as string[]}
                onChange={f.onChange}
                options={seasonOptions}
                placeholder={t('agreementForm.all')}
              />
            )}
          </Row>
          <Row control={control} name="scope_extra_ids" label={t('agreementForm.scopeExtras')}>
            {(f) => (
              <MultiSelect
                value={f.value as string[]}
                onChange={f.onChange}
                options={extraOptions}
                placeholder={t('agreementForm.allExtras')}
              />
            )}
          </Row>
        </div>
        <SwitchRow
          control={control}
          name="apply_to_extras"
          label={t('agreementForm.applyToExtras')}
          description={t('agreementForm.applyToExtrasHelp')}
        />
      </EditorSection>

      <EditorSection title={t('agreementForm.quota')} description={t('agreementForm.quotaHelp')}>
        <div className="grid gap-4 sm:grid-cols-3">
          <TextRow
            control={control}
            name="quota_max"
            label={t('agreementForm.quotaMax')}
            inputProps={numeric}
          />
          <TextRow
            control={control}
            name="quota_reserved"
            label={t('agreementForm.quotaReserved')}
            inputProps={numeric}
          />
          {!!reserved && (
            <TextRow
              control={control}
              name="quota_release_days"
              label={t('agreementForm.quotaRelease')}
              description={t('agreementForm.quotaReleaseHelp')}
              inputProps={{ ...numeric, placeholder: '3' }}
            />
          )}
        </div>
        <Row control={control} name="quota_zone_ids" label={t('agreementForm.quotaZones')}>
          {(f) => (
            <MultiSelect
              value={f.value as string[]}
              onChange={f.onChange}
              options={zoneOptions}
              placeholder={t('agreementForm.allZones')}
            />
          )}
        </Row>
      </EditorSection>

      <EditorSection title={t('agreementForm.desk')}>
        <TextRow
          control={control}
          name="requirement"
          label={t('agreementForm.requirement')}
          description={t('agreementForm.requirementHelp')}
        />
        <Row
          control={control}
          name="agreement_channels"
          label={t('agreementForm.channels')}
          description={t('agreementForm.channelsHelp')}
        >
          {(f) => (
            <MultiToggle
              value={f.value as string[]}
              onChange={f.onChange}
              label={t('agreementForm.channels')}
              options={AGREEMENT_CHANNELS.map((c) => ({ value: c, label: t(`channels.${c}`) }))}
            />
          )}
        </Row>
        {item?.public_link && <PublicLink path={item.public_link} />}
      </EditorSection>
    </>
  );
}

/** The agreement link the partner publishes or sends to its guests (`/b/{slug}?c={code}`). */
function PublicLink({ path }: { path: string }) {
  const { t } = useTranslation('partners');
  const url = path.startsWith('http') ? path : `${window.location.origin}${path}`;
  return (
    <div className="space-y-1.5 rounded-lg border p-3">
      <p className="text-sm font-medium">{t('agreementForm.link')}</p>
      <div className="flex flex-wrap items-center gap-2">
        <code className="min-w-0 flex-1 text-xs break-all">{url}</code>
        <Button
          variant="outline"
          size="sm"
          onClick={() =>
            void navigator.clipboard
              ?.writeText(url)
              .then(() => toast.success(t('agreementForm.linkCopied')))
          }
        >
          <CopyIcon aria-hidden="true" />
          {t('agreementForm.copy')}
        </Button>
      </div>
    </div>
  );
}
