package domain

import "context"

type scopeKey struct{}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

func FromContext(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(scopeKey{}).(Scope)
	return scope, ok
}

// ID is the persistence scope. Legacy/internal callers without a request scope
// address the official domain only; HTTP resource routes always bind a scope.
func ID(ctx context.Context) string {
	if scope, ok := FromContext(ctx); ok {
		return scope.Domain.ID
	}
	return OfficialID
}
