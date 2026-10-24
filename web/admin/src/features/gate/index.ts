// Public surface of the gate scanner: a generic camera QR reader
// (BarcodeDetector) used by the entry gate and reusable for other check-ins.
export { canScanQr, getBarcodeDetector, type BarcodeDetectorCtor } from './barcode';
export { QrScanner, type QrScannerLabels, type QrScannerProps } from './QrScanner';
export { useQrScanner, type ScannerStatus } from './useQrScanner';
