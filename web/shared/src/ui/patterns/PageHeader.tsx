import { cn } from '@arenile/shared/lib/utils';
import type { ReactNode } from 'react';

export interface PageHeaderProps {
  /** Page title, rendered as the page's only <h1>. */
  title: ReactNode;
  description?: ReactNode;
  /** Buttons aligned to the right (stack under the title on mobile). */
  actions?: ReactNode;
  /** Small element above the title (e.g. a back link or a Badge). */
  eyebrow?: ReactNode;
  /** id of the <h1>, for aria-labelledby on the surrounding <section>. */
  titleId?: string;
  className?: string;
}

/** Title block at the top of every panel page. */
export function PageHeader({
  title,
  description,
  actions,
  eyebrow,
  titleId = 'page-title',
  className,
}: PageHeaderProps) {
  return (
    <header
      className={cn(
        'flex flex-col gap-4 pb-2 sm:flex-row sm:items-end sm:justify-between',
        className,
      )}
    >
      <div className="min-w-0 space-y-1.5 sm:min-w-64">
        {eyebrow}
        <h1 id={titleId} className="text-2xl font-semibold tracking-tight text-balance">
          {title}
        </h1>
        {description && (
          <p className="max-w-2xl text-sm text-pretty text-muted-foreground">{description}</p>
        )}
      </div>
      {actions && (
        // Many actions wrap on more lines instead of squeezing the title column.
        <div className="flex min-w-0 flex-wrap items-center gap-2 sm:justify-end">{actions}</div>
      )}
    </header>
  );
}
