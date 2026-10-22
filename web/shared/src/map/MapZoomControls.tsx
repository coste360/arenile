import { MaximizeIcon, ZoomInIcon, ZoomOutIcon } from 'lucide-react';
import { useLayoutEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '../lib/utils';
import { Button } from '../ui/button';
import { ButtonGroup } from '../ui/button-group';
import { Slider } from '../ui/slider';

export interface MapZoomControlsProps {
  /** Current scale (1 = fit). */
  scale: number;
  min: number;
  max: number;
  onZoomIn: () => void;
  onZoomOut: () => void;
  onFit: () => void;
  /** Slider changes; omit to hide the slider. */
  onScaleChange?: (scale: number) => void;
  className?: string;
}

/**
 * Zoom controls of a map: zoom out / slider / zoom in / fit, as a shadcn
 * ButtonGroup floating over the drawing. The slider works in percent.
 */
export function MapZoomControls({
  scale,
  min,
  max,
  onZoomIn,
  onZoomOut,
  onFit,
  onScaleChange,
  className,
}: MapZoomControlsProps) {
  const { t } = useTranslation();
  const sliderRef = useRef<HTMLSpanElement>(null);
  const percent = Math.round(scale * 100);
  const label = t('map.zoom');

  // Radix names the thumb (role="slider") only from its own props, which the
  // shadcn Slider does not expose: name it and give a readable value here.
  useLayoutEffect(() => {
    const thumb = sliderRef.current?.querySelector('[role="slider"]');
    thumb?.setAttribute('aria-label', label);
    thumb?.setAttribute('aria-valuetext', `${percent}%`);
  });

  return (
    <ButtonGroup
      aria-label={label}
      className={cn('rounded-md bg-background/90 shadow-sm backdrop-blur', className)}
    >
      <Button
        variant="outline"
        size="icon-sm"
        onClick={onZoomOut}
        disabled={scale <= min}
        aria-label={t('map.zoomOut')}
        title={t('map.zoomOut')}
      >
        <ZoomOutIcon aria-hidden="true" />
      </Button>
      {onScaleChange && (
        <div className="hidden w-28 items-center border-y border-input bg-background px-3 sm:flex">
          <Slider
            ref={sliderRef}
            min={Math.round(min * 100)}
            max={Math.round(max * 100)}
            step={25}
            value={[percent]}
            onValueChange={([v]) => v !== undefined && onScaleChange(v / 100)}
          />
        </div>
      )}
      <Button
        variant="outline"
        size="icon-sm"
        onClick={onZoomIn}
        disabled={scale >= max}
        aria-label={t('map.zoomIn')}
        title={t('map.zoomIn')}
      >
        <ZoomInIcon aria-hidden="true" />
      </Button>
      <Button
        variant="outline"
        size="icon-sm"
        onClick={onFit}
        aria-label={t('map.fitView')}
        title={t('map.fitView')}
      >
        <MaximizeIcon aria-hidden="true" />
      </Button>
    </ButtonGroup>
  );
}
