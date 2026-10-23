import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';

/** Card with an icon, an accessible region name and an optional footer (save row). */
export function SectionCard({
  id,
  icon: Icon,
  title,
  description,
  actions,
  children,
  footer,
}: {
  id: string;
  icon: LucideIcon;
  title: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
}) {
  return (
    <Card aria-labelledby={`${id}-title`} role="region">
      <CardHeader className="flex flex-row items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary-on-surface">
          <Icon aria-hidden="true" className="size-4" />
        </span>
        <div className="min-w-0 flex-1 space-y-1.5">
          <CardTitle id={`${id}-title`}>{title}</CardTitle>
          {description && <CardDescription>{description}</CardDescription>}
        </div>
        {actions}
      </CardHeader>
      <CardContent>{children}</CardContent>
      {footer}
    </Card>
  );
}
