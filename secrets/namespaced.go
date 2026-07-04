package secrets

import (
	"context"

	secrets "oss.nandlabs.io/golly/secrets"
)

// NewNamespacedGCPSecretStore is a convenience constructor that assembles a
// namespaced GCP Secret Manager-backed store in one call. It builds the base
// [GCPSecretStore] via [NewGCPSecretStore] and wraps it with
// [secrets.Namespace], applying prefix and any [secrets.NamespaceOption] (for
// example [secrets.WithAuthorizer]) that callers pass through.
//
// The returned value is a [secrets.Store] so callers program against the
// interface — the concrete GCP store is intentionally hidden behind the
// wrapper. Use [NewGCPSecretStore] directly when you need access to the
// GCP-specific methods (List, Delete, GetClient, ClearCache, Close).
//
// Typical usage — per-tenant namespace with an Authorizer that reads the
// principal from request context:
//
//	auth := func(ctx context.Context, op secrets.Op, key string) error {
//	    user := userFromCtx(ctx)
//	    if !user.CanAccess(op, key) {
//	        return fmt.Errorf("%w: user %s", secrets.ErrForbidden, user.ID)
//	    }
//	    return nil
//	}
//	store, err := gcpsecrets.NewNamespacedGCPSecretStore(
//	    ctx,
//	    &gcpsecrets.GCPSecretStoreConfig{ProjectID: "my-project"},
//	    "tenants/"+tenantID,
//	    secrets.WithAuthorizer(auth),
//	)
//
// Every Get/Write against the returned store is fully-qualified with prefix
// before it reaches GCP — callers pass bare keys.
func NewNamespacedGCPSecretStore(
	ctx context.Context,
	cfg *GCPSecretStoreConfig,
	prefix string,
	opts ...secrets.NamespaceOption,
) (secrets.Store, error) {
	base, err := NewGCPSecretStore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return secrets.Namespace(base, prefix, opts...), nil
}
