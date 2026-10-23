import { SectionCard as SharedSectionCard } from '@arenile/shared/ui/patterns';
import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';

/**
 * A settings section of the online pages: the shared SectionCard (icon,
 * title, description, actions, footer bar) inside a named region, so screen
 * readers and tests can reach each section by its title.
 */
export function SectionCard({
  id,
  icon,
  title,
  description,
  actions,
  children,
  footer,
  flush,
}: {
  id: string;
  icon: LucideIcon;
  title: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
  /** Footer bar content (buttons right-aligned by the shared card). */
  footer?: ReactNode;
  /** Content touches the card edges (a DataTable embedded). */
  flush?: boolean;
}) {
  return (
    <section aria-labelledby={`${id}-title`}>
      <SharedSectionCard
        titleId={`${id}-title`}
        icon={icon}
        title={title}
        description={description}
        actions={actions}
        footer={footer}
        flush={flush}
      >
        {children}
      </SharedSectionCard>
    </section>
  );
}
