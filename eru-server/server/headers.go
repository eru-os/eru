package server

import (
	"context"
	"net/http"

	eru_utils "github.com/eru-os/eru/eru-utils"
)

const (
	// Canonical request headers. http.Header.Get canonicalises the name it is given, so any
	// casing a caller sends - x-tenant-id, X-Tenant-Id, X-TENANT-ID - matches these.
	TenantHeaderKey   string = "X-Tenant-Id"
	ProjectHeaderKey  string = "X-Project-Id"
	ClientHeaderKey   string = "X-Client-Id"
	AuthNameHeaderKey string = "X-Auth-Name"

	// Names used before the move to the X- headers. Still read so callers that have not moved
	// over keep working; drop these once nothing sends them.
	legacyTenantHeaderKey  string = "tenant_id"
	legacyProjectHeaderKey string = "project_id"
)

// headerValue reads the canonical header, falling back to the legacy name.
func headerValue(r *http.Request, headerKey string, legacyHeaderKey string) string {
	if value := r.Header.Get(headerKey); value != "" {
		return value
	}
	return r.Header.Get(legacyHeaderKey)
}

// RequestTenantRoute returns the tenant header exactly as the caller sent it, which may be
// the route form "<default tenant>___<tenant>". Use it when the value has to stay intact,
// such as when forwarding the request to another service.
func RequestTenantRoute(r *http.Request) string {
	return headerValue(r, TenantHeaderKey, legacyTenantHeaderKey)
}

// RequestTenant splits the tenant header into the tenant and the default tenant it carries.
func RequestTenant(r *http.Request) (tenantId string, defaultTenantId string) {
	return eru_utils.ParseTenantRoute(RequestTenantRoute(r))
}

// RequestTenantContext splits the tenant header and puts the default tenant on the context,
// so tenant lookups made further down fall back to it.
func RequestTenantContext(r *http.Request) (ctx context.Context, tenantId string) {
	tenantId, defaultTenantId := RequestTenant(r)
	return eru_utils.WithDefaultTenant(r.Context(), defaultTenantId), tenantId
}

// RequestProject returns the project the caller addressed.
func RequestProject(r *http.Request) string {
	return headerValue(r, ProjectHeaderKey, legacyProjectHeaderKey)
}

// RequestAuthName returns the auth the caller was routed to. The gateway listener rule names it,
// so a service can look up the matching auth config without it being pinned in its own store.
func RequestAuthName(r *http.Request) string {
	return r.Header.Get(AuthNameHeaderKey)
}

// RequestClient returns the client the caller identified itself as. The gateway authorizer
// reads the client from a header named in its own config, so this is for services that need
// the client id directly rather than for authorisation.
func RequestClient(r *http.Request) string {
	return r.Header.Get(ClientHeaderKey)
}
