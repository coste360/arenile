import { useQueryClient, type QueryClient } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import type { components } from '../../api/generated/schema';
import {
  establishmentKeys,
  TOPIC_AREAS,
  type EstablishmentArea,
  type StreamTopicName,
} from './keys';

export type StreamMessage = components['schemas']['StreamMessage'];

/** Connection state of the real-time stream. */
export type StreamStatus = 'idle' | 'connecting' | 'open' | 'reconnecting';

/** Minimal EventSource surface used by the hook (injectable in tests). */
export interface EventSourceLike {
  readonly readyState: number;
  onopen: ((ev: Event) => unknown) | null;
  onerror: ((ev: Event) => unknown) | null;
  addEventListener(type: string, listener: (ev: MessageEvent) => void): void;
  close(): void;
}

export type EventSourceFactory = (url: string) => EventSourceLike;

export interface EstablishmentEventsOptions {
  /** Origin prefix of the API (default: same origin). */
  baseUrl?: string;
  /** Creates the EventSource (default: the browser EventSource with credentials). */
  createEventSource?: EventSourceFactory;
  /** Coalescing window of invalidations in ms (default 250). */
  debounceMs?: number;
  /** Called for every message (after the invalidations are scheduled). */
  onMessage?: (topic: StreamTopicName, message: StreamMessage) => void;
  /** Disable the stream (e.g. offline or tests). */
  enabled?: boolean;
}

const TOPICS = Object.keys(TOPIC_AREAS) as StreamTopicName[];
const CLOSED = 2;
const MAX_BACKOFF_MS = 30_000;

/** Backoff before reconnect attempt n (0-based): 1s, 2s, 4s ... capped at 30s, ±20% jitter. */
export function backoffDelay(attempt: number, random: () => number = Math.random): number {
  const base = Math.min(MAX_BACKOFF_MS, 1000 * 2 ** Math.min(attempt, 10));
  return Math.round(base * (0.8 + 0.4 * random()));
}

function defaultFactory(url: string): EventSourceLike {
  return new EventSource(url, { withCredentials: true });
}

/** Invalidates the given areas of one establishment. */
export function invalidateAreas(
  qc: QueryClient,
  establishmentId: string,
  areas: Iterable<EstablishmentArea>,
): void {
  for (const area of new Set(areas)) {
    void qc.invalidateQueries({ queryKey: establishmentKeys.area(establishmentId, area) });
  }
}

/**
 * Subscribes to the SSE stream of an establishment (spec 21.1) and
 * invalidates the TanStack Query keys of the areas touched by each message
 * (TOPIC_AREAS), coalescing bursts. The browser EventSource retries network
 * errors by itself; when the stream is closed (HTTP error, server restart)
 * the hook reconnects with exponential backoff and, once reopened,
 * invalidates every operational query (events may have been missed).
 * Messages carry no personal data: only ids, dates and spots.
 */
export function useEstablishmentEvents(
  establishmentId: string | undefined,
  options: EstablishmentEventsOptions = {},
): StreamStatus {
  const qc = useQueryClient();
  const [status, setStatus] = useState<StreamStatus>('connecting');
  const { baseUrl = '', createEventSource, debounceMs = 250, onMessage, enabled = true } = options;
  const onMessageRef = useRef(onMessage);
  useEffect(() => {
    onMessageRef.current = onMessage;
  }, [onMessage]);

  useEffect(() => {
    const factory =
      createEventSource ?? (typeof EventSource === 'undefined' ? undefined : defaultFactory);
    if (!establishmentId || !enabled || !factory) return undefined;
    const url = `${baseUrl}/api/admin/v1/stream?establishment=${encodeURIComponent(establishmentId)}`;
    let source: EventSourceLike | undefined;
    let attempt = 0;
    let everOpened = false;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;
    let flushTimer: ReturnType<typeof setTimeout> | undefined;
    let stopped = false;
    const pending = new Set<EstablishmentArea>();

    const flush = () => {
      flushTimer = undefined;
      invalidateAreas(qc, establishmentId, pending);
      pending.clear();
    };
    const schedule = (areas: readonly EstablishmentArea[]) => {
      for (const a of areas) pending.add(a);
      if (pending.size > 0 && flushTimer === undefined) flushTimer = setTimeout(flush, debounceMs);
    };

    const connect = () => {
      if (stopped) return;
      const es = factory(url);
      source = es;
      es.onopen = () => {
        if (everOpened || attempt > 0) {
          void qc.invalidateQueries({ queryKey: establishmentKeys.all(establishmentId) });
        }
        everOpened = true;
        attempt = 0;
        setStatus('open');
      };
      es.onerror = () => {
        if (stopped) return;
        if (es.readyState === CLOSED) {
          es.close();
          setStatus('reconnecting');
          reconnectTimer = setTimeout(connect, backoffDelay(attempt));
          attempt += 1;
        } else {
          setStatus('reconnecting');
        }
      };
      for (const topic of TOPICS) {
        es.addEventListener(topic, (ev: MessageEvent) => {
          let msg: StreamMessage | undefined;
          try {
            msg = JSON.parse(String(ev.data)) as StreamMessage;
          } catch {
            return;
          }
          if (msg.establishment_id && msg.establishment_id !== establishmentId) return;
          schedule(TOPIC_AREAS[topic]);
          onMessageRef.current?.(topic, msg);
        });
      }
    };

    connect();
    return () => {
      stopped = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      if (flushTimer) clearTimeout(flushTimer);
      source?.close();
    };
  }, [qc, establishmentId, baseUrl, createEventSource, debounceMs, enabled]);

  const active =
    !!establishmentId && enabled && (!!createEventSource || typeof EventSource !== 'undefined');
  return active ? status : 'idle';
}
