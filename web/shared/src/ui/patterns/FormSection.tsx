import type { ReactNode } from 'react';
import type { Control, ControllerRenderProps, FieldPath, FieldValues } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@arenile/shared/ui/form';
import { Separator } from '@arenile/shared/ui/separator';
import { cn } from '@arenile/shared/lib/utils';

export interface FormSectionProps {
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  /** Footer under the fields (e.g. the section's Save button). */
  footer?: ReactNode;
  className?: string;
}

/**
 * Settings-style section: title and description on the left, fields on the
 * right (stacked on mobile). Put several in a page separated automatically.
 */
export function FormSection({ title, description, children, footer, className }: FormSectionProps) {
  return (
    <section
      className={cn('grid gap-x-10 gap-y-4 py-6 first:pt-0 md:grid-cols-[16rem_1fr]', className)}
    >
      <div className="space-y-1">
        <h2 className="text-base font-semibold">{title}</h2>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </div>
      <div className="min-w-0 space-y-5">
        {children}
        {footer && (
          <>
            <Separator />
            <div className="flex flex-wrap justify-end gap-2">{footer}</div>
          </>
        )}
      </div>
    </section>
  );
}

export interface FieldRowProps<TValues extends FieldValues, TName extends FieldPath<TValues>> {
  control: Control<TValues>;
  name: TName;
  label: ReactNode;
  description?: ReactNode;
  /** Shows "(facoltativo)" after the label. */
  optional?: boolean;
  /** Renders the control; spread `field` on a shadcn input (wrapped in FormControl). */
  children: (field: ControllerRenderProps<TValues, TName>) => ReactNode;
  className?: string;
}

/**
 * One form field on shadcn Form: label, control, description and the
 * validation message (zod messages are i18n keys and are translated).
 * The form must be wrapped in <Form {...form}>.
 */
export function FieldRow<TValues extends FieldValues, TName extends FieldPath<TValues>>({
  control,
  name,
  label,
  description,
  optional,
  children,
  className,
}: FieldRowProps<TValues, TName>) {
  const { t } = useTranslation();
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem className={className}>
          <FormLabel>
            {label}
            {optional && (
              <span className="font-normal text-muted-foreground">({t('ui.optional')})</span>
            )}
          </FormLabel>
          <FormControl>{children(field)}</FormControl>
          {description && <FormDescription>{description}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
