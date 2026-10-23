import { Slot } from 'radix-ui';
import type { ReactElement, ReactNode } from 'react';
import { tabsListVariants, tabsTriggerClasses } from '@arenile/shared/ui/tabs';
import { cn } from '@arenile/shared/lib/utils';

export interface NavTabsProps {
  /** Accessible name of the <nav>. */
  label: string;
  children: ReactNode;
  className?: string;
}

/**
 * Tabs that are links between routes (e.g. Listino / Simulatore / Matrice):
 * same look as <TabsList>/<TabsTrigger>, scrolls horizontally on narrow
 * screens, `aria-current="page"` on the active one.
 */
export function NavTabs({ label, children, className }: NavTabsProps) {
  return (
    <nav
      aria-label={label}
      data-orientation="horizontal"
      className={cn('group/tabs flex max-w-full', className)}
    >
      <div data-slot="tabs-list" data-variant="default" className={tabsListVariants()}>
        {children}
      </div>
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
