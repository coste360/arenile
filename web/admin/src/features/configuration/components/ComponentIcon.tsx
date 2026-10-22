import type { ReactNode } from 'react';
import { iconKindFor } from '../lib/componentIcons';

const PATHS: Record<string, ReactNode> = {
  umbrella: (
    <>
      <path d="M3 12a9 9 0 0 1 18 0Z" />
      <path d="M12 12v8a2 2 0 0 1-4 0" />
    </>
  ),
  sunbed: (
    <>
      <path d="M3 15h13l5-6" />
      <path d="M5 15v4M15 15v4" />
    </>
  ),
  deckchair: (
    <>
      <path d="M7 4l8 16M17 10l-10 10" />
      <path d="M9 9h7" />
    </>
  ),
  chair: (
    <>
      <path d="M7 3v10h10M7 13v8M17 13v8" />
    </>
  ),
  cabin: (
    <>
      <path d="M4 10l8-6 8 6v10H4Z" />
      <path d="M10 20v-6h4v6" />
    </>
  ),
  gazebo: (
    <>
      <path d="M2 10 12 4l10 6Z" />
      <path d="M5 10v10M19 10v10" />
    </>
  ),
  tent: (
    <>
      <path d="M3 20 12 4l9 16Z" />
      <path d="M12 20v-6" />
    </>
  ),
  table: (
    <>
      <path d="M3 9h18M6 9v11M18 9v11" />
    </>
  ),
  generic: (
    <>
      <rect x="5" y="5" width="14" height="14" rx="3" />
    </>
  ),
};

/** Small line icon of an inventory component, chosen from its code. */
export function ComponentIcon({ code, className }: { code: string; className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      width="20"
      height="20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      className={className}
      data-icon={iconKindFor(code)}
    >
      {PATHS[iconKindFor(code)]}
    </svg>
  );
}
