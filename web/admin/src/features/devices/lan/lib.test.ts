import { describe, expect, it } from 'vitest';
import { fingerprintLines, hostOf, panelAddress } from './lib';

const FP = Array.from({ length: 32 }, (_, i) => i.toString(16).padStart(2, '0')).join(':');

describe('LAN helpers', () => {
  it('builds the panel address', () => {
    expect(panelAddress('https://192.168.1.20:8443')).toBe('https://192.168.1.20:8443/admin/');
    expect(panelAddress('https://cassa.local:8443/')).toBe('https://cassa.local:8443/admin/');
  });

  it('splits the fingerprint in four lines of eight bytes', () => {
    const lines = fingerprintLines(FP);
    expect(lines).toHaveLength(4);
    expect(lines[0]).toBe('00:01:02:03:04:05:06:07');
    expect(lines[3]).toBe('18:19:1A:1B:1C:1D:1E:1F');
    expect(fingerprintLines('')).toEqual([]);
  });

  it('extracts the host', () => {
    expect(hostOf('https://192.168.1.20:8443')).toBe('192.168.1.20:8443');
    expect(hostOf('not a url')).toBe('not a url');
  });
});
