# gcslock

A `chrono.LeaderElector` backed by a single Cloud Storage object.
Ownership is coordinated by GCS's generation preconditions:
`DoesNotExist` for the first claim and `GenerationMatch` for every
renew, steal, and resign. GCS provides strong consistency for these
preconditions, so exactly one contender wins any contested transition.

## Quick start

```go
import (
    "cloud.google.com/go/storage"
    "oss.nandlabs.io/golly-gcp/chrono/gcslock"
    "oss.nandlabs.io/golly/chrono"
)

client, _ := storage.NewClient(ctx)
elector := gcslock.New(client, gcslock.Options{
    Bucket: "my-locks",
    Object: "chrono/scheduler.lock",
    Owner:  hostnamePidRandom(),
    Lease:  15 * time.Second,
})
_ = elector.Start(ctx)
defer elector.Resign(context.Background())

sched := chrono.NewScheduler(chrono.WithLeaderElector(elector))
```

`IsLeader` reads a local atomic — safe from hot paths.

**Clock skew:** the lock uses absolute `lease_until` timestamps, so
instance clocks SHOULD stay within `Lease/2` of one another (NTP/chrony
with the default sync interval is sufficient).
