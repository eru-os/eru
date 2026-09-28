package tools

import (
	"context"

	eru_utils "github.com/eru-os/eru/eru-utils"
)

// ClaimsHeaderKey is the header the gateway writes the verified claims to on the way in, and
// the key eru-server's middleware puts them on the context under - the same wire contract
// every eru service shares, so it is taken from there rather than repeated.
const ClaimsHeaderKey = eru_utils.ClaimsHeaderKey

// ClaimsKeyCtxKey carries the header name a project expects its token on. eru-ai reads the
// claims under ClaimsHeaderKey but has to forward them under whatever the callee's project is
// configured with, which is what eru-functions and eru-ql look for.
const ClaimsKeyCtxKey contextKey = "ClaimsKey"

// ClaimsFromContext reads the caller's claims off the context. A context that never went
// through the http middleware - a background job, a relayed sub agent call - carries none, so
// this returns an empty string rather than panicking on a type assertion.
func ClaimsFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	claims, _ := ctx.Value(ClaimsHeaderKey).(string)
	return claims
}

// WithClaimsKey records the header name downstream eru services expect the token on.
func WithClaimsKey(ctx context.Context, claimsKey string) context.Context {
	if claimsKey == "" {
		return ctx
	}
	return context.WithValue(ctx, ClaimsKeyCtxKey, claimsKey)
}

// ClaimsKeyFromContext is the header name to forward the claims under, falling back to the
// name they arrived on when a project configures none.
func ClaimsKeyFromContext(ctx context.Context) string {
	if ctx == nil {
		return ClaimsHeaderKey
	}
	if claimsKey, ok := ctx.Value(ClaimsKeyCtxKey).(string); ok && claimsKey != "" {
		return claimsKey
	}
	return ClaimsHeaderKey
}

// ClaimsHeader returns the header name and claims to forward, and whether there is anything
// to forward at all.
func ClaimsHeader(ctx context.Context) (claimsKey string, claims string, ok bool) {
	claims = ClaimsFromContext(ctx)
	return ClaimsKeyFromContext(ctx), claims, claims != ""
}

// CopyClaims carries the caller's claims, and the header name they have to be forwarded
// under, from a request context onto a background one. A goroutine started for a hook gets a
// fresh context, so without this the hook runs unauthenticated - or forwards the token on the
// wrong header.
func CopyClaims(from context.Context, to context.Context) context.Context {
	if from == nil || to == nil {
		return to
	}
	if claims := ClaimsFromContext(from); claims != "" {
		to = context.WithValue(to, ClaimsHeaderKey, claims)
	}
	if claimsKey, ok := from.Value(ClaimsKeyCtxKey).(string); ok && claimsKey != "" {
		to = context.WithValue(to, ClaimsKeyCtxKey, claimsKey)
	}
	return to
}
