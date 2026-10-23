import type { LucideIcon } from 'lucide-react';
import { useSyncExternalStore, type ReactNode } from 'react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@arenile/shared/ui/select';
import { TabsList, TabsTrigger } from '@arenile/shared/ui/tabs';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { cn } from '@arenile/shared/lib/utils';

/** Below this width (Tailwind `sm`) tab bars and long segmented controls become a Select. */
export const NARROW_QUERY = '(max-width: 639px)';

/** True while the media query matches (false without matchMedia, e.g. on the server). */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
        return () => {};
      }
      const mql = window.matchMedia(query);
      mql.addEventListener('change', onChange);
      return () => mql.removeEventListener('change', onChange);
    },
    () =>
      typeof window !== 'undefined' && typeof window.matchMedia === 'function'
        ? window.matchMedia(query).matches
        : false,
    () => false,
  );
}

/** True below the `sm` breakpoint. */
export function useIsNarrow(): boolean {
  return useMediaQuery(NARROW_QUERY);
}

export interface TabOption {
  value: string;
  label: ReactNode;
  icon?: LucideIcon;
  disabled?: boolean;
}

export interface OptionPickerProps {
  /** Accessible name (the tab list / control label). */
  label: string;
  value: string;
  onValueChange: (value: string) => void;
  options: readonly TabOption[];
  className?: string;
}

function OptionContent({ option }: { option: TabOption }) {
  const Icon = option.icon;
  return (
    <>
      {Icon && <Icon aria-hidden="true" />}
      {option.label}
    </>
  );
}

/** Full-width Select that stands in for a tab bar or segmented control on narrow screens. */
export function OptionSelectMenu({
  label,
  value,
  onValueChange,
  options,
  className,
}: OptionPickerProps) {
  return (
    <Select value={value} onValueChange={onValueChange}>
      <SelectTrigger aria-label={label} className={cn('w-full', className)}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o.value} value={o.value} disabled={o.disabled}>
            <OptionContent option={o} />
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export interface ResponsiveTabsListProps extends OptionPickerProps {
  /** `line` for a second level of tabs inside a tab. */
  variant?: 'default' | 'line';
}

/**
 * The tab bar of a controlled <Tabs value onValueChange>: shadcn TabsList from
 * `sm` up, a full-width Select (icon + label) below, driving the same value.
 * Never wraps, never scrolls.
 */
export function ResponsiveTabsList({
  label,
  value,
  onValueChange,
  options,
  variant = 'default',
  className,
}: ResponsiveTabsListProps) {
  const narrow = useIsNarrow();
  if (narrow) {
    return (
      <OptionSelectMenu
        label={label}
        value={value}
        onValueChange={onValueChange}
        options={options}
        className={className}
      />
    );
  }
  return (
    <TabsList variant={variant} aria-label={label} className={className}>
      {options.map((o) => (
        <TabsTrigger key={o.value} value={o.value} disabled={o.disabled}>
          <OptionContent option={o} />
        </TabsTrigger>
      ))}
    </TabsList>
  );
}

export interface SegmentedControlProps extends OptionPickerProps {
  /**
   * Become a Select below `sm`: set it when the options do not fit in 288px
   * (a 320px screen minus the page gutters).
   */
  collapseOnNarrow?: boolean;
}

/** Single-choice segmented control (outline ToggleGroup); optionally a Select on narrow screens. */
export function SegmentedControl({
  label,
  value,
  onValueChange,
  options,
  collapseOnNarrow,
  className,
}: SegmentedControlProps) {
  const narrow = useIsNarrow();
  if (collapseOnNarrow && narrow) {
    return (
      <OptionSelectMenu
        label={label}
        value={value}
        onValueChange={onValueChange}
        options={options}
        className={className}
      />
    );
  }
  return (
    <ToggleGroup
      type="single"
      variant="outline"
      aria-label={label}
      value={value}
      onValueChange={(v) => v && onValueChange(v)}
      className={className}
    >
      {options.map((o) => (
        <ToggleGroupItem key={o.value} value={o.value} disabled={o.disabled}>
          <OptionContent option={o} />
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
