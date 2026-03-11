package proxy

import "context"

type requestIdentityContextKey struct{}

const sharedLocalIdentity = "local:shared"

func withRequestIdentity(ctx context.Context, identity string) context.Context {
	return context.WithValue(ctx, requestIdentityContextKey{}, identity)
}

func requestIdentityFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	identity, ok := ctx.Value(requestIdentityContextKey{}).(string)
	if !ok || identity == "" {
		return "", false
	}
	return identity, true
}
