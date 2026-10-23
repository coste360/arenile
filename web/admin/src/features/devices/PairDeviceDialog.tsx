import { ApiProblem, useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@arenile/shared/ui/dialog';
import { Label } from '@arenile/shared/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@arenile/shared/ui/select';
import { Spinner } from '@arenile/shared/ui/spinner';
import { cn } from '@arenile/shared/lib/utils';
import { CheckCircle2Icon, CopyIcon, RefreshCwIcon, TimerIcon, WifiOffIcon } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  useCreatePairingCode,
  useDevices,
  useTeamMembers,
  type Device,
  type DevicePairingCode,
} from './api';
import { PAIRING_TTL_SECONDS, formatCountdown, isExpired, localDeadline, pairedWith } from './lib';

export interface PairDeviceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  establishmentId: string;
  /** Current user (default owner of the new device). */
  selfId: string | undefined;
  /** users.manage: may pair a device for another member of the team. */
  canManageUsers: boolean;
  /** Devices listed when the dialog opened (to recognise the new one). */
  knownDevices: readonly Device[];
}

interface Issued {
  code: DevicePairingCode;
  deadline: number;
  knownIds: ReadonlySet<string>;
}

/** Current time, ticking every second while mounted (countdown). */
function useNow(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);
  return now;
}

/**
 * "Abbina un dispositivo" (spec 29.2): issues a single-use code and shows
 * its QR (server-rendered SVG of `qr_text`), the code to type by hand and a
 * countdown. While the code is valid the device list is polled, and the
 * dialog switches to a confirmation as soon as the new device appears. The
 * code lives only in the dialog body state: closing the dialog unmounts the
 * body and forgets it.
 */
export function PairDeviceDialog({ open, onOpenChange, ...body }: PairDeviceDialogProps) {
  const { t } = useTranslation('devices');
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        closeLabel={t('common.close', { ns: 'translation' })}
        className="max-h-[90vh] overflow-y-auto sm:max-w-md"
      >
        <PairDeviceBody {...body} onClose={() => onOpenChange(false)} />
      </DialogContent>
    </Dialog>
  );
}

function PairDeviceBody({
  establishmentId,
  selfId,
  canManageUsers,
  knownDevices,
  onClose,
}: Omit<PairDeviceDialogProps, 'open' | 'onOpenChange'> & { onClose: () => void }) {
  const { t } = useTranslation('devices');
  const problemText = useProblemText();
  const create = useCreatePairingCode(establishmentId);
  const team = useTeamMembers(establishmentId, canManageUsers);
  const [userId, setUserId] = useState<string | undefined>(selfId);
  const [issued, setIssued] = useState<Issued | null>(null);
  const now = useNow();
  const expired = issued !== null && isExpired(issued.deadline, now);
  const devices = useDevices(establishmentId, { includeRevoked: false });
  const paired =
    issued && devices.data ? pairedWith(devices.data, issued.code, issued.knownIds) : undefined;
  const waiting = issued !== null && !expired && !paired;
  // Poll while the code is on screen (a second observer of the same query).
  useDevices(establishmentId, { includeRevoked: false, refetchInterval: waiting ? 3000 : false });

  const knownRef = useRef(knownDevices);
  useEffect(() => {
    knownRef.current = knownDevices;
  }, [knownDevices]);

  const issue = () => {
    create.mutate(
      {
        ttl_seconds: PAIRING_TTL_SECONDS,
        ...(userId && userId !== selfId ? { user_id: userId } : {}),
      },
      {
        onSuccess: (code) =>
          setIssued({
            code,
            deadline: localDeadline(Date.now()),
            knownIds: new Set(knownRef.current.map((d) => d.id)),
          }),
      },
    );
  };

  // Without users.manage there is nothing to choose: issue on open (the ref
  // keeps StrictMode's double effect from issuing two codes).
  const autoIssued = useRef(false);
  useEffect(() => {
    if (!canManageUsers && !autoIssued.current) {
      autoIssued.current = true;
      issue();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- once, on open
  }, []);

  const pairedId = paired?.id;
  const pairedName = paired?.name;
  useEffect(() => {
    if (pairedId) toast.success(t('pair.pairedToast', { name: pairedName }));
  }, [pairedId, pairedName, t]);

  const copy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(t('pair.copied'));
    } catch {
      // Clipboard denied: the code stays selectable.
    }
  };

  const lanRequired =
    create.error instanceof ApiProblem && create.error.type === 'lan_mode_required';
  const members = team.data ?? [];
  // Never show more than the lifetime (the clock ticks once a second).
  const shownNow = issued ? Math.max(now, issued.deadline - PAIRING_TTL_SECONDS * 1000) : now;

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t('pair.title')}</DialogTitle>
        <DialogDescription>{t('pair.description')}</DialogDescription>
      </DialogHeader>

      {canManageUsers && !issued && !paired && (
        <div className="space-y-2">
          <Label htmlFor="pair-user">{t('pair.user')}</Label>
          <Select value={userId} onValueChange={setUserId}>
            <SelectTrigger id="pair-user" className="w-full">
              <SelectValue placeholder={t('pair.userSelf')} />
            </SelectTrigger>
            <SelectContent>
              {selfId && <SelectItem value={selfId}>{t('pair.userSelf')}</SelectItem>}
              {members
                .filter((m) => m.user_id !== selfId)
                .map((m) => (
                  <SelectItem key={m.user_id} value={m.user_id}>
                    {m.name}
                  </SelectItem>
                ))}
            </SelectContent>
          </Select>
          <p className="text-sm text-muted-foreground">{t('pair.userHint')}</p>
        </div>
      )}

      {create.isPending && (
        <div className="flex justify-center py-10" role="status">
          <Spinner className="size-6" />
          <span className="sr-only">{t('pair.generating')}</span>
        </div>
      )}

      {create.isError &&
        (lanRequired ? (
          <Alert role="alert">
            <WifiOffIcon aria-hidden="true" />
            <AlertTitle>{t('lan.title')}</AlertTitle>
            <AlertDescription>{t('lan.description')}</AlertDescription>
          </Alert>
        ) : (
          <Alert variant="destructive" role="alert">
            <AlertDescription>{problemText(create.error)}</AlertDescription>
          </Alert>
        ))}

      {paired && (
        <Alert role="status" className="border-success/40 bg-success/10">
          <CheckCircle2Icon aria-hidden="true" />
          <AlertTitle>{t('pair.pairedTitle')}</AlertTitle>
          <AlertDescription>{t('pair.pairedDescription', { name: paired.name })}</AlertDescription>
        </Alert>
      )}

      {issued && !paired && !create.isPending && (
        <div className="flex flex-col items-center gap-4">
          <div className="relative">
            {issued.code.qr_svg ? (
              <img
                src={`data:image/svg+xml;charset=utf-8,${encodeURIComponent(issued.code.qr_svg)}`}
                alt={t('pair.qrAlt')}
                className={cn(
                  'size-56 rounded-md border bg-background p-2',
                  expired && 'opacity-20',
                )}
              />
            ) : (
              <p className="text-sm text-muted-foreground">{t('pair.qrUnavailable')}</p>
            )}
          </div>
          <div className="flex items-center gap-2">
            <code
              className="rounded-md border bg-muted px-3 py-1.5 font-mono text-xl tracking-widest select-all"
              aria-label={t('pair.codeLabel')}
            >
              {issued.code.code}
            </code>
            <Button
              size="icon-sm"
              variant="outline"
              aria-label={t('pair.copy')}
              disabled={expired}
              onClick={() => void copy(issued.code.code)}
            >
              <CopyIcon aria-hidden="true" />
            </Button>
          </div>
          <p className="text-center text-sm break-all text-muted-foreground">
            {t('pair.address', { url: issued.code.qr_payload.url })}
          </p>
          {expired ? (
            <Alert role="status">
              <TimerIcon aria-hidden="true" />
              <AlertTitle>{t('pair.expiredTitle')}</AlertTitle>
              <AlertDescription>{t('pair.expiredDescription')}</AlertDescription>
            </Alert>
          ) : (
            <p className="flex items-center gap-2 text-sm" aria-live="off">
              <TimerIcon aria-hidden="true" className="size-4 text-muted-foreground" />
              <span>
                {t('pair.expiresIn')}{' '}
                <span className="font-mono tabular-nums">
                  {formatCountdown(issued.deadline, shownNow)}
                </span>
              </span>
            </p>
          )}
          {!expired && (
            <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
              <li>{t('pair.step1')}</li>
              <li>{t('pair.step2')}</li>
              <li>{t('pair.step3')}</li>
            </ol>
          )}
        </div>
      )}

      <DialogFooter>
        {paired ? (
          <Button onClick={onClose}>{t('pair.done')}</Button>
        ) : (
          <>
            <Button variant="outline" onClick={onClose}>
              {t('common.close', { ns: 'translation' })}
            </Button>
            {(canManageUsers || expired || (create.isError && !lanRequired)) && !waiting && (
              <Button onClick={issue} disabled={create.isPending || lanRequired}>
                {issued || create.isError ? <RefreshCwIcon aria-hidden="true" /> : null}
                {issued || create.isError ? t('pair.regenerate') : t('pair.generate')}
              </Button>
            )}
          </>
        )}
      </DialogFooter>
    </>
  );
}
