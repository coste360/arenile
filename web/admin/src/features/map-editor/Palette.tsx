import { DECORATION_KIND_ICONS, SPOT_KIND_ICONS } from '@arenile/shared/map';
import { ButtonGroup } from '@arenile/shared/ui';
import { useTranslation } from 'react-i18next';
import { DECORATION_KINDS, SPOT_KINDS, type DecorationKind, type SpotKind } from './model/types';
import { ToolButton } from './ui';

/** Compact icon palette of the elements that can be added: spot kinds and decoration kinds. */
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
    <>
      <ButtonGroup aria-label={t('palette.spots')}>
        {SPOT_KINDS.map((k) => (
          <ToolButton
            key={k}
            icon={SPOT_KIND_ICONS[k]}
            label={t('palette.addSpot', { kind: t(`kinds.${k}`) })}
            disabled={!canAddSpots}
            onClick={() => onAddSpot(k)}
          />
        ))}
      </ButtonGroup>
      <ButtonGroup aria-label={t('palette.decorations')}>
        {DECORATION_KINDS.map((k) => (
          <ToolButton
            key={k}
            icon={DECORATION_KIND_ICONS[k]}
            label={t('palette.addDecoration', { kind: t(`decorations.${k}`) })}
            onClick={() => onAddDecoration(k)}
          />
        ))}
      </ButtonGroup>
    </>
  );
}
