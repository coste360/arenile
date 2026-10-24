import { Popover, PopoverAnchor, PopoverContent } from '@arenile/shared/ui/popover';
import type { RefObject } from 'react';
import AvailabilityCalendar, { type AvailabilityCalendarProps } from './AvailabilityCalendar';

export interface CalendarPopoverProps extends AvailabilityCalendarProps {
  /** The always-rendered trigger button: anchor, focus return target, outside-click exception. */
  triggerRef: RefObject<HTMLButtonElement | null>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  contentId: string;
  /** Accessible name of the popover dialog (the field label). */
  label: string;
}

/**
 * Popover + availability calendar, one lazy chunk loaded on the first open:
 * Radix Popover, floating-ui and react-day-picker stay out of the first
 * screen of the public page (spec 12.6). The trigger is a plain button
 * outside this chunk, used as a virtual anchor.
 */
export default function CalendarPopover({
  triggerRef,
  open,
  onOpenChange,
  contentId,
  label,
  ...calendar
}: CalendarPopoverProps) {
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor virtualRef={triggerRef} />
      <PopoverContent
        id={contentId}
        aria-label={label}
        className="w-auto p-0"
        align="start"
        onCloseAutoFocus={(e) => {
          e.preventDefault();
          triggerRef.current?.focus();
        }}
        onInteractOutside={(e) => {
          // A click on the trigger toggles; do not close-and-reopen.
          if (triggerRef.current?.contains(e.target as Node)) e.preventDefault();
        }}
      >
        <AvailabilityCalendar {...calendar} />
      </PopoverContent>
    </Popover>
  );
}
