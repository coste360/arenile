import { Button, Card, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm, useWatch, type Control } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import type { Extra, Pack, RatePeriod } from '../configuration/api/resources';
import { useCrudSubmit, useEditor } from '../configuration/api/useCrud';
import { Badge, EmptyState, QueryState } from '../configuration/components/ConfigPage';
import { ConfirmDelete, FormActions, FormDialog } from '../configuration/components/dialogs';
import { Checkbox, Field, Select } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { MultiCheck } from '../configuration/components/MultiCheck';
import { fieldAria } from '../configuration/lib/aria';
import { priceRules, type PriceRule } from './api';
import { ActionFields, ConditionFields } from './components';
import { describeRule, type SummaryNames } from './format';
import { sortRules, useSummaryNames } from './ruleNames';
import {
  ACTIONS_BY_STAGE,
  FORM_ALIASES,
  STAGES,
  ruleSchema,
  ruleValues,
  toRuleInput,
  type RuleValues,
} from './rules.schema';

export interface PricingTabProps {
  establishmentId: string;
  seasonId: string;
  readOnly: boolean;
  periods: RatePeriod[];
  packs: Pack[];
  extras: Extra[];
  zones: { id: string; name: string }[];
  name: (text: Record<string, string>) => string;
  locale: string;
}

/** Price rules of the season grouped by stage (base → modifier → discount), with create/edit/delete. */
export function RulesTab(p: PricingTabProps) {
  const { t } = useTranslation('pricing');
  const list = priceRules.useList(p.establishmentId, { season_id: p.seasonId });
  const remove = priceRules.useRemove(p.establishmentId);
  const editor = useEditor<PriceRule>();
  const summary = useSummaryNames(p);
  const items = list.data ?? [];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-slate-600">{t('rules.description')}</p>
        {!p.readOnly && <Button onClick={editor.create}>{t('rules.add')}</Button>}
      </div>
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && <EmptyState>{t('rules.empty')}</EmptyState>}
      {items.length > 0 &&
        STAGES.map((stage) => {
          const group = sortRules(items.filter((r) => r.stage === stage));
          return (
            <section key={stage} aria-labelledby={`rules-stage-${stage}`} className="space-y-2">
              <h3
                id={`rules-stage-${stage}`}
                className="flex items-center gap-2 text-sm font-semibold"
              >
                <Badge tone={STAGE_TONE[stage]}>{t(`stages.${stage}`)}</Badge>
                <span className="font-normal text-slate-600">{t(`stageHints.${stage}`)}</span>
              </h3>
              {group.length === 0 ? (
                <p className="text-sm text-slate-500">{t('rules.emptyStage')}</p>
              ) : (
                <Card className="overflow-x-auto">
                  <table className="w-full min-w-[44rem] text-sm">
                    <caption className="sr-only">
                      {t('rules.stageCaption', { stage: t(`stages.${stage}`) })}
                    </caption>
                    <thead className="bg-slate-50 text-left text-slate-600">
                      <tr>
                        <th scope="col" className="w-16 px-3 py-2 text-right font-medium">
                          {t('fields.priority')}
                        </th>
                        <th scope="col" className="px-3 py-2 font-medium">
                          {t('fields.name')}
                        </th>
                        <th scope="col" className="px-3 py-2 font-medium">
                          {t('rules.summary')}
                        </th>
                        <th scope="col" className="px-3 py-2">
                          <span className="sr-only">{t('actionsColumn')}</span>
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {group.map((r) => (
                        <tr
                          key={r.id}
                          className={`border-t border-slate-100 align-top ${r.active ? '' : 'text-slate-500'}`}
                        >
                          <td className="px-3 py-2 text-right tabular-nums">{r.priority}</td>
                          <th scope="row" className="px-3 py-2 text-left font-medium">
                            {r.name}
                            {!r.active && (
                              <span className="ml-2">
                                <Badge>{t('inactive')}</Badge>
                              </span>
                            )}
                            {r.stage !== 'base' && r.stackable && (
                              <span className="ml-2">
                                <Badge tone="green">{t('fields.stackable')}</Badge>
                              </span>
                            )}
                          </th>
                          <td className="px-3 py-2 text-slate-700">
                            {describeRule(t, r, summary.names(r.target_type), p.locale)}
                          </td>
                          <td className="px-3 py-2 text-right">
                            {!p.readOnly && (
                              <div className="flex justify-end gap-1">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => editor.edit(r)}
                                  aria-label={t('rules.editAria', { name: r.name })}
                                >
                                  {t('common.actions.edit', { ns: 'translation' })}
                                </Button>
                                <ConfirmDelete
                                  label={t('rules.deleteAria', { name: r.name })}
                                  message={t('rules.deleteConfirm', { name: r.name })}
                                  onConfirm={() =>
                                    remove.mutateAsync({ id: r.id, version: r.version })
                                  }
                                />
                              </div>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </Card>
              )}
            </section>
          );
        })}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={editor.item ? t('rules.editTitle', { name: editor.item.name }) : t('rules.add')}
      >
        <RuleForm key={editor.key} {...p} item={editor.item} onDone={editor.close} />
      </FormDialog>
    </div>
  );
}

const STAGE_TONE = { base: 'blue', modifier: 'amber', discount: 'green' } as const;

/**
 * Rule editor. `initial` prefills a new rule (e.g. from a coverage gap);
 * it is ignored when editing an existing one.
 */
export function RuleForm(
  p: PricingTabProps & { item?: PriceRule; initial?: Partial<RuleValues>; onDone: () => void },
) {
  const { t } = useTranslation('pricing');
  const form = useForm<RuleValues>({
    resolver: zodResolver(ruleSchema),
    defaultValues: { ...ruleValues(p.item, p.locale), ...(p.item ? {} : p.initial) },
  });
  const summary = useSummaryNames(p);
  const create = priceRules.useCreate(p.establishmentId);
  const update = priceRules.useUpdate(p.establishmentId);
  const s = useCrudSubmit({
    form,
    item: p.item,
    create,
    update,
    toInput: (v: RuleValues) => toRuleInput(p.seasonId, v),
    onDone: p.onDone,
    aliases: FORM_ALIASES,
  });
  const {
    register,
    control,
    setValue,
    formState: { errors },
  } = form;
  const stage = useWatch({ control, name: 'stage' });
  const targetType = useWatch({ control, name: 'target_type' });
  const targets = targetType === 'pack' ? p.packs : p.extras;
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={p.onDone} />
      <div className="grid gap-4 sm:grid-cols-3">
        <Field
          id="rule-name"
          label={t('fields.name')}
          error={errors.name}
          className="sm:col-span-2"
        >
          <Input {...fieldAria('rule-name', errors.name)} {...register('name')} />
        </Field>
        <Field
          id="rule-priority"
          label={t('fields.priority')}
          error={errors.priority}
          hint={t('hints.priority')}
        >
          <Input
            {...fieldAria('rule-priority', errors.priority, true)}
            inputMode="numeric"
            {...register('priority')}
          />
        </Field>
        <Field id="rule-stage" label={t('fields.stage')} error={errors.stage}>
          <Select
            {...fieldAria('rule-stage', errors.stage)}
            {...register('stage', {
              onChange: (e: { target: { value: keyof typeof ACTIONS_BY_STAGE } }) =>
                setValue('action_type', ACTIONS_BY_STAGE[e.target.value][0] ?? 'set'),
            })}
          >
            {STAGES.map((st) => (
              <option key={st} value={st}>
                {t(`stages.${st}`)}
              </option>
            ))}
          </Select>
        </Field>
        <Field id="rule-target_type" label={t('fields.target_type')} error={errors.target_type}>
          <Select
            {...fieldAria('rule-target_type', errors.target_type)}
            {...register('target_type', { onChange: () => setValue('target_ids', []) })}
          >
            <option value="pack">{t('targets.pack')}</option>
            <option value="extra">{t('targets.extra')}</option>
          </Select>
        </Field>
      </div>
      {targets.length > 0 && (
        <MultiCheck
          control={control}
          name="target_ids"
          id="rule-target_ids"
          legend={t('fields.targets')}
          hint={t('hints.targets')}
          options={targets.map((x) => ({ value: x.id, label: p.name(x.name) || x.code }))}
        />
      )}
      <ActionFields
        control={control}
        register={register}
        errors={errors}
        allowed={ACTIONS_BY_STAGE[stage]}
        idPrefix="rule"
      />
      <ConditionFields
        control={control}
        register={register}
        errors={errors}
        periods={p.periods}
        zones={p.zones}
        agreements={summary.agreements}
        idPrefix="rule"
      />
      <div className="flex flex-wrap gap-6">
        {stage !== 'base' && (
          <Checkbox
            id="rule-stackable"
            label={t('fields.stackable')}
            description={t('hints.stackable')}
            {...register('stackable')}
          />
        )}
        <Checkbox id="rule-active" label={t('fields.active')} {...register('active')} />
      </div>
      <RulePreview
        control={control}
        seasonId={p.seasonId}
        names={summary.names}
        locale={p.locale}
      />
      <FormActions onCancel={p.onDone} pending={s.pending} />
    </form>
  );
}

/** Live sentence of the rule being edited; shows nothing while the form is invalid. */
function RulePreview({
  control,
  seasonId,
  names,
  locale,
}: {
  control: Control<RuleValues>;
  seasonId: string;
  names: (targetType: 'pack' | 'extra') => SummaryNames;
  locale: string;
}) {
  const { t } = useTranslation('pricing');
  const values = useWatch({ control }) as RuleValues;
  const parsed = ruleSchema.safeParse(values);
  return (
    <p
      aria-live="polite"
      className="rounded-md bg-slate-50 px-3 py-2 text-sm text-slate-700"
      data-testid="rule-preview"
    >
      <span className="font-medium">{t('rules.summary')}: </span>
      {parsed.success
        ? describeRule(
            t,
            toRuleInput(seasonId, parsed.data),
            names(parsed.data.target_type),
            locale,
          )
        : t('rules.previewIncomplete')}
    </p>
  );
}
