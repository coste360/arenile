export interface HoldCountdownProps {
  /** RFC 3339 hold expiry from the server. */
  expiresAt: string;
  onExpire: () => void;
  /** Current time, injectable for tests. */
  now?: () => number;
}

/** Visible countdown of the hold (spec 12.3 passo 4), aria-live polite. Owner: booking app builder. */
export function HoldCountdown(_props: HoldCountdownProps) {
  return null;
}
