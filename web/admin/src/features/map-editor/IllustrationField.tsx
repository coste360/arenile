import { KIT_BY_ID, KitThumbnail, MapKitDefs, flagClass, type KitAsset } from '@arenile/shared/map';
import {
  Button,
  Popover,
  PopoverContent,
  PopoverTrigger,
  ToggleGroup,
  ToggleGroupItem,
  cn,
} from '@arenile/shared/ui';
import { ChevronsUpDownIcon, RotateCcwIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KitPicker, type KitFamily } from './KitPicker';

export interface IllustrationFieldProps {
  id: string;
  family: KitFamily;
  /** Explicit asset ('' = default of the kind); undefined = mixed selection. */
  value: string | undefined;
  /** The asset actually drawn (explicit or default), for the preview. */
  effective: string | null;
  disabled?: boolean;
  onPick: (asset: KitAsset) => void;
  /** Back to the default illustration of the kind. */
  onReset: () => void;
}

/** "Illustrazione" property: the current kit art and a picker to change it. */
export function IllustrationField({
  id,
  family,
  value,
  effective,
  disabled,
  onPick,
  onReset,
}: IllustrationFieldProps) {
  const { t } = useTranslation('mapEditor');
  const [open, setOpen] = useState(false);
  const ids = useMemo(() => (effective ? [effective] : []), [effective]);
  const asset = effective ? KIT_BY_ID.get(effective) : undefined;
  const name =
    value === undefined
      ? t('panel.mixed')
      : asset
        ? t(asset.labelKey, { defaultValue: asset.label })
        : t('panel.noIllustration');
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <MapKitDefs ids={ids} />
      <PopoverTrigger asChild>
        <Button
          id={id}
          variant="outline"
          disabled={disabled}
          className="h-auto w-full justify-start gap-3 px-2 py-1.5"
        >
          <span className="flex size-10 items-center justify-center rounded-md bg-muted">
            {asset && value !== undefined ? (
              <KitThumbnail asset={asset.id} className="size-9" />
            ) : null}
          </span>
          <span className="flex min-w-0 flex-1 flex-col items-start text-left">
            <span className="truncate text-sm">{name}</span>
            {value === '' && asset && (
              <span className="text-xs text-muted-foreground">{t('panel.defaultOfKind')}</span>
            )}
          </span>
          <ChevronsUpDownIcon aria-hidden="true" className="opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-[min(28rem,calc(100vw-2rem))] p-0">
        {open && (
          <KitPicker
            family={family}
            label={t('fields.illustration')}
            value={value || undefined}
            extras={
              value
                ? [{ value: 'default', label: t('panel.useDefault'), icon: RotateCcwIcon }]
                : undefined
            }
            onPickExtra={() => {
              onReset();
              setOpen(false);
            }}
            onPick={(a) => {
              onPick(a);
              setOpen(false);
            }}
          />
        )}
      </PopoverContent>
    </Popover>
  );
}

const FLAG_SWATCH = 'size-3 rounded-full bg-(--beach-flag) ring-1 ring-foreground/20';

/** Colour variant of an illustration that offers some (sea-state flag). */
export function VariantField({
  id,
  variants,
  value,
  disabled,
  onChange,
}: {
  id: string;
  variants: readonly string[];
  value: string;
  disabled?: boolean;
  onChange: (v: string) => void;
}) {
  const { t } = useTranslation('mapEditor');
  return (
    <ToggleGroup
      id={id}
      type="single"
      variant="outline"
      size="sm"
      disabled={disabled}
      value={value || variants[0]}
      onValueChange={(v) => v && onChange(v)}
      className="w-full"
    >
      {variants.map((v) => (
        <ToggleGroupItem key={v} value={v} className="flex-1 gap-1.5" data-variant={v}>
          <span aria-hidden="true" className={cn(FLAG_SWATCH, flagClass(v))} />
          {t(`variants.${v}`)}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
