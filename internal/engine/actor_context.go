package engine

import "context"

type actorContextKey struct{}

func WithAuthenticatedActor(ctx context.Context, actor string) context.Context {
	if actor == "" {
		return ctx
	}
	return context.WithValue(ctx, actorContextKey{}, actor)
}

func authenticatedActor(ctx context.Context) (string, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(string)
	return actor, ok && actor != ""
}
