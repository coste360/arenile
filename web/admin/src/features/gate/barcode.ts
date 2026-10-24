/**
 * Minimal typing of the Shape Detection API (BarcodeDetector), which the DOM
 * lib of TypeScript does not ship yet. Supported by Chrome/Edge on Android and
 * desktop and by Safari 17+ behind the platform; elsewhere the gate falls back
 * to typing the code.
 */
export interface DetectedBarcode {
  rawValue: string;
  format: string;
}

export interface BarcodeDetectorLike {
  detect(source: CanvasImageSource): Promise<DetectedBarcode[]>;
}

export interface BarcodeDetectorCtor {
  new (options?: { formats?: string[] }): BarcodeDetectorLike;
  getSupportedFormats?: () => Promise<string[]>;
}

/** The BarcodeDetector constructor of this browser, if any (injectable for tests). */
export function getBarcodeDetector(
  scope: unknown = typeof globalThis === 'undefined' ? undefined : globalThis,
): BarcodeDetectorCtor | undefined {
  const ctor = (scope as { BarcodeDetector?: unknown } | undefined)?.BarcodeDetector;
  return typeof ctor === 'function' ? (ctor as BarcodeDetectorCtor) : undefined;
}

/** True when the camera can be used to scan QR codes here (detector + getUserMedia). */
export async function canScanQr(
  scope: unknown = typeof globalThis === 'undefined' ? undefined : globalThis,
): Promise<boolean> {
  const ctor = getBarcodeDetector(scope);
  const media = (scope as { navigator?: Navigator } | undefined)?.navigator?.mediaDevices;
  if (!ctor || typeof media?.getUserMedia !== 'function') return false;
  if (!ctor.getSupportedFormats) return true;
  try {
    return (await ctor.getSupportedFormats()).includes('qr_code');
  } catch {
    return false;
  }
}
