package module_store

import (
	"context"

	tools "github.com/eru-os/eru/eru-ai/tools"
	function_module_store "github.com/eru-os/eru/eru-functions/module_store"
)

// WithEruServiceContext seeds the base urls every downstream eru call resolves from the
// context. Two sets of keys are needed and both have to be present:
//
//   - the plain string keys the eru-ai tools read when they call eru-ql, eru-files and
//     eru-functions over http
//   - the typed function_module_store keys eru-functions reads when an agent runs its own
//     FuncGroup inside this process - those fall back to eru-functions' own package defaults
//     (localhost), which are never configured here, so leaving them out sends query, tool and
//     agent steps to localhost instead of the real service
//
// Every entry point that ends up executing a tool or an agent has to call this, otherwise the
// two paths drift and one of them silently talks to the wrong host.
// WithProjectContext is WithEruServiceContext plus the project's own claims key, so calls
// made downstream forward the caller's token on the header that project's services read it
// from. Use this wherever the store is at hand; it is what every tool and agent entry point
// should seed the context with.
func WithProjectContext(ctx context.Context, projectId string, store ModuleStoreI) context.Context {
	ctx = WithEruServiceContext(ctx)
	if projectId == "" || store == nil {
		return ctx
	}
	prj, err := store.GetProjectConfig(ctx, projectId)
	if err != nil || prj == nil {
		// a project that cannot be read just keeps the default claims header
		return ctx
	}
	return tools.WithClaimsKey(ctx, prj.ProjectSettings.ClaimsKey)
}

func WithEruServiceContext(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, "eruauthbaseurl", Eruauthbaseurl)
	ctx = context.WithValue(ctx, "eruaiport", Eruaiport)
	ctx = context.WithValue(ctx, "eruqlbaseurl", Eruqlbaseurl)
	ctx = context.WithValue(ctx, "erufilesbaseurl", Erufilesbaseurl)
	ctx = context.WithValue(ctx, tools.EruFuncBaseUrlKey, Erufuncbaseurl)
	ctx = context.WithValue(ctx, function_module_store.ContextKeyEruaibaseurl, Eruaibaseurl)
	ctx = context.WithValue(ctx, function_module_store.ContextKeyEruqlbaseurl, Eruqlbaseurl)
	return ctx
}
