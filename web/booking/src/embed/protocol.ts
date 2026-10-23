/** postMessage protocol between the booking iframe and embed.js (spec 12.1). */
export const MESSAGE_TYPES = ['arenile:resize', 'arenile:booked', 'arenile:navigate'] as const;
export type MessageType = (typeof MESSAGE_TYPES)[number];

export interface ResizeMessage {
  type: 'arenile:resize';
  height: number;
}

export interface BookedMessage {
  type: 'arenile:booked';
  /** Public booking code, no personal data. */
  code: string;
}

export interface NavigateMessage {
  type: 'arenile:navigate';
  /** Absolute http(s) URL the host page should open at top level (e.g. payment). */
  url: string;
}

export type ArenileMessage = ResizeMessage | BookedMessage | NavigateMessage;

/**
 * Admin panel ↔ theme preview (spec 12.5: "l'anteprima nel pannello è la
 * pagina reale in modalità prova"). The panel frames `/b/{slug}?embed=1&preview=1`
 * on the SAME origin and posts the theme being edited; the page announces
 * itself when ready. Never exchanged with third-party hosts.
 */
export interface PreviewReadyMessage {
  type: 'arenile:preview-ready';
}

export interface PreviewThemeMessage {
  type: 'arenile:preview-theme';
  /** Theme as edited (not yet saved), same shape as the API `Theme`. */
  theme: Record<string, unknown>;
}

export type PreviewMessage = PreviewReadyMessage | PreviewThemeMessage;
