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
