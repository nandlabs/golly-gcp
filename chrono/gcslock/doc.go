// Package gcslock provides a chrono.LeaderElector backed by a single
// Cloud Storage object. Leadership is coordinated by GCS's generation
// preconditions: DoesNotExist for first-claim, GenerationMatch for
// renew and steal, so at most one caller can commit a given transition.
//
// The elector runs a background renewal goroutine at Lease/2 cadence.
// The persisted body is a small JSON blob carrying the current
// {owner, lease_until}. IsLeader reads a local atomic — no I/O — so it
// is safe from hot paths.
//
// GCS provides strong consistency for object metadata (including
// generation numbers), so a losing writer's Close returns 412
// PreconditionFailed exactly when someone else won the race. The
// elector treats that outcome as "not leader this tick" and lets the
// next tick observe the fresh state.
//
// Clock skew: lease_until is a wall-clock timestamp, so instances SHOULD
// keep clock skew below Lease/2 for correctness.
package gcslock
