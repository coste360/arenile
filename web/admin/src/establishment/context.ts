import { useEstablishments, useMe, type Establishment } from '@arenile/shared/api';
import { useParams } from '@tanstack/react-router';

/**
 * Establishment selected in the URL (`/e/$establishmentId/...`), or undefined
 * on routes that are not establishment-scoped. The URL is the only source of
 * truth for the current establishment: features read it with this hook (or
 * `Route.useParams()` inside their pages) and never store it elsewhere.
 */
export function useEstablishmentId(): string | undefined {
  const params = useParams({ strict: false }) as { establishmentId?: string };
  return params.establishmentId;
}

/**
 * Like useEstablishmentId, but throws outside establishment-scoped routes.
 * Use it in pages mounted under `/e/$establishmentId`.
 */
export function useRequiredEstablishmentId(): string {
  const id = useEstablishmentId();
  if (!id) throw new Error('useRequiredEstablishmentId used outside /e/$establishmentId routes');
  return id;
}

export interface CurrentEstablishment {
  /** ID from the URL, else the first accessible establishment. */
  id: string | undefined;
  /** The establishment, once the list is loaded. */
  establishment: Establishment | undefined;
  /** Accessible establishments (first page, up to 200). */
  items: Establishment[];
  isLoading: boolean;
}

/** Current establishment for the shell (switcher and configuration menu). */
export function useCurrentEstablishment(): CurrentEstablishment {
  const fromUrl = useEstablishmentId();
  const list = useEstablishments({ limit: 200 });
  const items = list.data?.items ?? [];
  const id = fromUrl ?? items[0]?.id;
  return {
    id,
    establishment: items.find((e) => e.id === id),
    items,
    isLoading: list.isLoading,
  };
}

/** True when the signed-in user is owner somewhere (may create establishments, manage users). */
export function useIsOwner(): boolean {
  const me = useMe();
  return (me.data?.memberships ?? []).some((m) => m.role === 'owner');
}
