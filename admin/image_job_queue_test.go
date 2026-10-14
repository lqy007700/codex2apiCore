package admin

// Saturated key must not reject durable admission or download references.

// Reconstructing the handler must no longer fail queued jobs.

// A key saturated by its own long-lived requests must not park workers on its
// backlog: its jobs stay queued while other keys' jobs keep executing.

// Keep observing a few dispatch ticks.

// created_at has second precision.

// Exercise the real dispatcher, input loading, in-process proxy and fenced
// terminal write without making a billable upstream request (no accounts).
