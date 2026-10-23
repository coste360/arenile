// Package sse serves the Server-Sent Events stream of an establishment to
// the admin panel (spec 21.1): GET /api/admin/v1/stream?establishment=.
//
// # Flow
//
// Domain services write events to the outbox in the transaction of the
// change; the outbox dispatcher (internal/domain/event) publishes them on
// the ext.EventBus only after commit, and Server forwards them to the
// subscribed browsers. Nothing is ever streamed for a rolled-back change.
//
// Authentication is the session cookie (or bearer key) handled by the admin
// middleware; the handler resolves the establishment through the
// principal's scope before calling Serve, so a stream is always scoped to an
// establishment the caller can read (cross-tenant requests get 404).
//
// # Wire format
//
// Every message is
//
//	id: <event id>
//	event: <topic>            booking.changed, allocation.changed, hold.changed,
//	                          cash.changed, customer.changed, config.changed, event
//	data: <StreamMessage>     {id, topic, type, establishment_id, occurred_at,
//	                           booking_id?, spot_ids?, dates?}
//
// The data is built by event.ToStreamMessage, which copies only ids, dates
// and spots out of the payload: names, emails, phones, notes and amounts are
// never streamed (spec 21.1, 23). A ": ping" comment is sent every
// Heartbeat to keep proxies from closing idle connections.
//
// # Resume
//
// When the bus implements Replayer (ReplayBus, a decorator over any
// ext.EventBus keeping the last events per establishment), a client that
// reconnects with Last-Event-ID first receives the events it missed, then
// the live stream, deduplicated by id. When the id is unknown the stream
// starts live and the client refetches (the panel invalidates every
// operational query on reconnect anyway).
//
// # Backpressure
//
// Publishers never block: the broker drops messages for a full subscriber
// buffer. Server turns that into a clean reconnect instead of a silent gap:
// a subscriber whose buffer is found full, or whose socket does not accept a
// write within WriteTimeout, is disconnected (Server.Disconnected counts
// them); the browser reconnects and resumes or refetches.
package sse
