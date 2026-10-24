import { useCallback, useEffect, useRef, useState } from 'react';
import { canScanQr, getBarcodeDetector, type BarcodeDetectorLike } from './barcode';

export type ScannerStatus =
  /** Checking whether the browser can scan. */
  | 'checking'
  /** No BarcodeDetector or no camera API: manual entry only. */
  | 'unsupported'
  /** Ready, camera off. */
  | 'idle'
  | 'starting'
  | 'scanning'
  /** Camera permission refused. */
  | 'denied'
  /** No camera or the camera failed. */
  | 'error';

export interface UseQrScannerOptions {
  /** Called with the raw QR text; scanning pauses while `paused` is true. */
  onCode: (raw: string) => void;
  paused?: boolean;
  /** Milliseconds between two detections (battery friendly). */
  intervalMs?: number;
}

/**
 * Rear camera + BarcodeDetector loop for the gate (staff phone). The stream
 * starts only on an explicit `start()` (a user gesture, as iOS requires) and
 * is always stopped on `stop()` and on unmount.
 */
export function useQrScanner({ onCode, paused = false, intervalMs = 250 }: UseQrScannerOptions) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const detectorRef = useRef<BarcodeDetectorLike | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const onCodeRef = useRef(onCode);
  const pausedRef = useRef(paused);
  const [status, setStatus] = useState<ScannerStatus>('checking');

  useEffect(() => {
    onCodeRef.current = onCode;
    pausedRef.current = paused;
  }, [onCode, paused]);

  useEffect(() => {
    let alive = true;
    void canScanQr().then((ok) => {
      if (alive) setStatus(ok ? 'idle' : 'unsupported');
    });
    return () => {
      alive = false;
    };
  }, []);

  const stop = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = null;
    streamRef.current?.getTracks().forEach((track) => track.stop());
    streamRef.current = null;
    if (videoRef.current) videoRef.current.srcObject = null;
    setStatus((s) => (s === 'scanning' || s === 'starting' ? 'idle' : s));
  }, []);

  /** Detection loop: one detect() at a time, then wait `intervalMs`, until stopped. */
  const loop = useCallback(() => {
    const tick = () => {
      if (!streamRef.current) return;
      const video = videoRef.current;
      const detector = detectorRef.current;
      const next = () => {
        if (streamRef.current) timerRef.current = setTimeout(tick, intervalMs);
      };
      if (!video || !detector || pausedRef.current || video.readyState < 2) {
        next();
        return;
      }
      detector
        .detect(video)
        .then((codes) => {
          const value = codes.find((c) => c.rawValue.trim() !== '')?.rawValue;
          if (value && !pausedRef.current) onCodeRef.current(value);
        })
        .catch(() => {
          // A frame that cannot be decoded: try the next one.
        })
        .finally(next);
    };
    tick();
  }, [intervalMs]);

  const start = useCallback(async () => {
    const Detector = getBarcodeDetector();
    if (!Detector || !navigator.mediaDevices?.getUserMedia) {
      setStatus('unsupported');
      return;
    }
    setStatus('starting');
    try {
      detectorRef.current ??= new Detector({ formats: ['qr_code'] });
      const stream = await navigator.mediaDevices.getUserMedia({
        video: { facingMode: { ideal: 'environment' } },
        audio: false,
      });
      streamRef.current = stream;
      const video = videoRef.current;
      if (video) {
        video.srcObject = stream;
        await video.play().catch(() => undefined);
      }
      setStatus('scanning');
      loop();
    } catch (err) {
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
      const name = err instanceof DOMException ? err.name : '';
      setStatus(name === 'NotAllowedError' || name === 'SecurityError' ? 'denied' : 'error');
    }
  }, [loop]);

  useEffect(() => stop, [stop]);

  return { videoRef, status, start, stop };
}
