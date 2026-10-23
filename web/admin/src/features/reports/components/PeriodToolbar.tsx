import { formatIsoDate } from '@arenile/shared/lib/dates';
import { Label } from '@arenile/shared/ui/label';
import { NativeSelect, NativeSelectOption } from '@arenile/shared/ui/native-select';
import { DateRangePicker, PageToolbar, SegmentedControl } from '@arenile/shared/ui/patterns';
import { Switch } from '@arenile/shared/ui/switch';
import { useTranslation } from 'react-i18next';
import type { PeriodMode, PeriodSelection, SeasonLike } from '../lib/period';

export interface PeriodToolbarProps {
  selection: PeriodSelection;
  onChange: (sel: PeriodSelection) => void;
  seasons: readonly SeasonLike[];
  /** Today in the establishment calendar (YYYY-MM-DD): seeds the month picker. */
  today: string;
  /** Default season when switching to "season". */
  fallbackSeasonId: string | undefined;
  compare: boolean;
  onCompareChange: (compare: boolean) => void;
  /** Year the comparison is made against (label of the switch). */
  compareYear: number | undefined;
}

/**
 * Period of the reports: season, calendar month or custom range, plus the
 * "compare with previous year" switch. One row above every section (filters
 * in one place); wraps on narrow screens.
 */
export function PeriodToolbar({
  selection,
  onChange,
  seasons,
  today,
  fallbackSeasonId,
  compare,
  onCompareChange,
  compareYear,
}: PeriodToolbarProps) {
  const { t } = useTranslation('reports');
  const setMode = (mode: PeriodMode) => {
    if (mode === selection.mode) return;
    if (mode === 'season') onChange({ mode, seasonId: fallbackSeasonId ?? '' });
    else if (mode === 'month') onChange({ mode, month: today.slice(0, 7) });
    else onChange({ mode, from: `${today.slice(0, 7)}-01`, to: today });
  };
  return (
    <PageToolbar className="gap-x-4 gap-y-2">
      <SegmentedControl
        label={t('period.label')}
        value={selection.mode}
        onValueChange={(v) => setMode(v as PeriodMode)}
        options={[
          { value: 'season', label: t('period.season') },
          { value: 'month', label: t('period.month') },
          { value: 'custom', label: t('period.custom') },
        ]}
      />

      {selection.mode === 'season' && (
        <div className="flex min-w-0 items-center gap-2">
          <Label htmlFor="report-season" className="text-muted-foreground">
            {t('period.seasonLabel')}
          </Label>
          <NativeSelect
            id="report-season"
            value={selection.seasonId}
            disabled={seasons.length === 0}
            onChange={(e) => onChange({ mode: 'season', seasonId: e.target.value })}
          >
            {seasons.length === 0 && (
              <NativeSelectOption value="">{t('period.noSeasons')}</NativeSelectOption>
            )}
            {seasons.map((s) => (
              <NativeSelectOption key={s.id} value={s.id}>
                {t('period.seasonOption', {
                  year: s.year,
                  from: formatIsoDate(s.opens_on),
                  to: formatIsoDate(s.closes_on),
                })}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
      )}

      {selection.mode === 'month' && (
        <MonthPicker
          value={selection.month}
          today={today}
          onChange={(month) => onChange({ mode: 'month', month })}
        />
      )}

      {selection.mode === 'custom' && (
        <div className="flex min-w-0 items-center gap-2">
          <Label htmlFor="report-range" className="text-muted-foreground">
            {t('period.range')}
          </Label>
          <DateRangePicker
            id="report-range"
            value={{ from: selection.from, to: selection.to }}
            onChange={(v) => v && onChange({ mode: 'custom', from: v.from, to: v.to })}
          />
        </div>
      )}

      <div className="flex h-9 items-center gap-2">
        <Switch id="report-compare" checked={compare} onCheckedChange={onCompareChange} />
        <Label htmlFor="report-compare">
          {compareYear ? t('period.compareWith', { year: compareYear }) : t('period.compare')}
        </Label>
      </div>
    </PageToolbar>
  );
}

function MonthPicker({
  value,
  today,
  onChange,
}: {
  value: string;
  today: string;
  onChange: (month: string) => void;
}) {
  const { t, i18n } = useTranslation('reports');
  const [year, mm] = value.split('-');
  const thisYear = Number(today.slice(0, 4));
  const years = Array.from({ length: 6 }, (_, i) => thisYear - 4 + i);
  const monthName = new Intl.DateTimeFormat(i18n.language, { month: 'long', timeZone: 'UTC' });
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="flex items-center gap-2">
        <Label htmlFor="report-month" className="text-muted-foreground">
          {t('period.monthLabel')}
        </Label>
        <NativeSelect
          id="report-month"
          value={mm}
          onChange={(e) => onChange(`${year}-${e.target.value}`)}
        >
          {Array.from({ length: 12 }, (_, i) => {
            const v = String(i + 1).padStart(2, '0');
            return (
              <NativeSelectOption key={v} value={v}>
                {monthName.format(new Date(Date.UTC(2000, i, 1)))}
              </NativeSelectOption>
            );
          })}
        </NativeSelect>
      </div>
      <div className="flex items-center gap-2">
        <Label htmlFor="report-year" className="text-muted-foreground">
          {t('period.year')}
        </Label>
        <NativeSelect
          id="report-year"
          value={year}
          onChange={(e) => onChange(`${e.target.value}-${mm}`)}
        >
          {years.map((y) => (
            <NativeSelectOption key={y} value={String(y)}>
              {y}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </div>
    </div>
  );
}
