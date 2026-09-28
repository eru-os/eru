package tools

import (
	"context"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	utils "github.com/eru-os/eru/eru-utils"
)

// TenantScope picks which tenant a write action targets. Reads never take a scope - they
// always forward the route form of the tenant and rely on the callee's fallback order.
type TenantScope string

const (
	ScopeTenant        TenantScope = "tenant"
	ScopeDefaultTenant TenantScope = "default_tenant"
	ScopeProject       TenantScope = "project"
)

// ActionSuffix is appended to a write action name to expose one action per scope.
func (scope TenantScope) ActionSuffix() string {
	switch scope {
	case ScopeDefaultTenant:
		return "_default"
	case ScopeProject:
		return "_project"
	default:
		return ""
	}
}

// DescriptionSuffix explains to the caller where the write lands.
func (scope TenantScope) DescriptionSuffix() string {
	switch scope {
	case ScopeDefaultTenant:
		return " Writes against the default tenant (the prefix of the tenant the caller connected with), so every tenant sharing that default sees the change."
	case ScopeProject:
		return " Writes against the project itself, outside any tenant, so every tenant of the project falls back to it."
	default:
		return " Writes against the tenant the caller connected with."
	}
}

// WriteTenantRoute resolves the tenant path segment a write action should address.
// An empty segment means project level, i.e. the tenant is left out of the url.
func WriteTenantRoute(ctx context.Context, tenantId string, scope TenantScope) (routeTenant string, err error) {
	switch scope {
	case ScopeProject:
		return "", nil
	case ScopeDefaultTenant:
		defaultTenantId := utils.DefaultTenant(ctx)
		if defaultTenantId == "" {
			return "", logs.Err(ctx, fmt.Errorf("no default tenant in this request - send the tenant as <default tenant>%s<tenant> to write against the default tenant", utils.TenantSeparator), "")
		}
		return defaultTenantId, nil
	case ScopeTenant, "":
		if tenantId == "" {
			return "", logs.Err(ctx, fmt.Errorf("no tenant in this request - use the project scoped action to write outside a tenant"), "")
		}
		return tenantId, nil
	default:
		return "", logs.Err(ctx, fmt.Errorf("unknown tenant scope %q", scope), "")
	}
}

// TenantPath joins url segments below a project, leaving the tenant out when it is empty so
// the project level route is addressed. Empty trailing segments are dropped.
func TenantPath(baseUrl string, projectId string, routeTenant string, parts ...string) string {
	segments := make([]string, 0, len(parts)+2)
	segments = append(segments, projectId)
	if routeTenant != "" {
		segments = append(segments, routeTenant)
	}
	for _, part := range parts {
		if part == "" {
			continue
		}
		segments = append(segments, strings.Trim(part, "/"))
	}
	return fmt.Sprint(strings.TrimSuffix(baseUrl, "/"), "/", strings.Join(segments, "/"))
}

// FetchTenantPath is TenantPath for a read, forwarding the route form of the tenant so the
// callee keeps the tenant -> default tenant -> project fallback.
func FetchTenantPath(ctx context.Context, baseUrl string, projectId string, tenantId string, parts ...string) string {
	return TenantPath(baseUrl, projectId, utils.FetchTenantRoute(ctx, tenantId), parts...)
}

// WriteScopes is the set of tenant scopes a write action is exposed under.
var WriteScopes = []TenantScope{ScopeTenant, ScopeDefaultTenant, ScopeProject}

// ScopedAction is the base action and tenant scope an exposed action name maps back to.
type ScopedAction struct {
	BaseName string
	Scope    TenantScope
}

// ExpandScopedActions returns the actions a tool exposes - read actions unchanged, and one
// action per tenant scope for every action named in writeActions - along with a lookup from
// exposed action name back to the base action and the scope it carries.
func ExpandScopedActions(actions []ToolAction, writeActions map[string]bool) (expanded []ToolAction, scopes map[string]ScopedAction) {
	return ExpandScopedActionsFor(actions, writeActions, WriteScopes)
}

// ExpandScopedActionsFor is ExpandScopedActions for a tool that offers its writes under only
// some of the scopes. A scope left out is not exposed and has no entry in scopes, so a call
// naming it is rejected as an unknown action rather than silently writing somewhere else.
func ExpandScopedActionsFor(actions []ToolAction, writeActions map[string]bool, writeScopes []TenantScope) (expanded []ToolAction, scopes map[string]ScopedAction) {
	expanded = make([]ToolAction, 0, len(actions)+len(writeScopes)*len(writeActions))
	scopes = make(map[string]ScopedAction, len(actions)+len(writeScopes)*len(writeActions))
	for _, action := range actions {
		if !writeActions[action.ActionName] {
			expanded = append(expanded, action)
			scopes[action.ActionName] = ScopedAction{BaseName: action.ActionName, Scope: ScopeTenant}
			continue
		}
		for _, scope := range writeScopes {
			scoped := action
			scoped.ActionName = action.ActionName + scope.ActionSuffix()
			scoped.Description = action.Description + scope.DescriptionSuffix()
			expanded = append(expanded, scoped)
			scopes[scoped.ActionName] = ScopedAction{BaseName: action.ActionName, Scope: scope}
		}
	}
	return expanded, scopes
}
