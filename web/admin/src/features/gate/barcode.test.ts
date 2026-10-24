import { describe, expect, it } from 'vitest';
import { canScanQr, getBarcodeDetector } from './barcode';

class FakeDetector {
  static formats = ['qr_code', 'ean_13'];
  static getSupportedFormats = () => Promise.resolve(FakeDetector.formats);
  detect() {
    return Promise.resolve([]);
  }
}

const camera = { mediaDevices: { getUserMedia: () => Promise.resolve({}) } };

describe('QR scanner support', () => {
  it('falls back to manual entry without BarcodeDetector or camera API', async () => {
    expect(getBarcodeDetector({})).toBeUndefined();
    expect(await canScanQr({ navigator: camera })).toBe(false);
    expect(await canScanQr({ BarcodeDetector: FakeDetector, navigator: {} })).toBe(false);
  });

  it('scans when the detector supports qr_code and the camera API exists', async () => {
    expect(await canScanQr({ BarcodeDetector: FakeDetector, navigator: camera })).toBe(true);
  });

  it('refuses a detector without qr_code support', async () => {
    class NoQr extends FakeDetector {
      static override getSupportedFormats = () => Promise.resolve(['ean_13']);
    }
    expect(await canScanQr({ BarcodeDetector: NoQr, navigator: camera })).toBe(false);
  });
});
