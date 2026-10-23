import type { ReactElement, ReactNode } from 'react';
import { Button } from '@arenile/shared/ui/button';
import { cn } from '@arenile/shared/lib/utils';
import { PageHeader, type PageHeaderProps } from './PageHeader';

export interface PageProps extends Omit<PageHeaderProps, 'className'> {
  /** Notices under the header (read-only banner, permission alert...). */
  notice?: ReactNode;
  children?: ReactNode;
  className?: string;
}

/**
 * Skeleton of every panel page: a <section> labelled by the PageHeader title,
 * the header, optional notices, then the content with the standard 24px gap.
 */
export function Page({
  notice,
  children,
  className,
  titleId = 'page-title',
  ...header
}: PageProps) {
  return (
    <section aria-labelledby={titleId} className={cn('min-w-0 space-y-6', className)}>
      <PageHeader titleId={titleId} {...header} />
      {notice}
      {children}
    </section>
  );
}

/**
 * "← Back" link above a page title (PageHeader `eyebrow`). Wraps the router
 * link: `<BackLink><Link to="…"><ArrowLeftIcon aria-hidden="true" />Prenotazioni</Link></BackLink>`.
 */
export function BackLink({ children }: { children: ReactElement }) {
  return (
    <Button asChild variant="ghost" size="sm" className="-ml-3 w-fit text-muted-foreground">
      {children}
    </Button>
  );
}

export interface PageToolbarProps {
  children: ReactNode;
  className?: string;
}

/**
 * Row of page-scope controls under the header (season, date navigation,
 * period toggles): one line, wraps on narrow screens. Controls carry
 * accessible names (aria-label) rather than visible stacked labels.
 */
export function PageToolbar({ children, className }: PageToolbarProps) {
  return <div className={cn('flex flex-wrap items-center gap-2', className)}>{children}</div>;
}
