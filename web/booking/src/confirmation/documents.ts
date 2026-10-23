/** Saves a blob as a file through a temporary object URL (works inside iframes without sandbox). */
export function saveBlob(blob: Blob, filename: string, doc: Document = document): void {
  const url = URL.createObjectURL(blob);
  const a = doc.createElement('a');
  a.href = url;
  a.download = filename;
  a.rel = 'noopener';
  doc.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 30_000);
}

/**
 * QR SVG from the server as an <img> source. A data URL in an <img> never
 * runs scripts, so the server markup is not injected into the page DOM.
 */
export function svgDataUrl(svg: string): string {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
}
