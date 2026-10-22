import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@arenile/shared/ui/empty';
import { cn } from '@arenile/shared/lib/utils';

export interface EmptyStateProps {
  icon?: LucideIcon;
  title: ReactNode;
  description?: ReactNode;
  /** Primary/secondary actions (Buttons). */
  action?: ReactNode;
  /** Extra content under the actions (e.g. a Badge with the milestone). */
  children?: ReactNode;
  /** Dashed border around the block (default true). */
  bordered?: boolean;
  className?: string;
}

/** Empty list / not-yet-available state, built on shadcn Empty. */
export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  children,
  bordered = true,
  className,
}: EmptyStateProps) {
  return (
    <Empty className={cn(bordered && 'border', className)}>
      <EmptyHeader>
        {Icon && (
          <EmptyMedia variant="icon">
            <Icon aria-hidden="true" />
          </EmptyMedia>
        )}
        <EmptyTitle>{title}</EmptyTitle>
        {description && <EmptyDescription>{description}</EmptyDescription>}
      </EmptyHeader>
      {(action || children) && (
        <EmptyContent>
          {action && <div className="flex flex-wrap justify-center gap-2">{action}</div>}
          {children}
        </EmptyContent>
      )}
    </Empty>
  );
}
