import {
  CircleCheckIcon,
  InfoIcon,
  Loader2Icon,
  OctagonXIcon,
  TriangleAlertIcon,
} from 'lucide-react';
import { useTheme } from '@arenile/shared/ui/patterns/ThemeProvider';
import { Toaster as Sonner, type ToasterProps } from 'sonner';

const Toaster = ({ ...props }: ToasterProps) => {
  const { resolvedTheme } = useTheme();

  return (
    <Sonner
      theme={resolvedTheme}
      className="toaster group"
      icons={{
        success: <CircleCheckIcon className="size-4" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <OctagonXIcon className="size-4" />,
        loading: <Loader2Icon className="size-4 animate-spin" />,
      }}
      style={
        {
          '--normal-bg': 'var(--popover)',
          '--normal-text': 'var(--popover-foreground)',
          '--normal-border': 'var(--border)',
          '--border-radius': 'var(--radius)',
          // Rich colours keep their tinted background and icon, but the text
          // uses the theme foreground: sonner's own green/red/orange text is
          // below 4.5:1 on its light backgrounds (WCAG 1.4.3).
          '--success-text': 'var(--popover-foreground)',
          '--error-text': 'var(--popover-foreground)',
          '--warning-text': 'var(--popover-foreground)',
          '--info-text': 'var(--popover-foreground)',
        } as React.CSSProperties
      }
      {...props}
    />
  );
};

export { Toaster };
