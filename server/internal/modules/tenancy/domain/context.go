package domain

import (
	"context"
	"time"
)

type scopeKey struct{}
type domainKey struct{}
type readTimeKey struct{}

// WithReadTime carries one observation time through a read and its projections.
// Mutation authorization and worker leases deliberately use current time.
func WithReadTime(ctx context.Context, at time.Time) context.Context {
	return context.WithValue(ctx, readTimeKey{}, at)
}
func ReadTime(ctx context.Context) time.Time {
	if at, ok := ctx.Value(readTimeKey{}).(time.Time); ok {
		return at
	}
	return time.Now()
}

// WithDomain binds a tenant identity for a trusted internal caller. It grants
// no permissions: resource operations still reload their explicit actor.
func WithDomain(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, domainKey{}, id)
}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

func FromContext(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(scopeKey{}).(Scope)
	return scope, ok
}

// ID returns only an explicitly bound persistence scope. An absent scope must
// never select a tenant implicitly, including for internal callers.
func ID(ctx context.Context) string {
	if scope, ok := FromContext(ctx); ok {
		return scope.Domain.ID
	}
	id, _ := ctx.Value(domainKey{}).(string)
	return id
}

func RequireScope(ctx context.Context) (Scope, error) {
	scope, ok := FromContext(ctx)
	if !ok {
		scope.Domain.ID = ID(ctx)
	}
	if scope.Domain.ID == "" {
		return Scope{}, ErrMissingScope
	}
	return scope, nil
}

// ActorID identifies the authenticated caller bound by domain middleware.
// No scope means no authenticated actor, never an implicit administrator.
func ActorID(ctx context.Context) string {
	scope, _ := FromContext(ctx)
	return scope.UserID
}
