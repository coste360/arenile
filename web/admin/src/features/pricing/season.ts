import { useNavigate, useSearch } from '@tanstack/react-router';
import { ratePeriods, seasons, type RatePeriod } from '../configuration/api/resources';

/**
 * Season selected through the `?seasonId=` search parameter (shared by the
 * pricing pages and the policies page); defaults to the active season, else
 * the most recent one.
 */
export function useSelectedSeason(establishmentId: string) {
  const search = useSearch({ strict: false }) as { seasonId?: string };
  const navigate = useNavigate();
  const list = seasons.useList(establishmentId);
  const items = [...(list.data ?? [])].sort((a, b) => b.year - a.year);
  const season =
    items.find((s) => s.id === search.seasonId) ??
    items.find((s) => s.status === 'active') ??
    items[0];
  const periods = ratePeriods.useList(
    establishmentId,
    { season_id: season?.id },
    { enabled: !!season },
  );
  const select = (id: string) =>
    void navigate({
      to: '.',
      search: (prev: Record<string, unknown>) => ({ ...prev, seasonId: id }),
    });
  return {
    seasons: items,
    season,
    periods: [...(periods.data ?? [])].sort((a, b) => a.sort_order - b.sort_order) as RatePeriod[],
    isLoading: list.isPending,
    error: list.error,
    refetch: () => void list.refetch(),
    select,
  };
}
