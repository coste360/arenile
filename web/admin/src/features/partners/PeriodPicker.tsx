import { formatIsoDate } from '@arenile/shared/lib/dates';
import { Label } from '@arenile/shared/ui/label';
import { NativeSelect, NativeSelectOption } from '@arenile/shared/ui/native-select';
import { DateRangePicker, PageToolbar } from '@arenile/shared/ui/patterns';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCurrentEstablishment } from '../../establishment';
import { seasons as seasonsResource, type Season } from '../configuration/api/resources';
import { todayIn } from '../operations/lib/dates';
import { defaultSeason, resolvePeriod, type PeriodError } from '../reports/lib/period';
import type { Period } from './api';
import { yearOf } from './lib';

/** "season:<id>" or "custom". */
type Choice = string;

export interface PeriodState {
  /** Resolved inclusive period, or null while it cannot be queried. */
  period: Period | null;
  error?: PeriodError;
  /** Season of the chosen period (register header). */
  season?: Season;
  picker: ReactNode;
}

/**
 * Period of the partner statement and of the agreement reports: a season of
 * the establishment (the current one by default) or a custom range (at most
 * 400 days, like every report). Without seasons the current calendar year.
 */
export function usePeriodPicker(establishmentId: string, idPrefix: string): PeriodState {
  const { t } = useTranslation('partners');
  const { establishment } = useCurrentEstablishment();
  const today = todayIn(establishment?.timezone);
  const list = seasonsResource.useList(establishmentId);
  const seasons = [...(list.data ?? [])].sort((a, b) => b.opens_on.localeCompare(a.opens_on));
  const fallback = defaultSeason(seasons, today);
  const [choice, setChoice] = useState<Choice | null>(null);
  const [range, setRange] = useState<{ from: string; to?: string }>(() => yearOf(today));
  const effective: Choice = choice ?? (fallback ? `season:${fallback.id}` : 'custom');
  const seasonId = effective.startsWith('season:') ? effective.slice(7) : '';
  const resolved = seasonId
    ? resolvePeriod({ mode: 'season', seasonId }, seasons)
    : resolvePeriod({ mode: 'custom', from: range.from, to: range.to }, seasons);

  const picker = (
    <PageToolbar>
      <div className="flex min-w-0 items-center gap-2">
        <Label htmlFor={`${idPrefix}-period`} className="text-muted-foreground">
          {t('period.label')}
        </Label>
        <NativeSelect
          id={`${idPrefix}-period`}
          value={effective}
          onChange={(e) => setChoice(e.target.value)}
        >
          {seasons.map((s) => (
            <NativeSelectOption key={s.id} value={`season:${s.id}`}>
              {t('period.season', {
                year: s.year,
                from: formatIsoDate(s.opens_on),
                to: formatIsoDate(s.closes_on),
              })}
            </NativeSelectOption>
          ))}
          <NativeSelectOption value="custom">{t('period.custom')}</NativeSelectOption>
        </NativeSelect>
      </div>
      {effective === 'custom' && (
        <div className="flex min-w-0 items-center gap-2">
          <Label htmlFor={`${idPrefix}-range`} className="text-muted-foreground">
            {t('period.range')}
          </Label>
          <DateRangePicker
            id={`${idPrefix}-range`}
            value={range}
            onChange={(v) => v && setRange({ from: v.from, to: v.to })}
          />
        </div>
      )}
      {'error' in resolved && (
        <p role="status" className="text-sm text-muted-foreground">
          {t(`period.errors.${resolved.error}`)}
        </p>
      )}
    </PageToolbar>
  );

  // Wait for the seasons: the default period is the current season.
  if (list.isPending) return { period: null, picker };
  return 'period' in resolved
    ? {
        period: resolved.period,
        season: seasons.find((s) => s.id === seasonId),
        picker,
      }
    : { period: null, error: resolved.error, picker };
}
