import { ApiProblem, useProblemText } from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Page, SectionCard } from '@arenile/shared/ui/patterns';
import {
  CircleCheckIcon,
  CircleXIcon,
  KeyboardIcon,
  ScanQrCodeIcon,
  TriangleAlertIcon,
  type LucideIcon,
} from 'lucide-react';
import { useCallback, useId, useMemo, useRef, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useCurrentEstablishment,
  useInstantFormat,
  useRequiredEstablishmentId,
} from '../../establishment';
import { localize } from '../configuration/lib/i18nText';
import { QrScanner } from '../gate';
import { useGateCheckIn, type GateCheckInResult, type Ticket } from './api';
import { createScanDeduper, extractTicketCode } from './lib';

type Outcome = GateCheckInResult['outcome'] | 'not_found' | 'failed';

interface ScanResult {
  outcome: Outcome;
  code: string;
  ticket?: Ticket;
  message?: string;
}

const LOOK: Record<Outcome, { variant: 'success' | 'warning' | 'destructive'; icon: LucideIcon }> =
  {
    admitted: { variant: 'success', icon: CircleCheckIcon },
    already_entered: { variant: 'warning', icon: TriangleAlertIcon },
    wrong_date: { variant: 'warning', icon: TriangleAlertIcon },
    voided: { variant: 'destructive', icon: CircleXIcon },
    not_found: { variant: 'destructive', icon: CircleXIcon },
    failed: { variant: 'destructive', icon: CircleXIcon },
  };

/** Short haptic feedback on phones: one pulse when admitted, three when refused. */
function vibrate(outcome: Outcome) {
  try {
    navigator.vibrate?.(outcome === 'admitted' ? 120 : [80, 60, 80, 60, 80]);
  } catch {
    // Not supported: nothing to do.
  }
}

/**
 * Gate check-in from the staff phone (spec 14.3, ADR 0020): scan the ticket
 * QR with the camera (BarcodeDetector) or type the code. One entry per
 * ticket: the server registers the first pass atomically and answers
 * already_entered to any later scan, also from another phone.
 */
export function EntryGatePage() {
  const { t, i18n } = useTranslation('entryTickets');
  const establishmentId = useRequiredEstablishmentId();
  const { establishment } = useCurrentEstablishment();
  const fmt = useInstantFormat();
  const problemText = useProblemText();
  const checkIn = useGateCheckIn(establishmentId);
  const [result, setResult] = useState<ScanResult | null>(null);
  const [manual, setManual] = useState('');
  const dedupe = useMemo(() => createScanDeduper(), []);
  const busy = useRef(false);
  const inputId = useId();
  const locale = i18n.language || 'it';
  const defaultLocale = establishment?.default_locale ?? 'it';

  const submit = useCallback(
    async (raw: string) => {
      const code = extractTicketCode(raw);
      if (!code || busy.current) return;
      busy.current = true;
      try {
        const res = await checkIn.mutateAsync(code);
        setResult({ outcome: res.outcome, code, ticket: res.ticket });
        vibrate(res.outcome);
      } catch (err) {
        const notFound = err instanceof ApiProblem && err.status === 404;
        const outcome: Outcome = notFound ? 'not_found' : 'failed';
        setResult({ outcome, code, message: notFound ? undefined : problemText(err) });
        vibrate(outcome);
      } finally {
        busy.current = false;
      }
    },
    [checkIn, problemText],
  );

  const onScan = useCallback(
    (raw: string) => {
      const code = extractTicketCode(raw);
      if (code && dedupe(code, Date.now())) void submit(code);
    },
    [dedupe, submit],
  );

  const onManual = (e: FormEvent) => {
    e.preventDefault();
    void submit(manual).then(() => setManual(''));
  };

  return (
    <Page title={t('gate.title')} description={t('gate.description')}>
      <div className="grid gap-6 lg:grid-cols-2">
        <div className="min-w-0 space-y-6">
          <SectionCard title={t('gate.scanTitle')} icon={ScanQrCodeIcon}>
            <QrScanner
              onCode={onScan}
              paused={checkIn.isPending}
              labels={{
                start: t('gate.start'),
                stop: t('gate.stop'),
                preview: t('gate.preview'),
                scanning: t('gate.scanning'),
                unsupported: t('gate.unsupported'),
                denied: t('gate.denied'),
                error: t('gate.cameraError'),
              }}
            />
          </SectionCard>
          <SectionCard title={t('gate.manualTitle')} icon={KeyboardIcon}>
            <form noValidate onSubmit={onManual} className="space-y-2">
              <Label htmlFor={inputId}>{t('gate.code')}</Label>
              <div className="flex flex-col gap-2 sm:flex-row">
                <Input
                  id={inputId}
                  value={manual}
                  onChange={(e) => setManual(e.target.value)}
                  autoComplete="off"
                  autoCapitalize="characters"
                  spellCheck={false}
                  maxLength={200}
                  className="font-mono"
                />
                <Button
                  type="submit"
                  disabled={checkIn.isPending || extractTicketCode(manual) === ''}
                >
                  {checkIn.isPending && <Spinner />}
                  {t('gate.check')}
                </Button>
              </div>
            </form>
          </SectionCard>
        </div>
        <div className="min-w-0" aria-live="polite">
          {result ? (
            <GateResult
              result={result}
              name={(text) => localize(text, locale, defaultLocale)}
              time={fmt.time}
            />
          ) : (
            <Alert role="status">
              <ScanQrCodeIcon aria-hidden="true" />
              <AlertDescription>{t('gate.waiting')}</AlertDescription>
            </Alert>
          )}
        </div>
      </div>
    </Page>
  );
}

function GateResult({
  result,
  name,
  time,
}: {
  result: ScanResult;
  name: (text: Record<string, string>) => string;
  time: (iso: string | null | undefined) => string;
}) {
  const { t } = useTranslation('entryTickets');
  const { ticket } = result;
  const look = LOOK[result.outcome];
  const Icon = look.icon;
  const people = (ticket?.entry_items ?? []).reduce((n, i) => n + i.quantity, 0);
  return (
    <Alert
      variant={look.variant}
      role={result.outcome === 'admitted' ? 'status' : 'alert'}
      data-outcome={result.outcome}
    >
      <Icon aria-hidden="true" />
      <AlertTitle>{t(`gate.outcome.${result.outcome}`)}</AlertTitle>
      <AlertDescription>
        <div className="space-y-1">
          {ticket ? (
            <>
              <p className="text-2xl font-bold tabular-nums text-foreground">
                {ticket.number_label}
                <span className="ml-2 text-base font-normal text-muted-foreground">
                  {formatIsoDate(ticket.ticket_date)}
                </span>
              </p>
              {people > 0 && <p>{t('gate.people', { count: people })}</p>}
              {ticket.entry_items.length > 0 && (
                <ul className="list-inside list-disc">
                  {ticket.entry_items.map((i) => (
                    <li key={i.entry_ticket_type_id}>
                      {i.quantity} × {name(i.name)}
                    </li>
                  ))}
                </ul>
              )}
              {result.outcome === 'already_entered' && ticket.entered_at && (
                <p>{t('gate.enteredAt', { time: time(ticket.entered_at) })}</p>
              )}
              {result.outcome === 'voided' && ticket.void_reason && <p>{ticket.void_reason}</p>}
            </>
          ) : (
            <p className="font-mono">{result.code}</p>
          )}
          {result.message && <p>{result.message}</p>}
        </div>
      </AlertDescription>
    </Alert>
  );
}
