import { cn } from '@arenile/shared/lib/utils';
import {
  ArmchairIcon,
  BedSingleIcon,
  BoxIcon,
  HouseIcon,
  SofaIcon,
  TentIcon,
  TentTreeIcon,
  UmbrellaIcon,
  UtensilsCrossedIcon,
  type LucideIcon,
} from 'lucide-react';
import { iconKindFor } from '../lib/componentIcons';

const ICONS: Record<string, LucideIcon> = {
  umbrella: UmbrellaIcon,
  sunbed: BedSingleIcon,
  deckchair: SofaIcon,
  chair: ArmchairIcon,
  cabin: HouseIcon,
  gazebo: TentTreeIcon,
  tent: TentIcon,
  table: UtensilsCrossedIcon,
  generic: BoxIcon,
};

/** Lucide icon of an inventory component, chosen from its code (decorative). */
export function ComponentIcon({ code, className }: { code: string; className?: string }) {
  const kind = iconKindFor(code);
  const Icon = ICONS[kind] ?? BoxIcon;
  return (
    <Icon
      aria-hidden="true"
      focusable="false"
      data-icon={kind}
      className={cn('size-4 shrink-0', className)}
    />
  );
}
