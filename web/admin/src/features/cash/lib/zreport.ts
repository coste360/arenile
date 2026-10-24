/**
 * Printable Z closing (ADR 0019): the server renders a self-contained HTML
 * document with print styles (A4 or 80 mm receipt) and the mono Arenile
 * logo; the PDF is the browser's print ("Save as PDF"). The panel prints it
 * from a hidden same-origin iframe, or opens it in a new tab.
 */
export type ZPaper = 'a4' | 'receipt80';

/** Relative URL of the printable Z of a session (same origin, session cookie). */
export function zReportUrl(
  establishmentId: string,
  sessionId: string,
  paper: ZPaper = 'a4',
): string {
  const params = new URLSearchParams({ paper });
  return `/api/admin/v1/establishments/${encodeURIComponent(establishmentId)}/cash-sessions/${encodeURIComponent(sessionId)}/z-report?${params.toString()}`;
}

/**
 * Loads the document in a hidden iframe and opens the print dialog on it.
 * Resolves after the dialog was opened; rejects when the document cannot be
 * loaded (the caller then offers the new-tab link). The iframe is removed
 * after printing.
 */
export function printZReport(url: string, doc: Document = document): Promise<void> {
  return new Promise((resolve, reject) => {
    const frame = doc.createElement('iframe');
    frame.title = 'z-report';
    frame.setAttribute('aria-hidden', 'true');
    frame.tabIndex = -1;
    // Invisible but laid out (some browsers do not print display:none frames).
    frame.className = 'pointer-events-none fixed top-0 left-0 size-px opacity-0';
    const cleanup = () => {
      window.setTimeout(() => frame.remove(), 1000);
    };
    frame.onload = () => {
      const win = frame.contentWindow;
      try {
        if (!win || !win.document.querySelector('header.doc')) throw new Error('not a z report');
        win.focus();
        win.addEventListener('afterprint', cleanup, { once: true });
        win.print();
        resolve();
      } catch (err) {
        frame.remove();
        reject(err instanceof Error ? err : new Error(String(err)));
      }
    };
    frame.onerror = () => {
      frame.remove();
      reject(new Error('z report not loaded'));
    };
    frame.src = url;
    doc.body.appendChild(frame);
  });
}
