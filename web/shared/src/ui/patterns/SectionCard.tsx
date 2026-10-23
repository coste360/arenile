import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
} from '@arenile/shared/ui/card';
import { cn } from '@arenile/shared/lib/utils';

export interface SectionCardProps {
  title: ReactNode;
  description?: ReactNode;
  /** Small icon tile before the title (settings sections). */
  icon?: LucideIcon;
  /** Buttons on the right of the title (e.g. "Registra incasso"). */
  actions?: ReactNode;
  /** Bar under the content, buttons right-aligned (e.g. the section's Save). */
  footer?: ReactNode;
  /**
   * Content touches the card edges: for a <DataTable embedded> or a list
   * that draws its own rows.
   */
  flush?: boolean;
  /** Heading level of the title (default h2). */
  as?: 'h2' | 'h3';
  /** id of the title, for aria-labelledby. */
  titleId?: string;
  children?: ReactNode;
  className?: string;
}

/**
 * The one card surface for a titled block of a page: detail sections,
 * breakdown tables, settings forms. Title, description and actions on top,
 * content, optional footer bar.
 */
export function SectionCard({
  title,
  description,
  icon: Icon,
  actions,
  footer,
  flush,
  as: Heading = 'h2',
  titleId,
  children,
  className,
}: SectionCardProps) {
  return (
    <Card className={cn('min-w-0 gap-4', flush && !footer && 'overflow-hidden pb-0', className)}>
      <CardHeader className="gap-1">
        <div className="flex min-w-0 items-center gap-3">
          {Icon && (
            <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <Icon aria-hidden="true" className="size-4" />
            </span>
          )}
          <div className="min-w-0 space-y-1">
            <Heading id={titleId} data-slot="card-title" className="leading-snug font-semibold">
              {title}
            </Heading>
            {description && <CardDescription>{description}</CardDescription>}
          </div>
        </div>
        {actions && (
          <CardAction className="flex flex-wrap items-center gap-2">{actions}</CardAction>
        )}
      </CardHeader>
      {children !== undefined && (
        <CardContent className={cn('min-w-0', flush && 'px-0')}>{children}</CardContent>
      )}
      {footer && <CardFooter className="flex-wrap justify-end gap-2 border-t">{footer}</CardFooter>}
    </Card>
  );
}
