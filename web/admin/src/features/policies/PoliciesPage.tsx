import {
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
  Alert,
  AlertDescription,
  Badge,
  Button,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Spinner,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
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
  type Option,
  QueryState,
  RowActions,
  SectionCard,
  SheetForm,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  CalendarXIcon,
  PlusIcon,
  ShieldCheckIcon,
  TriangleAlertIcon,
  WandSparklesIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useForm, type Control } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { createResource } from '../configuration/api/resource';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { useCrudSubmit, useEditor } from '../configuration/api/useCrud';
import { useEstablishmentSection } from '../configuration/api/useEstablishmentSection';
import {
  EditorSection,
  FormErrorAlert,
  MultiToggle,
  PricingLayout,
  SeasonSelect,
  SwitchRow,
  TextRow,
} from '../pricing/kit';
import { useSelectedSeason } from '../pricing/season';
import { usePricingData } from '../pricing/usePricingData';
import {
  defaultPolicySchema,
  policySchema,
  policyValues,
  rulesValues,
  toPolicyInput,
  toRules,
  type BookingPolicy,
  type BookingPolicyInput,
  type DefaultPolicyValues,
  type PolicyRules,
  type PolicyValues,
  type RulesValues,
} from './policy.schema';
import {
  POLICY_EXAMPLES,
  policyExample,
  type ExampleCatalog,
  type PolicyExampleId,
} from './examples';
import { RulesFields } from './RulesFields';

const policies = createResource<BookingPolicy, BookingPolicyInput, { season_id?: string }>({
  key: 'policies',
  collection: '/api/admin/v1/establishments/{establishmentId}/policies',
  item: '/api/admin/v1/establishments/{establishmentId}/policies/{policyId}',
  idParam: 'policyId',
});

const CHANNELS = ['desk', 'online', 'api'] as const;

/** Configurazione → Policy (spec 9): establishment default policy + season overrides by scope. */
export function PoliciesPage() {
  const { t } = useTranslation('policies');
  const ctx = useConfigContext();
  const sel = useSelectedSeason(ctx.establishmentId);
  const data = usePricingData(ctx);
  const readOnly = !ctx.isLoading && !ctx.canConfigure;
  const list = policies.useList(
    ctx.establishmentId,
    { season_id: sel.season?.id },
    { enabled: !!sel.season },
  );
  const remove = policies.useRemove(ctx.establishmentId);
  const editor = useEditor<BookingPolicy>();
  const items = list.data ?? [];
  const [tab, setTab] = useState<'season' | 'default'>('season');

  const periodName = (code: string) => {
    const rp = sel.periods.find((x) => x.code === code);
    return (rp && data.name(rp.name)) || code;
  };
  const scopeParts = (p: BookingPolicy): string[] => {
    const parts: string[] = [];
    for (const c of p.scope.channels ?? []) parts.push(t(`pricing:channels.${c}`));
    for (const z of p.scope.zones ?? []) {
      parts.push(data.zones.find((x) => x.id === z || x.code === z)?.name ?? z);
    }
    for (const id of p.scope.packs ?? []) {
      const pk = data.packs.find((x) => x.id === id || x.code === id);
      parts.push(pk ? data.name(pk.name) || pk.code : id);
    }
    for (const code of p.scope.rate_periods ?? []) parts.push(periodName(code));
    return parts;
  };

  const columns: ColumnDef<BookingPolicy>[] = [
    {
      accessorKey: 'priority',
      meta: { label: t('fields.priority'), align: 'right' },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('fields.priority')} />
      ),
      cell: ({ row }) => row.original.priority,
    },
    {
      accessorKey: 'name',
      meta: { label: t('fields.name') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.name')} />,
      cell: ({ row }) => <span className="font-medium">{row.original.name}</span>,
    },
    {
      id: 'scope',
      meta: { label: t('fields.scope') },
      header: t('fields.scope'),
      accessorFn: (p) => scopeParts(p).join(' '),
      cell: ({ row }) => {
        const parts = scopeParts(row.original);
        return parts.length === 0 ? (
          <span className="text-muted-foreground">{t('scopeAll')}</span>
        ) : (
          <div className="flex flex-wrap gap-1">
            {parts.map((s) => (
              <Badge key={s} variant="outline">
                {s}
              </Badge>
            ))}
          </div>
        );
      },
    },
    {
      accessorKey: 'active',
      meta: { label: t('fields.active') },
      header: t('fields.active'),
      cell: ({ row }) =>
        row.original.active ? (
          <StatusBadge tone="success">{t('fields.active')}</StatusBadge>
        ) : (
          <StatusBadge tone="neutral">{t('inactive')}</StatusBadge>
        ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) =>
        readOnly ? null : (
          <RowActions>
            <EditButton
              label={t('editAria', { name: row.original.name })}
              onClick={() => editor.edit(row.original)}
            />
            <DeleteButton
              label={t('deleteAria', { name: row.original.name })}
              message={t('deleteConfirm', { name: row.original.name })}
              onConfirm={() =>
                remove.mutateAsync({ id: row.original.id, version: row.original.version })
              }
            />
          </RowActions>
        ),
    },
  ];

  return (
    <PricingLayout
      title={t('title')}
      description={t('description')}
      readOnly={readOnly}
      establishmentId={ctx.establishmentId}
      seasonId={sel.season?.id}
      actions={<SeasonSelect seasons={sel.seasons} season={sel.season} onSelect={sel.select} />}
    >
      <Tabs value={tab} onValueChange={(v) => setTab(v as 'season' | 'default')} className="gap-4">
        <TabsList variant="line" aria-label={t('tabs.label')}>
          <TabsTrigger value="season">{t('tabs.season')}</TabsTrigger>
          <TabsTrigger value="default">{t('tabs.default')}</TabsTrigger>
        </TabsList>
        <TabsContent value="default">
          {ctx.establishment ? (
            <DefaultPolicyForm
              key={ctx.establishment.id}
              establishmentId={ctx.establishmentId}
              rules={ctx.establishment.settings.default_policy}
              startVersion={ctx.establishment.version}
              locale={ctx.uiLocale}
              readOnly={readOnly}
            />
          ) : (
            <QueryState isLoading={ctx.isLoading} error={ctx.error} onRetry={ctx.refetch} />
          )}
        </TabsContent>
        <TabsContent value="season" className="space-y-4">
          <p className="text-sm text-muted-foreground">{t('seasonHint')}</p>
          <QueryState
            isLoading={sel.isLoading || (!!sel.season && list.isPending)}
            error={sel.error ?? list.error}
            onRetry={() => void list.refetch()}
          />
          {!sel.isLoading && sel.seasons.length === 0 && (
            <EmptyState icon={CalendarXIcon} title={t('pricing:noSeason')} />
          )}
          {list.data && (
            <DataTable
              caption={t('title')}
              columns={columns}
              data={items}
              getRowId={(p) => p.id}
              searchable
              toolbar={
                !readOnly &&
                sel.season && (
                  <Button onClick={editor.create}>
                    <PlusIcon aria-hidden="true" />
                    {t('add')}
                  </Button>
                )
              }
              initialSorting={[{ id: 'priority', desc: true }]}
              empty={<EmptyState icon={ShieldCheckIcon} title={t('empty')} bordered={false} />}
            />
          )}
        </TabsContent>
      </Tabs>
      {sel.season && (
        <FormSheet
          size="lg"
          padded={false}
          open={editor.open}
          onOpenChange={editor.setOpen}
          title={editor.item ? t('editTitle', { name: editor.item.name }) : t('add')}
        >
          <PolicyForm
            key={editor.key}
            establishmentId={ctx.establishmentId}
            seasonId={sel.season.id}
            item={editor.item}
            locale={ctx.uiLocale}
            zones={data.zones}
            packs={data.packs.map((p) => ({ id: p.id, name: data.name(p.name) || p.code }))}
            periods={sel.periods.map((p) => ({ code: p.code, name: periodName(p.code) }))}
            catalog={{
              packs: data.packs.map((p) => ({
                id: p.id,
                code: p.code,
                composition: p.composition as Record<string, number>,
              })),
              periods: sel.periods.map((p) => ({ code: p.code, ranges: p.ranges })),
            }}
            onDone={editor.close}
          />
        </FormSheet>
      )}
    </PricingLayout>
  );
}

function ScopeMulti({
  control,
  name,
  label,
  options,
}: {
  control: Control<PolicyValues>;
  name: 'scope_zones' | 'scope_packs' | 'scope_periods';
  label: string;
  options: Option[];
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <MultiSelect value={field.value} onChange={field.onChange} options={options} />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

function PolicyForm(p: {
  establishmentId: string;
  seasonId: string;
  item?: BookingPolicy;
  locale: string;
  zones: { id: string; name: string }[];
  packs: { id: string; name: string }[];
  periods: { code: string; name: string }[];
  catalog: ExampleCatalog;
  onDone: () => void;
}) {
  const { t } = useTranslation('policies');
  const [exampleWarnings, setExampleWarnings] = useState<string[]>([]);
  const form = useForm<PolicyValues>({
    resolver: zodResolver(policySchema),
    defaultValues: policyValues(p.item),
  });
  const s = useCrudSubmit({
    form,
    item: p.item,
    create: policies.useCreate(p.establishmentId),
    update: policies.useUpdate(p.establishmentId),
    toInput: (v: PolicyValues) => toPolicyInput(p.seasonId, v),
    onDone: p.onDone,
  });
  const { control } = form;
  const applyExample = (id: PolicyExampleId) => {
    const fill = policyExample(id, p.catalog);
    const values = { name: form.getValues('name') || t(`examples.${id}`), ...fill.values };
    for (const [key, value] of Object.entries(values)) {
      form.setValue(key as keyof PolicyValues, value as never, {
        shouldDirty: true,
        shouldValidate: form.formState.isSubmitted,
      });
    }
    setExampleWarnings(fill.warnings);
  };
  const scope = (
    <AccordionItem value="scope">
      <AccordionTrigger className="text-base">{t('fields.scope')}</AccordionTrigger>
      <AccordionContent className="space-y-4 px-1 pt-1">
        <p className="text-sm text-muted-foreground">{t('hints.scope')}</p>
        <FormField
          control={control}
          name="scope_channels"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('scope.channels')}</FormLabel>
              <FormControl>
                <MultiToggle
                  label={t('scope.channels')}
                  value={field.value}
                  onChange={field.onChange}
                  options={CHANNELS.map((c) => ({ value: c, label: t(`pricing:channels.${c}`) }))}
                />
              </FormControl>
            </FormItem>
          )}
        />
        <div className="grid gap-4 sm:grid-cols-3">
          {p.zones.length > 0 && (
            <ScopeMulti
              control={control}
              name="scope_zones"
              label={t('scope.zones')}
              options={p.zones.map((z) => ({ value: z.id, label: z.name }))}
            />
          )}
          {p.packs.length > 0 && (
            <ScopeMulti
              control={control}
              name="scope_packs"
              label={t('scope.packs')}
              options={p.packs.map((x) => ({ value: x.id, label: x.name }))}
            />
          )}
          {p.periods.length > 0 && (
            <ScopeMulti
              control={control}
              name="scope_periods"
              label={t('scope.periods')}
              options={p.periods.map((rp) => ({ value: rp.code, label: rp.name }))}
            />
          )}
        </div>
      </AccordionContent>
    </AccordionItem>
  );
  return (
    <Form {...form}>
      <SheetForm onSubmit={s.submit} onCancel={p.onDone} pending={s.pending}>
        <FormErrorAlert state={s.alert} onReload={p.onDone} />
        <EditorSection
          title={
            <span className="flex items-center gap-2">
              <WandSparklesIcon aria-hidden="true" className="size-4 text-muted-foreground" />
              {t('examples.title')}
            </span>
          }
          description={t('examples.hint')}
        >
          <div className="flex flex-wrap gap-2">
            {POLICY_EXAMPLES.map((id) => (
              <Button key={id} variant="outline" size="sm" onClick={() => applyExample(id)}>
                {t(`examples.${id}`)}
              </Button>
            ))}
          </div>
          {exampleWarnings.length > 0 && (
            <Alert role="status" variant="warning">
              <TriangleAlertIcon aria-hidden="true" />
              <AlertDescription>
                <ul className="list-inside list-disc">
                  {exampleWarnings.map((w) => (
                    <li key={w}>{t(w)}</li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          )}
        </EditorSection>
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
              inputProps={{ inputMode: 'numeric' }}
            />
          </div>
          <SwitchRow control={control} name="active" label={t('fields.active')} />
        </EditorSection>
        <RulesFields control={control as unknown as Control<RulesValues>} before={scope} />
      </SheetForm>
    </Form>
  );
}

function DefaultPolicyForm(p: {
  establishmentId: string;
  rules: PolicyRules;
  startVersion: number;
  locale: string;
  readOnly: boolean;
}) {
  const { t } = useTranslation('policies');
  const [version, setVersion] = useState(p.startVersion);
  const form = useForm<DefaultPolicyValues>({
    resolver: zodResolver(defaultPolicySchema),
    defaultValues: rulesValues(p.rules),
  });
  const section = useEstablishmentSection({
    establishmentId: p.establishmentId,
    form,
    version,
    onVersion: setVersion,
    toPatch: (v, current) => ({ settings: { ...current.settings, default_policy: toRules(v) } }),
    toValues: (est) => rulesValues(est.settings.default_policy),
  });
  return (
    <Form {...form}>
      <form noValidate onSubmit={section.submit}>
        <SectionCard
          title={t('tabs.default')}
          description={t('defaultHint')}
          footer={
            !p.readOnly && (
              <Button type="submit" disabled={section.pending}>
                {section.pending && <Spinner />}
                {t('common.actions.save', { ns: 'translation' })}
              </Button>
            )
          }
        >
          <div className="space-y-4">
            <FormErrorAlert state={section.alert} onReload={section.reload} />
            <fieldset disabled={p.readOnly} className="min-w-0">
              <RulesFields control={form.control} isDefault />
            </fieldset>
          </div>
        </SectionCard>
      </form>
    </Form>
  );
}
