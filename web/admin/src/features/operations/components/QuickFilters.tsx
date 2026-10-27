import { Button } from '@arenile/shared/ui/button';
import { OptionSelect } from '@arenile/shared/ui/patterns';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import {
  CircleCheckIcon,
  EuroIcon,
  LogInIcon,
  LogOutIcon,
  XIcon,
  type LucideIcon,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  SIMPLE_FILTERS,
  type FilterOption,
  type QuickFilter,
  type QuickFilterKind,
} from '../lib/quickFilters';

const ICONS: Partial<Record<QuickFilterKind, LucideIcon>> = {
  arrivals: LogInIcon,
  departures: LogOutIcon,
  to_collect: EuroIcon,
  present: CircleCheckIcon,
};

export interface QuickFiltersProps {
  value: QuickFilter | null;
  onChange: (filter: QuickFilter | null) => void;
  agreements: readonly FilterOption[];
  groups: readonly FilterOption[];
}

/**
 * Quick filters of the operational map (spec 30.4): arrivals, departures,
 * open balances, guests present, one agreement, one group. One at a time;
 * the map highlights the matching spots and dims the others. Each button
 * has an icon and a text, never colour alone.
 */
export function QuickFilters({ value, onChange, agreements, groups }: QuickFiltersProps) {
  const { t } = useTranslation('operations');
  const simple = value && SIMPLE_FILTERS.includes(value.kind) ? value.kind : '';
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2" data-testid="quick-filters">
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        spacing={1}
        aria-label={t('filters.label')}
        value={simple}
        onValueChange={(v) => onChange(v ? { kind: v as QuickFilterKind } : null)}
        className="flex-wrap"
      >
        {SIMPLE_FILTERS.map((k) => {
          const Icon = ICONS[k]!;
          return (
            <ToggleGroupItem key={k} value={k}>
              <Icon aria-hidden="true" />
              {t(`filters.${k}`)}
            </ToggleGroupItem>
          );
        })}
      </ToggleGroup>
      {agreements.length > 0 && (
        <OptionSelect
          aria-label={t('filters.agreement')}
          className="w-auto max-w-56"
          value={value?.kind === 'agreement' ? (value.value ?? '') : ''}
          onChange={(v) => onChange(v ? { kind: 'agreement', value: v } : null)}
          placeholder={t('filters.agreement')}
          options={[{ value: '', label: t('filters.anyAgreement') }, ...agreements]}
        />
      )}
      {groups.length > 0 && (
        <OptionSelect
          aria-label={t('filters.group')}
          className="w-auto max-w-56"
          value={value?.kind === 'group' ? (value.value ?? '') : ''}
          onChange={(v) => onChange(v ? { kind: 'group', value: v } : null)}
          placeholder={t('filters.group')}
          options={[{ value: '', label: t('filters.anyGroup') }, ...groups]}
        />
      )}
      {value && (
        <Button size="sm" variant="ghost" onClick={() => onChange(null)}>
          <XIcon aria-hidden="true" />
          {t('filters.clear')}
        </Button>
      )}
    </div>
  );
}
