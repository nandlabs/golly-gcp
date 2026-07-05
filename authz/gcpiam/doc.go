// Package gcpiam adapts Google Cloud IAM permission checks to the
// oss.nandlabs.io/golly/authz.Policy interface.
//
// The upstream Policy interface has the signature
//
//	Can(p Principal, action string, resource any) bool
//
// which is context-free and returns a plain bool. That doesn't fit the
// shape of a Cloud IAM call (which needs a context.Context and can fail
// with a network error). This package offers two workable shapes:
//
//   - A cached Evaluator that refreshes a permission map on a background
//     goroutine — Can consults the cached map synchronously. Freshness is
//     bounded by the configured refresh interval. This is the recommended
//     production shape.
//
//   - An IAMResource envelope callers pass as the `resource any` argument
//     when they want a live TestIamPermissions call instead of the cache.
//     The context and the fully-qualified resource name ride along on the
//     envelope so Can can type-assert them out.
//
// Under the hood the evaluator calls
// iampb.TestIamPermissions(ctx, {Resource, Permissions}) via any client
// that exposes that method — the built-in constructor accepts a
// *resourcemanager.ProjectsClient, and NewCachedEvaluatorFromCaller
// takes the small IAMCaller interface directly so callers can bring a
// Storage / Pub-Sub / other client that exposes the same method.
package gcpiam
