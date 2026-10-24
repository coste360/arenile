import { useProblemText } from '@arenile/shared/api';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@arenile/shared/ui/popover';
import { Separator } from '@arenile/shared/ui/separator';
import { useNavigate } from '@tanstack/react-router';
import {
  BellIcon,
  CheckCheckIcon,
  CircleAlertIcon,
  InfoIcon,
  TriangleAlertIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useEstablishmentId, useInstantFormat } from '../../establishment';
import { useMarkNotificationRead, useNotifications, type AdminNotification } from './api';

const SEVERITY_ICON = {
  info: InfoIcon,
  warning: TriangleAlertIcon,
  error: CircleAlertIcon,
} as const;

/** Interpolation values of a notification: params plus the formatted amount. */
function values(n: AdminNotification): Record<string, string> {
  const v: Record<string, string> = { ...n.params };
  if (n.params.amount && /^\d+$/.test(n.params.amount))
    v.amount = formatCents(Number(n.params.amount));
  return v;
}

/**
 * Bell of the shell header (ADR 0019 F): the latest panel notifications of
 * the establishment (webhook endpoint disabled, refund failed...) with the
 * unread count. Text comes from i18n keys of the kind; params hold only ids
 * and non-personal labels.
 */
export function NotificationsMenu() {
  const establishmentId = useEstablishmentId();
  if (!establishmentId) return null;
  return <NotificationsBell establishmentId={establishmentId} />;
}

function NotificationsBell({ establishmentId }: { establishmentId: string }) {
  const { t } = useTranslation('notifications');
  const problemText = useProblemText();
  const fmt = useInstantFormat();
  const navigate = useNavigate();
  const query = useNotifications(establishmentId);
  const mark = useMarkNotificationRead(establishmentId);
  const [open, setOpen] = useState(false);
  const unread = Number(query.data?.unread_count ?? 0);
  const items = query.data?.items ?? [];

  const openItem = (n: AdminNotification) => {
    if (!n.read_at) mark.mutate(n.id, { onError: (e) => toast.error(problemText(e)) });
    setOpen(false);
    if (n.resource_type === 'webhook_endpoint') {
      void navigate({ to: '/e/$establishmentId/config/webhooks', params: { establishmentId } });
    } else if (n.resource_type === 'booking' && n.resource_id) {
      void navigate({
        to: '/e/$establishmentId/prenotazioni/$bookingId',
        params: { establishmentId, bookingId: n.resource_id },
      });
    } else if (n.resource_type === 'import_job') {
      void navigate({ to: '/e/$establishmentId/config/import', params: { establishmentId } });
    }
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className="relative"
          aria-label={unread > 0 ? t('bell.unread', { count: unread }) : t('bell.label')}
          data-testid="notifications-bell"
        >
          <BellIcon aria-hidden="true" />
          {unread > 0 && (
            <Badge className="absolute -top-1 -right-1 px-1 tabular-nums">
              {unread > 9 ? '9+' : unread}
            </Badge>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 p-0 sm:w-80">
        <div className="flex items-center justify-between gap-2 px-4 py-3">
          <p className="font-semibold">{t('title')}</p>
          {unread > 0 && (
            <Button
              variant="ghost"
              size="sm"
              disabled={mark.isPending}
              onClick={() => mark.mutate('all', { onError: (e) => toast.error(problemText(e)) })}
            >
              <CheckCheckIcon aria-hidden="true" />
              {t('markAll')}
            </Button>
          )}
        </div>
        <Separator />
        {items.length === 0 ? (
          <p className="px-4 py-6 text-center text-sm text-muted-foreground">
            {query.isError ? problemText(query.error) : t('empty')}
          </p>
        ) : (
          <ul className="max-h-96 overflow-y-auto py-1">
            {items.map((n) => {
              const Icon = SEVERITY_ICON[n.severity] ?? InfoIcon;
              const v = values(n);
              return (
                <li key={n.id}>
                  <button
                    type="button"
                    onClick={() => openItem(n)}
                    className="flex w-full gap-3 px-4 py-2 text-left hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
                  >
                    <Icon
                      aria-hidden="true"
                      className={
                        n.severity === 'error'
                          ? 'mt-0.5 size-4 shrink-0 text-destructive'
                          : 'mt-0.5 size-4 shrink-0 text-muted-foreground'
                      }
                    />
                    <span className="min-w-0 flex-1 space-y-0.5">
                      <span className={n.read_at ? 'block text-sm' : 'block text-sm font-medium'}>
                        {t(`kinds.${n.kind}.title`, v)}
                      </span>
                      <span className="block text-xs break-words text-muted-foreground">
                        {t(`kinds.${n.kind}.body`, v)}
                      </span>
                      <span className="block text-xs text-muted-foreground">
                        {fmt.dateTime(n.created_at)}
                      </span>
                    </span>
                    {!n.read_at && (
                      <span className="mt-1.5 size-2 shrink-0 rounded-full bg-primary">
                        <span className="sr-only">{t('unreadMarker')}</span>
                      </span>
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  );
}
