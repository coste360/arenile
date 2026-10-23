import { useMe } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@arenile/shared/ui/card';
import { PageHeader, QueryState } from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import {
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
import { fingerprintLines, hostOf, panelAddress } from './lib';

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
  const header = <PageHeader title={t('title')} description={t('description')} />;

  if (meta.isPending || meta.error) {
    return (
      <section aria-labelledby="page-title" className="mx-auto max-w-3xl space-y-6">
        {header}
        <QueryState
          isLoading={meta.isPending}
          error={meta.error}
          onRetry={() => void meta.refetch()}
        />
      </section>
    );
  }

  if (mode !== 'lan' || !lanUrl) {
    const desktop = mode === 'local';
    return (
      <section aria-labelledby="page-title" className="mx-auto max-w-3xl space-y-6">
        {header}
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
      </section>
    );
  }

  return (
    <LanDetails
      establishmentId={establishmentId}
      lanUrl={lanUrl}
      fingerprint={fingerprint}
      qrSvg={qrSvg}
    />
  );
}

interface LanDetailsProps {
  establishmentId: string;
  lanUrl: string;
  fingerprint: string;
  /** QR (SVG) of the panel address for phone cameras (meta lan_qr_svg). */
  qrSvg?: string;
}

function LanDetails({ establishmentId, lanUrl, fingerprint, qrSvg }: LanDetailsProps) {
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
    <section aria-labelledby="page-title" className="mx-auto max-w-3xl space-y-6">
      <PageHeader
        title={t('title')}
        description={t('description')}
        actions={
          <Button onClick={() => setPairOpen(true)}>
            <QrCodeIcon aria-hidden="true" />
            {t('pair.open')}
          </Button>
        }
      />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <WifiIcon aria-hidden="true" className="size-4" />
            {t('address.title')}
          </CardTitle>
          <CardDescription>{t('address.description')}</CardDescription>
          <CardAction>
            <Button
              variant="outline"
              size="sm"
              onClick={() => void copy(address)}
              aria-label={t('address.copy')}
            >
              <CopyIcon aria-hidden="true" />
              <span className="hidden sm:inline">{t('address.copy')}</span>
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent className="flex flex-col gap-4 sm:flex-row sm:items-center">
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
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheckIcon aria-hidden="true" className="size-4" />
            {t('fingerprint.title')}
          </CardTitle>
          <CardDescription>{t('fingerprint.description')}</CardDescription>
          <CardAction>
            <Button
              variant="outline"
              size="sm"
              onClick={() => void copy(fingerprint)}
              aria-label={t('fingerprint.copy')}
            >
              <CopyIcon aria-hidden="true" />
              <span className="hidden sm:inline">{t('fingerprint.copy')}</span>
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent>
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
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <GlobeLockIcon aria-hidden="true" className="size-4" />
            {t('browser.title')}
          </CardTitle>
          <CardDescription>{t('browser.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <ol className="list-decimal space-y-2 pl-5 text-sm">
            <li>{t('browser.step1')}</li>
            <li>{t('browser.step2', { address })}</li>
            <li>{t('browser.step3')}</li>
            <li>{t('browser.step4')}</li>
            <li>{t('browser.step5')}</li>
          </ol>
          <p className="mt-4 text-sm text-muted-foreground">{t('browser.hint')}</p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <QrCodeIcon aria-hidden="true" className="size-4" />
            {t('apps.title')}
          </CardTitle>
          <CardDescription>{t('apps.description')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <p>{t('apps.security')}</p>
          <p className="text-muted-foreground">{t('apps.guarantee')}</p>
        </CardContent>
      </Card>

      <PairDeviceDialog
        open={pairOpen}
        onOpenChange={setPairOpen}
        establishmentId={establishmentId}
        selfId={me.data?.user?.id}
        canManageUsers={false}
        knownDevices={devices.data ?? []}
      />
    </section>
  );
}
