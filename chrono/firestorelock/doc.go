// Package firestorelock provides a chrono.LeaderElector backed by a Cloud
// Firestore document. Leadership is coordinated through a single lock
// document whose ownership is renewed via serializable transactions.
//
// The elector runs a background goroutine that periodically (Lease/2)
// attempts to claim, renew, or steal an expired lease inside a Firestore
// transaction. Firestore transactions provide strongly consistent
// serializable reads and writes, so at most one caller can win the
// contested update; the losing caller simply observes fresh state on the
// next tick.
//
// The public IsLeader call performs no I/O — it reads a local atomic
// updated by the renewal loop, so it is safe to invoke from hot paths
// (e.g. per-tick from the scheduler).
//
// Clock skew: because the lock is expressed as an absolute wall-clock
// deadline (lease_until), instances SHOULD keep clock skew below Lease/2
// for correctness. Chrony/NTP with a modest sync interval is sufficient.
package firestorelock
