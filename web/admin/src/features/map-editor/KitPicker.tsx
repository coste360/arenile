import {
  KIT_CATEGORIES,
  KitThumbnail,
  MapKitDefs,
  isBookableAsset,
  isDecorationAsset,
  kitAssets,
  type KitAsset,
} from '@arenile/shared/map';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  cn,
} from '@arenile/shared/ui';
import { CheckIcon, type LucideIcon } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

export type KitFamily = 'spots' | 'decorations';

/** A picker entry that is not a kit asset (e.g. a free text label). */
export interface KitPickerExtra {
  value: string;
  label: string;
  icon: LucideIcon;
}

export interface KitPickerProps {
  family: KitFamily;
  /** Current illustration (highlighted), if any. */
  value?: string;
  onPick: (asset: KitAsset) => void;
  extras?: KitPickerExtra[];
  onPickExtra?: (value: string) => void;
  /** Accessible name of the list. */
  label: string;
}

/**
 * Categorised, searchable grid of kit illustrations (shadcn Command): spots
 * list the bookable assets, decorations every structure and decoration
 * asset. Thumbnails are <use> of the shared sprite (MapKitDefs), so the
 * ~100 previews add no drawing copies. Type to filter by name or category;
 * arrows move, Enter picks.
 */
export function KitPicker({ family, value, onPick, extras, onPickExtra, label }: KitPickerProps) {
  const { t } = useTranslation('mapEditor');
  const assets = useMemo(
    () => kitAssets(family === 'spots' ? isBookableAsset : isDecorationAsset),
    [family],
  );
  const ids = useMemo(() => assets.map((a) => a.id), [assets]);
  const groups = KIT_CATEGORIES.map((c) => ({
    category: c,
    items: assets.filter((a) => a.category === c),
  })).filter((g) => g.items.length > 0);

  return (
    <Command
      aria-label={label}
      className="[&_[cmdk-group-items]]:grid [&_[cmdk-group-items]]:grid-cols-4 [&_[cmdk-group-items]]:gap-1"
    >
      <MapKitDefs ids={ids} />
      <CommandInput placeholder={t('picker.search')} />
      <CommandList className="max-h-[min(26rem,60vh)]">
        <CommandEmpty>{t('picker.empty')}</CommandEmpty>
        {extras && extras.length > 0 && (
          <CommandGroup heading={t('picker.other')}>
            {extras.map((x) => (
              <CommandItem
                key={x.value}
                value={`extra:${x.value}`}
                keywords={[x.label]}
                onSelect={() => onPickExtra?.(x.value)}
                className="flex-col gap-1 p-2 text-center"
              >
                <x.icon aria-hidden="true" className="size-10! text-muted-foreground" />
                <span className="line-clamp-2 text-xs leading-tight">{x.label}</span>
              </CommandItem>
            ))}
          </CommandGroup>
        )}
        {groups.map((g) => {
          const heading = t(`kit.category.${g.category}`);
          return (
            <CommandGroup key={g.category} heading={heading}>
              {g.items.map((a) => {
                const name = t(a.labelKey, { defaultValue: a.label });
                const active = a.id === value;
                return (
                  <CommandItem
                    key={a.id}
                    value={a.id}
                    keywords={[name, a.label, heading]}
                    onSelect={() => onPick(a)}
                    data-asset={a.id}
                    aria-current={active || undefined}
                    className={cn(
                      'relative flex-col gap-1 p-2 text-center',
                      active && 'bg-accent text-accent-foreground',
                    )}
                  >
                    <KitThumbnail asset={a.id} className="size-12" />
                    <span className="line-clamp-2 text-xs leading-tight">{name}</span>
                    {active && (
                      <CheckIcon
                        aria-hidden="true"
                        className="absolute top-1 right-1 size-3.5! text-primary"
                      />
                    )}
                  </CommandItem>
                );
              })}
            </CommandGroup>
          );
        })}
      </CommandList>
    </Command>
  );
}
