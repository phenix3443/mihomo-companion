package mihomo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/phenix3443/mihomo-companion/internal/runtime"
)

func TestUpdateRulesRemoteUpdatesAllRuntimeRuleProviders(t *testing.T) {
	var mu sync.Mutex
	var updated []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/providers/rules":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"providers": map[string]any{
					"alpha": map[string]any{},
					"beta":  map[string]any{},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/providers/rules/alpha":
			mu.Lock()
			updated = append(updated, "alpha")
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.Path == "/providers/rules/beta":
			mu.Lock()
			updated = append(updated, "beta")
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	env := &Env{}
	if err := env.UpdateRulesRemote(runtime.APIReloadInfo{BaseURL: server.URL, Secret: "demo-secret"}); err != nil {
		t.Fatal(err)
	}

	sort.Strings(updated)
	if got := len(updated); got != 2 {
		t.Fatalf("updated provider count = %d, want 2", got)
	}
	if updated[0] != "alpha" || updated[1] != "beta" {
		t.Fatalf("updated providers = %v", updated)
	}
}

func TestUpdateRulesRemotePassesAuthorizationHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/providers/rules":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"providers": map[string]any{
					"demo": map[string]any{},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/providers/rules/demo":
			if got := r.Header.Get("Authorization"); got != "Bearer demo-secret" {
				t.Fatalf("authorization header = %q", got)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	env := &Env{}
	if err := env.UpdateRulesRemote(runtime.APIReloadInfo{BaseURL: server.URL, Secret: "demo-secret"}); err != nil {
		t.Fatal(err)
	}
}
