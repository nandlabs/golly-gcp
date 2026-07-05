# firestorelock

A `chrono.LeaderElector` backed by a single Cloud Firestore document.
Ownership is coordinated by serializable transactions: every renewal
tick reads the current `{owner, lease_until}`, decides claim/renew/skip,
and writes back — atomically — in one `RunTransaction` call.

## Quick start

```go
import (
    "cloud.google.com/go/firestore"
    "oss.nandlabs.io/golly-gcp/chrono/firestorelock"
    "oss.nandlabs.io/golly/chrono"
)

client, _ := firestore.NewClient(ctx, "my-project")
elector := firestorelock.New(client, firestorelock.Options{
    Collection: "chrono-locks",
    DocID:      "scheduler",
    Owner:      hostnamePidRandom(),
    Lease:      15 * time.Second,
})
_ = elector.Start(ctx)
defer elector.Resign(context.Background())

sched := chrono.NewScheduler(chrono.WithLeaderElector(elector))
```

`IsLeader` reads a local atomic — safe to call in hot paths.

**Clock skew:** the lock uses absolute `lease_until` timestamps, so
instance clocks SHOULD stay within `Lease/2` of one another (chrony/NTP
with the default sync interval is sufficient).
