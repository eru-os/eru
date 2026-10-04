package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/eru-os/eru/eru-ai/module_store"
	"github.com/eru-os/eru/eru-ai/tools"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	server_handlers "github.com/eru-os/eru/eru-server/server/handlers"
	store "github.com/eru-os/eru/eru-store/store"
	"github.com/gorilla/mux"
)

var (
	secretRefExact = regexp.MustCompile(`^\$SECRET_([A-Za-z0-9_]+)$`)
	secretRefAny   = regexp.MustCompile(`\$SECRET_([A-Za-z0-9_]+)`)
)

func catalogSchema(toolType string) (eru_models.JSONSchema, bool) {
	for _, entry := range tools.GetToolCatalog() {
		if entry.ToolType == toolType {
			return entry.ToolSchema, true
		}
	}
	return eru_models.JSONSchema{}, false
}

// literalSecretFields lists the writeOnly fields of a tool config that hold
// something other than a single $SECRET_<name> reference. A credential is
// stored as a reference to a secret, so a literal value here is a credential
// about to be saved in plain text.
func literalSecretFields(schema eru_models.JSONSchema, value map[string]interface{}, path string) []string {
	var bad []string
	for name, prop := range schema.Properties {
		v, ok := value[name]
		if !ok || v == nil {
			continue
		}
		p := name
		if path != "" {
			p = path + "." + name
		}
		if prop.WriteOnly {
			if s, isStr := v.(string); !isStr || (s != "" && !secretRefExact.MatchString(s)) {
				bad = append(bad, p)
			}
			continue
		}
		if child, isMap := v.(map[string]interface{}); isMap && len(prop.Properties) > 0 {
			bad = append(bad, literalSecretFields(prop, child, p)...)
		}
	}
	sort.Strings(bad)
	return bad
}

func secretRefs(value interface{}) []string {
	seen := map[string]bool{}
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch t := v.(type) {
		case string:
			for _, m := range secretRefAny.FindAllStringSubmatch(t, -1) {
				seen[m[1]] = true
			}
		case map[string]interface{}:
			for _, c := range t {
				walk(c)
			}
		case []interface{}:
			for _, c := range t {
				walk(c)
			}
		}
	}
	walk(value)
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func secretKnown(ctx context.Context, s module_store.ModuleStoreI, projectId, tenantId, key string) bool {
	if tv, err := s.FetchTenantVars(ctx, projectId); err == nil {
		for _, tid := range []string{tenantId, projectId} {
			if _, ok := tv[tid].Secrets[key]; ok {
				return true
			}
		}
	}
	if pv, err := s.FetchVars(ctx, projectId); err == nil {
		if _, ok := pv.Secrets[key]; ok {
			return true
		}
	}
	return false
}

// registerSecretRefs makes every $SECRET_<name> a tool config refers to
// resolvable for the tenant. $SECRET_ substitution only covers keys the store
// has registered; a secret saved from an app lands in the tenant's secret
// manager entry without being registered here, so it would stay as literal
// text. Unknown keys are read from that entry and registered; a key that is
// not there either is an error, so the tool is not saved half-configured.
func registerSecretRefs(ctx context.Context, s module_store.ModuleStoreI, projectId, tenantId string, keys []string) error {
	var missing []string
	for _, key := range keys {
		if secretKnown(ctx, s, projectId, tenantId, key) {
			continue
		}
		v, err := s.GetSmValue(ctx, projectId, tenantId, key, false)
		value, isStr := v.(string)
		if err != nil || !isStr || value == "" {
			missing = append(missing, key)
			continue
		}
		if err = s.SaveTenantSecret(ctx, projectId, tenantId, store.Secrets{Key: key, Value: value}, s); err != nil {
			return err
		}
		logs.WithContext(ctx).Info(fmt.Sprint("registered tenant secret ", key, " for ", tenantId))
	}
	if len(missing) > 0 {
		return fmt.Errorf("secret not found: %s - create it under Secrets first", strings.Join(missing, ", "))
	}
	return nil
}

// SecretNamesHandler lists the secret names a tenant's tools can refer to as
// $SECRET_<name>: the keys in the tenant's secret manager entry (where
// apps save secrets) and the keys registered for the tenant, its project
// fallback and the project. Names only - a value never leaves this handler.
func SecretNamesHandler(sh *module_store.StoreHolder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		vars := mux.Vars(r)
		projectId, tenantId := vars["project"], vars["tenant"]
		names := map[string]bool{}
		if smObj, err := sh.Store.FetchSm(ctx, projectId); err == nil && smObj != nil {
			if values, vErr := smObj.GetSmValues(ctx, tenantId); vErr == nil {
				for k := range values {
					names[k] = true
				}
			}
		}
		if tv, err := sh.Store.FetchTenantVars(ctx, projectId); err == nil {
			for _, tid := range []string{tenantId, projectId} {
				for k := range tv[tid].Secrets {
					names[k] = true
				}
			}
		}
		if pv, err := sh.Store.FetchVars(ctx, projectId); err == nil {
			for k := range pv.Secrets {
				names[k] = true
			}
		}
		out := make([]string, 0, len(names))
		for k := range names {
			out = append(out, k)
		}
		sort.Strings(out)
		server_handlers.FormatResponse(w, 200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"secret_names": out})
	}
}
