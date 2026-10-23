import { Slot } from 'radix-ui';
import type { ReactElement, ReactNode } from 'react';
import { tabsListVariants, tabsTriggerClasses } from '@arenile/shared/ui/tabs';
import { cn } from '@arenile/shared/lib/utils';
import { OptionSelectMenu, useIsNarrow, type TabOption } from './ResponsiveTabs';

export interface NavTabsProps {
  /** Accessible name of the <nav>. */
  label: string;
  /** The links (<NavTab>), shown from `sm` up. */
  children: ReactNode;
  /**
   * Narrow screens: a full-width Select with these options; `value` is the
   * current route's option and `onValueChange` navigates.
   */
  options: readonly TabOption[];
  value: string;
  onValueChange: (value: string) => void;
  className?: string;
}

/**
 * Tabs that are links between routes (e.g. Listino / Simulatore / Matrice):
 * same look as <TabsList>/<TabsTrigger> from `sm` up, `aria-current="page"`
 * on the active one; a Select on narrow screens, like ResponsiveTabsList.
 */
export function NavTabs({
  label,
  children,
  options,
  value,
  onValueChange,
  className,
}: NavTabsProps) {
  const narrow = useIsNarrow();
  return (
    <nav
      aria-label={label}
      data-orientation="horizontal"
      className={cn('group/tabs flex max-w-full', className)}
    >
      {narrow ? (
        <OptionSelectMenu
          label={label}
          value={value}
          onValueChange={onValueChange}
          options={options}
        />
      ) : (
        <div data-slot="tabs-list" data-variant="default" className={tabsListVariants()}>
          {children}
        </div>
      )}
    </nav>
  );
}

/** One link of <NavTabs>: wraps the router link (`<NavTab active><Link …/></NavTab>`). */
export function NavTab({ active, children }: { active: boolean; children: ReactElement }) {
  return (
    <Slot.Root
      data-slot="tabs-trigger"
      data-state={active ? 'active' : 'inactive'}
      aria-current={active ? 'page' : undefined}
      className={cn(...tabsTriggerClasses)}
    >
      {children}
    </Slot.Root>
  );
}
