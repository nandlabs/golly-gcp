# gcpiam — Google Cloud IAM authorizer for `golly/authz`

`gcpiam` adapts Cloud IAM to the `oss.nandlabs.io/golly/authz.Policy`
interface, so IAM roles bound to GCP resources can be used as the source
of truth for `Can(principal, action, resource)` checks in a Golly app.

## Why this package looks the way it does

`authz.Policy.Can(p Principal, action string, resource any) bool` is
deliberately context-free and returns a plain bool. That doesn't fit a
Cloud IAM `TestIamPermissions` call, which needs a `context.Context`
and can fail with a network error. This package offers two shapes:

- **Cached evaluator (recommended)** — a background goroutine refreshes
  a permission map on a configurable interval. `Can` reads from the map.
  Freshness is bounded by the refresh interval; latency is amortized.
- **Live check via `IAMResource`** — pass an `IAMResource{Ctx, Name}` as
  the `resource any` argument. `Can` type-asserts, then makes a live
  `TestIamPermissions` round-trip. Simpler but couples callers.

## Usage

```go
import (
    "context"
    "time"

    resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
    "oss.nandlabs.io/golly/authz"
    "oss.nandlabs.io/golly-gcp/authz/gcpiam"
)

func main() {
    ctx := context.Background()

    // 1. A GCP client that speaks TestIamPermissions.
    client, err := resourcemanager.NewProjectsClient(ctx)
    if err != nil { /* handle */ }
    defer client.Close()

    // 2. Cached IAM evaluator, refreshed every minute.
    eval := gcpiam.NewCachedEvaluator(client, "projects/my-project", time.Minute,
        gcpiam.WithExpectedMember("serviceAccount:svc@my-project.iam.gserviceaccount.com"),
        gcpiam.WithInitialPermissions(
            "resourcemanager.projects.get",
            "resourcemanager.projects.getIamPolicy",
        ),
    )
    defer eval.Close()

    // 3. Compose with any other Policy — here RBAC as the local fallback.
    rbac := authz.NewRBAC()
    rbac.AddRole("admin", "resourcemanager.projects.get")

    policy := authz.Any(eval, rbac) // IAM says yes OR RBAC says yes

    // 4. Evaluate.
    p := &myPrincipal{id: "serviceAccount:svc@my-project.iam.gserviceaccount.com"}
    if policy.Can(p, "resourcemanager.projects.get", nil) {
        // proceed
    }
}
```

`myPrincipal` needs `Roles()`, `Capabilities()`, and either an
`ID() string` accessor (picked up by the default extractor) or you
inject one with `gcpiam.WithPrincipalMember(func(p authz.Principal) string { ... })`.

## Storage bucket (or any other service)

The cached evaluator is wired to a `*resourcemanager.ProjectsClient` for
the common case. To use it against any other service, plug in a client
that satisfies `gcpiam.IAMCaller` via `NewCachedEvaluatorFromCaller`.
Below, the Storage bucket IAM handle is exposed through a small
shim because Storage's `*iam.Handle` uses a slightly different signature:

```go
import (
    "cloud.google.com/go/iam/apiv1/iampb"
    "cloud.google.com/go/storage"
    gax "github.com/googleapis/gax-go/v2"
    "oss.nandlabs.io/golly-gcp/authz/gcpiam"
)

type bucketCaller struct{ b *storage.BucketHandle }

func (c bucketCaller) TestIamPermissions(ctx context.Context, req *iampb.TestIamPermissionsRequest, _ ...gax.CallOption) (*iampb.TestIamPermissionsResponse, error) {
    got, err := c.b.IAM().TestPermissions(ctx, req.GetPermissions())
    if err != nil { return nil, err }
    return &iampb.TestIamPermissionsResponse{Permissions: got}, nil
}

// Usage:
gcs, _ := storage.NewClient(ctx)
eval := gcpiam.NewCachedEvaluatorFromCaller(
    bucketCaller{b: gcs.Bucket("photos")},
    "projects/_/buckets/photos",
    time.Minute,
    gcpiam.WithInitialPermissions("storage.objects.get", "storage.objects.list"),
)
```

## Live per-call checks (bypass the cache)

Pass an `IAMResource` as the `resource any` argument to Can. The
evaluator will call `TestIamPermissions` synchronously using the
embedded context and resource name.

```go
res := gcpiam.IAMResource{
    Ctx:  ctx,
    Name: "projects/_/buckets/photos", // if empty, evaluator's default is used
}
if eval.Can(principal, "storage.objects.get", res) {
    // ...
}
```

## Options

| Option                        | Effect                                                                       |
|-------------------------------|------------------------------------------------------------------------------|
| `WithPrincipalMember(fn)`     | Override how a Principal is mapped to a GCP IAM member string.               |
| `WithExpectedMember(m)`       | Deny any Principal whose extracted member doesn't equal `m`.                 |
| `WithInitialPermissions(...)` | Seed the tracked-action set so the first Refresh already checks them.        |

## IAM permission required

The credentials the client is authenticated with need
`iam.serviceAccounts.testIamPermissions` (or the service-specific
equivalent, e.g. `storage.buckets.testIamPermissions`). Read the
[IAM reference](https://cloud.google.com/iam/docs/permissions-reference)
for the resource type you're checking against.

## Notes

- `TestIamPermissions` reports permissions granted to the *authenticated
  caller identity*, not to an arbitrary principal. `WithExpectedMember`
  makes that constraint explicit — it prevents accidentally granting
  access to a Principal whose identity differs from the client's.
- IAM Conditions (`request.time < ...`) are evaluated server-side by
  IAM; this package makes no attempt to re-evaluate them locally.
- The cache is invalidated only by `Refresh` (manual or the background
  loop). Rotate the refresh interval to trade freshness for QPS.
