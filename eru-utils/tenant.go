package eru_utils

import (
	"context"
	"fmt"
	"strings"
)

const TenantSeparator = "___"

// ClaimsHeaderKey is the header the gateway writes verified claims to once it has
// authenticated a request, and the name every eru service reads them back from. It is a wire
// contract between services, so it is defined once here rather than repeated as a literal.
// It is also the key the claims are carried on inside a request context.
const ClaimsHeaderKey = "claims"

type defaultTenantKeyType struct{}

var defaultTenantKey = defaultTenantKeyType{}

func ParseTenantRoute(routeTenant string) (tenantId string, defaultTenantId string) {
	routeTenant = strings.ToLower(strings.TrimSpace(routeTenant))
	if routeTenant == "" {
		return "", ""
	}
	defaultTenantId, tenantId, found := strings.Cut(routeTenant, TenantSeparator)
	if !found {
		return routeTenant, ""
	}
	if tenantId == "" {
		return defaultTenantId, ""
	}
	if defaultTenantId == tenantId {
		return tenantId, ""
	}
	return tenantId, defaultTenantId
}

func WithDefaultTenant(ctx context.Context, defaultTenantId string) context.Context {
	if defaultTenantId == "" {
		return ctx
	}
	return context.WithValue(ctx, defaultTenantKey, defaultTenantId)
}

func DefaultTenant(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	defaultTenantId, _ := ctx.Value(defaultTenantKey).(string)
	return defaultTenantId
}

func TenantLookupOrder(ctx context.Context, tenantId string, extraFallbacks ...string) []string {
	candidates := make([]string, 0, 2+len(extraFallbacks))
	seen := make(map[string]bool)
	for _, candidate := range append([]string{tenantId, DefaultTenant(ctx)}, extraFallbacks...) {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		candidates = append(candidates, candidate)
	}
	return candidates
}

// JoinTenantRoute builds the route form of a tenant, the inverse of ParseTenantRoute.
// It returns "defaultTenantId___tenantId" when a distinct default tenant applies, so the
// receiving service can rebuild the same fallback order, and the plain tenant otherwise.
func JoinTenantRoute(defaultTenantId string, tenantId string) string {
	defaultTenantId = strings.ToLower(strings.TrimSpace(defaultTenantId))
	tenantId = strings.ToLower(strings.TrimSpace(tenantId))
	if tenantId == "" {
		return defaultTenantId
	}
	if defaultTenantId == "" || defaultTenantId == tenantId {
		return tenantId
	}
	return fmt.Sprint(defaultTenantId, TenantSeparator, tenantId)
}

// FetchTenantRoute builds the route tenant to forward on a read call so the callee keeps
// the route tenant -> default tenant -> project level fallback this request came in with.
func FetchTenantRoute(ctx context.Context, tenantId string) string {
	return JoinTenantRoute(DefaultTenant(ctx), tenantId)
}
