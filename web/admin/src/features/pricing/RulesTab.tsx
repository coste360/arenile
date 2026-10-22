import {
  Badge,
  Button,
  Card,
  CardContent,
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@arenile/shared/ui';
import {
  type ColumnDef,
  DataTable,
  DataTableColumnHeader,
  DeleteButton,
  EditButton,
  EmptyState,
  FormSheet,
  MultiSelect,
  OptionSelect,
  QueryState,
  RowActions,
  SheetForm,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { ListOrderedIcon, PlusIcon, SparklesIcon } from 'lucide-react';
import { useForm, useWatch, type Control } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import type { Extra, Pack, RatePeriod } from '../configuration/api/resources';
import { useCrudSubmit, useEditor } from '../configuration/api/useCrud';
import { priceRules, type PriceRule } from './api';
import { ActionFields, ConditionFields } from './components';
import { describeRule, type SummaryNames } from './format';
import { EditorSection, FormErrorAlert, SwitchRow, TextRow } from './kit';
import { STAGE_TONE, sortRules, useSummaryNames } from './ruleNames';
import {
  ACTIONS_BY_STAGE,
  FORM_ALIASES,
  STAGES,
  ruleSchema,
  ruleValues,
  toRuleInput,
  type RuleValues,
  type Stage,
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

  const columns: ColumnDef<PriceRule>[] = [
    {
      accessorKey: 'priority',
      meta: { label: t('fields.priority') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('fields.priority')} />
      ),
      cell: ({ row }) => <span className="tabular-nums">{row.original.priority}</span>,
    },
    {
      accessorKey: 'name',
      meta: { label: t('fields.name') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.name')} />,
      cell: ({ row }) => {
        const r = row.original;
        return (
          <div className="flex flex-wrap items-center gap-1.5">
            <span className={r.active ? 'font-medium' : 'font-medium text-muted-foreground'}>
              {r.name}
            </span>
            {!r.active && <Badge variant="outline">{t('inactive')}</Badge>}
            {r.stage !== 'base' && r.stackable && (
              <Badge variant="secondary">{t('fields.stackable')}</Badge>
            )}
          </div>
        );
      },
    },
    {
      id: 'summary',
      enableSorting: false,
      meta: { label: t('rules.summary') },
      header: t('rules.summary'),
      cell: ({ row }) => (
        <p className="max-w-xl text-sm whitespace-normal text-muted-foreground">
          {describeRule(t, row.original, summary.names(row.original.target_type), p.locale)}
        </p>
      ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actionsColumn')}</span>,
      cell: ({ row }) =>
        p.readOnly ? null : (
          <RowActions>
            <EditButton
              label={t('rules.editAria', { name: row.original.name })}
              onClick={() => editor.edit(row.original)}
            />
            <DeleteButton
              label={t('rules.deleteAria', { name: row.original.name })}
              message={t('rules.deleteConfirm', { name: row.original.name })}
              onConfirm={() =>
                remove.mutateAsync({ id: row.original.id, version: row.original.version })
              }
            />
          </RowActions>
        ),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">{t('rules.description')}</p>
        {!p.readOnly && (
          <Button onClick={editor.create}>
            <PlusIcon aria-hidden="true" />
            {t('rules.add')}
          </Button>
        )}
      </div>
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && (
        <EmptyState
          icon={ListOrderedIcon}
          title={t('rules.empty')}
          action={
            !p.readOnly && (
              <Button onClick={editor.create}>
                <PlusIcon aria-hidden="true" />
                {t('rules.add')}
              </Button>
            )
          }
        />
      )}
      {items.length > 0 &&
        STAGES.map((stage) => {
          const group = sortRules(items.filter((r) => r.stage === stage));
          return (
            <section key={stage} aria-labelledby={`rules-stage-${stage}`} className="space-y-3">
              <h3 id={`rules-stage-${stage}`} className="flex flex-wrap items-center gap-2">
                <StatusBadge tone={STAGE_TONE[stage]}>{t(`stages.${stage}`)}</StatusBadge>
                <span className="text-sm text-muted-foreground">{t(`stageHints.${stage}`)}</span>
                <Badge variant="outline" className="ml-auto tabular-nums">
                  {group.length}
                </Badge>
              </h3>
              {group.length === 0 ? (
                <p className="rounded-lg border border-dashed px-4 py-3 text-sm text-muted-foreground">
                  {t('rules.emptyStage')}
                </p>
              ) : (
                <DataTable
                  caption={t('rules.stageCaption', { stage: t(`stages.${stage}`) })}
                  columns={columns}
                  data={group}
                  getRowId={(r) => r.id}
                  pageSize={0}
                  columnToggle={false}
                />
              )}
            </section>
          );
        })}
      <FormSheet
        size="lg"
        padded={false}
        open={editor.open}
        onOpenChange={editor.setOpen}
        title={editor.item ? t('rules.editTitle', { name: editor.item.name }) : t('rules.add')}
      >
        <RuleForm key={editor.key} {...p} item={editor.item} onDone={editor.close} />
      </FormSheet>
    </div>
  );
}

/**
 * Rule editor (render inside an EditorSheet). `initial` prefills a new rule
 * (e.g. from a coverage gap); it is ignored when editing an existing one.
 */
export function RuleForm(
  p: PricingTabProps & { item?: PriceRule; initial?: Partial<RuleValues>; onDone: () => void },
) {
  const { t } = useTranslation('pricing');
  const form = useForm<RuleValues>({
    resolver: zodResolver(ruleSchema),
    defaultValues: { ...ruleValues(p.item), ...(p.item ? {} : p.initial) },
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
  const { control, setValue } = form;
  const stage = useWatch({ control, name: 'stage' });
  const targetType = useWatch({ control, name: 'target_type' });
  const targets = targetType === 'pack' ? p.packs : p.extras;
  return (
    <Form {...form}>
      <SheetForm onSubmit={s.submit} onCancel={p.onDone} pending={s.pending}>
        <FormErrorAlert state={s.alert} onReload={p.onDone} />
        <EditorSection title={t('sections.general')}>
          <div className="grid gap-4 sm:grid-cols-3">
            <TextRow
              control={control}
              name="name"
              label={t('fields.name')}
              className="sm:col-span-2"
            />
            <TextRow
              control={control}
              name="priority"
              label={t('fields.priority')}
              description={t('hints.priority')}
              inputProps={{ inputMode: 'numeric' }}
            />
            <FormField
              control={control}
              name="stage"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('fields.stage')}</FormLabel>
                  <FormControl>
                    <OptionSelect
                      value={field.value}
                      onChange={(v) => {
                        field.onChange(v);
                        setValue('action_type', ACTIONS_BY_STAGE[v as Stage][0] ?? 'set');
                      }}
                      options={STAGES.map((st) => ({ value: st, label: t(`stages.${st}`) }))}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={control}
              name="target_type"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('fields.target_type')}</FormLabel>
                  <FormControl>
                    <OptionSelect
                      value={field.value}
                      onChange={(v) => {
                        field.onChange(v);
                        setValue('target_ids', []);
                      }}
                      options={[
                        { value: 'pack', label: t('targets.pack') },
                        { value: 'extra', label: t('targets.extra') },
                      ]}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            {targets.length > 0 && (
              <FormField
                control={control}
                name="target_ids"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('fields.targets')}</FormLabel>
                    <FormControl>
                      <MultiSelect
                        value={field.value}
                        onChange={field.onChange}
                        options={targets.map((x) => ({
                          value: x.id,
                          label: p.name(x.name) || x.code,
                        }))}
                      />
                    </FormControl>
                    <FormDescription>{t('hints.targets')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
          </div>
        </EditorSection>
        <ActionFields control={control} allowed={ACTIONS_BY_STAGE[stage]} />
        <ConditionFields
          control={control}
          periods={p.periods}
          zones={p.zones}
          agreements={summary.agreements}
        />
        <EditorSection title={t('sections.options')}>
          <div className="grid gap-3 sm:grid-cols-2">
            {stage !== 'base' && (
              <SwitchRow
                control={control}
                name="stackable"
                label={t('fields.stackable')}
                description={t('hints.stackable')}
              />
            )}
            <SwitchRow control={control} name="active" label={t('fields.active')} />
          </div>
        </EditorSection>
        <RulePreview
          control={control}
          seasonId={p.seasonId}
          names={summary.names}
          locale={p.locale}
        />
      </SheetForm>
    </Form>
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
    <Card className="border-primary/30 bg-primary/5 py-0 shadow-none">
      <CardContent className="flex gap-3 p-4">
        <SparklesIcon
          aria-hidden="true"
          className="mt-0.5 size-4 shrink-0 text-primary-on-surface"
        />
        <p aria-live="polite" className="text-sm" data-testid="rule-preview">
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
      </CardContent>
    </Card>
  );
}
