import { useMe } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Page, QueryState, SectionCard } from '@arenile/shared/ui/patterns';
import { formatInstantDate } from '@arenile/shared/lib/dates';
import { Link } from '@tanstack/react-router';
import {
  CalendarClockIcon,
  CopyIcon,
  GlobeLockIcon,
  QrCodeIcon,
  ShieldCheckIcon,
  WifiIcon,
  WifiOffIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useRequiredEstablishmentId } from '../../../establishment';
import { panelMode, useInstanceMeta } from '../../../lib/meta';
import { useDevices } from '../api';
import { PairDeviceDialog } from '../PairDeviceDialog';
import { certRenewalSoon, fingerprintLines, hostOf, panelAddress } from './lib';

/**
 * Configurazione → Dispositivi → Rete locale (spec 29.1): on the desktop
 * edition with the LAN mode on, shows the address other devices use, the
 * SHA-256 fingerprint of the self-signed certificate and how to accept it
 * in a browser, and pairs the mobile app / a desktop app in remote mode
 * (their QR carries the fingerprint, so they need no warning). The tray
 * item "Mostra QR rete locale" opens this page.
 */
export function LanInfoPage() {
  const { t } = useTranslation('devicesLan');
  const establishmentId = useRequiredEstablishmentId();
  const meta = useInstanceMeta();
  const mode = panelMode(meta.data);
  const lanUrl = meta.data?.desktop?.lan_url;
  const fingerprint = meta.data?.desktop?.lan_cert_fingerprint ?? '';
  const qrSvg = meta.data?.desktop?.lan_qr_svg;
  const renewsAt = meta.data?.desktop?.lan_cert_renews_at;

  if (meta.isPending || meta.error) {
    return (
      <Page title={t('title')} description={t('description')}>
        <QueryState
          isLoading={meta.isPending}
          error={meta.error}
          onRetry={() => void meta.refetch()}
        />
      </Page>
    );
  }

  if (mode !== 'lan' || !lanUrl) {
    const desktop = mode === 'local';
    return (
      <Page title={t('title')} description={t('description')}>
        <Alert role="note">
          <WifiOffIcon aria-hidden="true" />
          <AlertTitle>{desktop ? t('off.title') : t('server.title')}</AlertTitle>
          <AlertDescription>
            <p>{desktop ? t('off.description') : t('server.description')}</p>
            <Button asChild variant="outline" size="sm" className="mt-3">
              {desktop ? (
                <Link to="/e/$establishmentId/config/desktop" params={{ establishmentId }}>
                  {t('off.action')}
                </Link>
              ) : (
                <Link to="/e/$establishmentId/config/devices" params={{ establishmentId }}>
                  {t('server.action')}
                </Link>
              )}
            </Button>
          </AlertDescription>
        </Alert>
      </Page>
    );
  }

  return (
    <LanDetails
      establishmentId={establishmentId}
      lanUrl={lanUrl}
      fingerprint={fingerprint}
      qrSvg={qrSvg}
      renewsAt={renewsAt}
    />
  );
}

interface LanDetailsProps {
  establishmentId: string;
  lanUrl: string;
  fingerprint: string;
  /** QR (SVG) of the panel address for phone cameras (meta lan_qr_svg). */
  qrSvg?: string;
  /** When the certificate will be regenerated (meta lan_cert_renews_at). */
  renewsAt?: string;
}

function LanDetails({ establishmentId, lanUrl, fingerprint, qrSvg, renewsAt }: LanDetailsProps) {
  const { t } = useTranslation('devicesLan');
  const [pairOpen, setPairOpen] = useState(false);
  const me = useMe();
  const devices = useDevices(establishmentId, { includeRevoked: false });

  const copy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(t('copied'));
    } catch {
      // Clipboard denied: the text stays selectable.
    }
  };

  const address = panelAddress(lanUrl);
  const lines = fingerprintLines(fingerprint);
  return (
    <Page
      title={t('title')}
      description={t('description')}
      actions={
        <Button onClick={() => setPairOpen(true)}>
          <QrCodeIcon aria-hidden="true" />
          {t('pair.open')}
        </Button>
      }
    >
      <SectionCard
        icon={WifiIcon}
        title={t('address.title')}
        description={t('address.description')}
        actions={
          <Button
            variant="outline"
            onClick={() => void copy(address)}
            aria-label={t('address.copy')}
          >
            <CopyIcon aria-hidden="true" />
            <span className="hidden sm:inline">{t('address.copy')}</span>
          </Button>
        }
      >
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
          {qrSvg && (
            <img
              src={`data:image/svg+xml;charset=utf-8,${encodeURIComponent(qrSvg)}`}
              alt={t('address.qrAlt', { address })}
              className="size-40 shrink-0 self-center rounded-md border bg-background p-2"
            />
          )}
          <div className="min-w-0 space-y-2">
            <p className="font-mono text-2xl font-semibold break-all" data-testid="lan-host">
              {hostOf(lanUrl)}
            </p>
            <p className="font-mono text-sm break-all text-muted-foreground">{address}</p>
            {qrSvg && <p className="text-sm text-muted-foreground">{t('address.qrHint')}</p>}
          </div>
        </div>
      </SectionCard>

      <SectionCard
        icon={ShieldCheckIcon}
        title={t('fingerprint.title')}
        description={t('fingerprint.description')}
        actions={
          <Button
            variant="outline"
            onClick={() => void copy(fingerprint)}
            aria-label={t('fingerprint.copy')}
          >
            <CopyIcon aria-hidden="true" />
            <span className="hidden sm:inline">{t('fingerprint.copy')}</span>
          </Button>
        }
      >
        <div
          role="group"
          className="rounded-md bg-muted p-3 font-mono text-sm break-all"
          aria-label={t('fingerprint.aria')}
        >
          {lines.map((line, i) => (
            // Lines can repeat (same bytes): the position is the identity.
            <div key={`${i}-${line}`}>{line}</div>
          ))}
        </div>
      </SectionCard>

      {certRenewalSoon(renewsAt) && (
        <Alert role="note" data-testid="lan-cert-renewal">
          <CalendarClockIcon aria-hidden="true" />
          <AlertTitle>{t('renewal.title', { date: formatInstantDate(renewsAt) })}</AlertTitle>
          <AlertDescription>{t('renewal.description')}</AlertDescription>
        </Alert>
      )}

      <SectionCard
        icon={GlobeLockIcon}
        title={t('browser.title')}
        description={t('browser.description')}
      >
        <ol className="list-decimal space-y-2 pl-5 text-sm">
          <li>{t('browser.step1')}</li>
          <li>{t('browser.step2', { address })}</li>
          <li>{t('browser.step3')}</li>
          <li>{t('browser.step4')}</li>
          <li>{t('browser.step5')}</li>
        </ol>
        <p className="mt-4 text-sm text-muted-foreground">{t('browser.hint')}</p>
      </SectionCard>

      <SectionCard icon={QrCodeIcon} title={t('apps.title')} description={t('apps.description')}>
        <div className="space-y-3 text-sm">
          <p>{t('apps.security')}</p>
          <p className="text-muted-foreground">{t('apps.guarantee')}</p>
        </div>
      </SectionCard>

      <PairDeviceDialog
        open={pairOpen}
        onOpenChange={setPairOpen}
        establishmentId={establishmentId}
        selfId={me.data?.user?.id}
        canManageUsers={false}
        knownDevices={devices.data ?? []}
      />
    </Page>
  );
}
