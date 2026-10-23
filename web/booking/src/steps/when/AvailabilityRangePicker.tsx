import { formatIsoDate } from '@arenile/shared/lib/dates';
import { Button } from '@arenile/shared/ui/button';
import { CalendarIcon, XIcon } from 'lucide-react';
import { Suspense, lazy, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Slots, StayDates } from '../../flow/selection';

// Popover, floating-ui, react-day-picker and the date-fns locales load on
// first open (first-screen budget, spec 12.6).
const CalendarPopover = lazy(() => import('./CalendarPopover'));

export interface AvailabilityRangePickerProps {
  id: string;
  slug: string;
  value: StayDates | undefined;
  onChange: (value: StayDates | undefined) => void;
  min: string;
  max?: string;
  slots: Slots;
  showRatePeriods: boolean;
  'aria-labelledby'?: string;
  'aria-describedby'?: string;
}

/**
 * Stay dates field of the public page: a button showing
 * dd/MM/yyyy – dd/MM/yyyy that opens the availability calendar in a popover
 * (dialog semantics). Closes when the range is complete.
 */
export function AvailabilityRangePicker({
  id,
  slug,
  value,
  onChange,
  min,
  max,
  slots,
  showRatePeriods,
  ...aria
}: AvailabilityRangePickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const contentId = useId();
  const label = value
    ? `${formatIsoDate(value.from)} – ${value.to ? formatIsoDate(value.to) : '…'}`
    : t('ui.date.rangePlaceholder');
  return (
    <div className="flex min-w-0 items-center gap-1">
      <Button
        ref={triggerRef}
        id={id}
        type="button"
        variant="outline"
        data-empty={!value}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? contentId : undefined}
        onClick={() => {
          setLoaded(true);
          setOpen((o) => !o);
        }}
        className="h-11 w-full min-w-0 justify-start gap-2 bg-transparent px-3 text-left font-normal tabular-nums data-[empty=true]:text-muted-foreground sm:w-80 dark:bg-input/30"
        {...aria}
      >
        <CalendarIcon aria-hidden="true" className="text-muted-foreground" />
        {label}
      </Button>
      {loaded && (
        <Suspense
          fallback={
            <span role="status" className="sr-only">
              {t('common.loading')}
            </span>
          }
        >
          <CalendarPopover
            triggerRef={triggerRef}
            open={open}
            onOpenChange={setOpen}
            contentId={contentId}
            slug={slug}
            value={value}
            min={min}
            max={max}
            slots={slots}
            showRatePeriods={showRatePeriods}
            onSelect={(next) => {
              onChange(next);
              if (next?.to) setOpen(false);
            }}
          />
        </Suspense>
      )}
      {value && (
        <Button
          variant="ghost"
          size="icon"
          className="size-11"
          aria-label={t('ui.date.clear')}
          onClick={() => onChange(undefined)}
        >
          <XIcon aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}
