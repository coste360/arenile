import { afterEach, describe, expect, it, vi } from 'vitest';
import { printZReport, zReportUrl } from './zreport';

describe('zReportUrl', () => {
  it('builds the same-origin URL with the paper', () => {
    expect(zReportUrl('e 1', 's1')).toBe(
      '/api/admin/v1/establishments/e%201/cash-sessions/s1/z-report?paper=a4',
    );
    expect(zReportUrl('e1', 's1', 'receipt80')).toContain('paper=receipt80');
  });
});

describe('printZReport', () => {
  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('prints the document loaded in a hidden iframe', async () => {
    const promise = printZReport('about:blank');
    const frame = document.querySelector('iframe') as HTMLIFrameElement;
    expect(frame).not.toBeNull();
    expect(frame.getAttribute('aria-hidden')).toBe('true');
    const win = frame.contentWindow as Window;
    win.document.body.innerHTML = '<header class="doc"></header>';
    const print = vi.fn();
    Object.defineProperty(win, 'print', { value: print, configurable: true });
    frame.onload?.(new Event('load'));
    await promise;
    expect(print).toHaveBeenCalledOnce();
  });

  it('rejects when the response is not a Z report (e.g. an error page)', async () => {
    const promise = printZReport('about:blank');
    const frame = document.querySelector('iframe') as HTMLIFrameElement;
    frame.onload?.(new Event('load'));
    await expect(promise).rejects.toThrow();
    expect(document.querySelector('iframe')).toBeNull();
  });
});
