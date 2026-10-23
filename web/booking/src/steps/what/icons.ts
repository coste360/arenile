import {
  ArmchairIcon,
  BedSingleIcon,
  DoorClosedIcon,
  PackageIcon,
  SofaIcon,
  TentIcon,
  UmbrellaIcon,
  type LucideIcon,
} from 'lucide-react';

/**
 * Icon of a pack component (spec 12.3 passo 3: "composizione chiara
 * (icone)"). The catalog sends a kit or lucide name; known names and the
 * usual Italian component codes map to a small static set (no dynamic icon
 * loading on the public page).
 */
const BY_NAME: Record<string, LucideIcon> = {
  umbrella: UmbrellaIcon,
  ombrellone: UmbrellaIcon,
  parasol: UmbrellaIcon,
  sunbed: BedSingleIcon,
  lettino: BedSingleIcon,
  'bed-single': BedSingleIcon,
  daybed: SofaIcon,
  sofa: SofaIcon,
  chair: ArmchairIcon,
  sdraio: ArmchairIcon,
  regista: ArmchairIcon,
  armchair: ArmchairIcon,
  cabin: DoorClosedIcon,
  cabina: DoorClosedIcon,
  cabana: DoorClosedIcon,
  tent: TentIcon,
  tenda: TentIcon,
  gazebo: TentIcon,
};

export function componentIcon(icon: string | undefined, code: string): LucideIcon {
  const keys = [icon, code].filter(Boolean).map((k) => (k as string).toLowerCase());
  for (const k of keys) {
    const direct = BY_NAME[k];
    if (direct) return direct;
    const partial = Object.entries(BY_NAME).find(([name]) => k.includes(name));
    if (partial) return partial[1];
  }
  return PackageIcon;
}
