import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Spinner } from '@arenile/shared/ui/spinner';
import { CameraIcon, CameraOffIcon, ScanLineIcon } from 'lucide-react';
import { useQrScanner, type ScannerStatus } from './useQrScanner';

export interface QrScannerLabels {
  start: string;
  stop: string;
  /** Accessible name of the live camera preview. */
  preview: string;
  scanning: string;
  unsupported: string;
  denied: string;
  error: string;
}

export interface QrScannerProps {
  onCode: (raw: string) => void;
  /** Pause detection (e.g. while the previous scan is being checked). */
  paused?: boolean;
  labels: QrScannerLabels;
}

const NOTICE: Partial<Record<ScannerStatus, keyof QrScannerLabels>> = {
  unsupported: 'unsupported',
  denied: 'denied',
  error: 'error',
};

/**
 * Camera QR scanner (BarcodeDetector) for staff phones. Generic: the caller
 * passes translated labels and handles the code; where the browser cannot
 * scan it shows a notice and the caller's manual entry stays the way in.
 */
export function QrScanner({ onCode, paused, labels }: QrScannerProps) {
  const { videoRef, status, start, stop } = useQrScanner({ onCode, paused });
  const live = status === 'scanning' || status === 'starting';
  const notice = NOTICE[status];
  return (
    <div className="space-y-3">
      {notice && (
        <Alert variant={status === 'unsupported' ? 'default' : 'warning'} role="status">
          <CameraOffIcon aria-hidden="true" />
          <AlertDescription>{labels[notice]}</AlertDescription>
        </Alert>
      )}
      <div className={live ? 'relative overflow-hidden rounded-lg border bg-muted' : 'hidden'}>
        <video
          ref={videoRef}
          aria-label={labels.preview}
          className="aspect-square w-full object-cover sm:aspect-video"
          muted
          playsInline
        />
        {status === 'scanning' && (
          <p className="absolute inset-x-0 bottom-0 flex items-center justify-center gap-2 bg-background/80 py-2 text-sm">
            <ScanLineIcon aria-hidden="true" className="size-4" />
            {labels.scanning}
          </p>
        )}
      </div>
      {status !== 'unsupported' && status !== 'checking' && (
        <div className="flex flex-wrap gap-2">
          {live ? (
            <Button variant="outline" className="w-full sm:w-auto" onClick={stop}>
              <CameraOffIcon aria-hidden="true" />
              {labels.stop}
            </Button>
          ) : (
            <Button className="w-full sm:w-auto" onClick={() => void start()}>
              <CameraIcon aria-hidden="true" />
              {labels.start}
            </Button>
          )}
          {status === 'starting' && <Spinner />}
        </div>
      )}
    </div>
  );
}
