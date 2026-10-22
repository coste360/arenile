import { Button } from '@arenile/shared/ui';
import { useTranslation } from 'react-i18next';
import { DECORATION_KINDS, SPOT_KINDS, type DecorationKind, type SpotKind } from './model/types';

/** Palette of the elements that can be added: spot kinds and decoration kinds. */
export function Palette({
  canAddSpots,
  onAddSpot,
  onAddDecoration,
}: {
  canAddSpots: boolean;
  onAddSpot: (kind: SpotKind) => void;
  onAddDecoration: (kind: DecorationKind) => void;
}) {
  const { t } = useTranslation('mapEditor');
  return (
    <div className="space-y-2">
      <div
        role="group"
        aria-label={t('palette.spots')}
        className="flex flex-wrap items-center gap-1"
      >
        <span className="mr-1 text-xs font-medium text-slate-600" aria-hidden>
          {t('palette.spots')}
        </span>
        {SPOT_KINDS.map((k) => (
          <Button
            key={k}
            size="sm"
            variant="secondary"
            disabled={!canAddSpots}
            aria-label={t('palette.addSpot', { kind: t(`kinds.${k}`) })}
            onClick={() => onAddSpot(k)}
          >
            {t(`kinds.${k}`)}
          </Button>
        ))}
      </div>
      <div
        role="group"
        aria-label={t('palette.decorations')}
        className="flex flex-wrap items-center gap-1"
      >
        <span className="mr-1 text-xs font-medium text-slate-600" aria-hidden>
          {t('palette.decorations')}
        </span>
        {DECORATION_KINDS.map((k) => (
          <Button
            key={k}
            size="sm"
            variant="outline"
            aria-label={t('palette.addDecoration', { kind: t(`decorations.${k}`) })}
            onClick={() => onAddDecoration(k)}
          >
            {t(`decorations.${k}`)}
          </Button>
        ))}
      </div>
    </div>
  );
}
