package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/shipyard/shipyard-cli/pkg/types"
)

func (handler) getAllEnvironments(w http.ResponseWriter, r *http.Request) {
	org := r.URL.Query().Get("org")
	envs, ok := store[org]
	if !ok {
		orgNotFound(w)
		return
	}
	resp := types.RespManyEnvs{Data: envs}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
	}
}

func (handler) getEnvironmentByID(w http.ResponseWriter, r *http.Request) {
	env := findEnvByID(w, r)
	if env != nil {
		resp := types.Response{Data: *env}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
		}
	}
}

func (handler) rebuildEnvironment(w http.ResponseWriter, r *http.Request) {
	env := findEnvByID(w, r)
	if env != nil {
		_, _ = fmt.Fprint(w, "Environment queued for a rebuild.")
	}
}

func (handler) deployDetached(w http.ResponseWriter, r *http.Request) {
	org := r.URL.Query().Get("org")
	if _, ok := store[org]; !ok {
		orgNotFound(w)
		return
	}
	if r.PathValue("id") == "missing-build" {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, "application build not found")
		return
	}

	var req struct {
		DisplayName string `json:"display_name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	name := req.DisplayName
	if name == "" {
		name = "detached"
	}

	w.WriteHeader(http.StatusCreated)
	resp := map[string]any{
		"data": map[string]any{
			"message":                fmt.Sprintf("Detached environment '%s' deployed successfully", name),
			"application_uuid":       "new-app-uuid",
			"application_build_uuid": "new-app-build-uuid",
			"display_name":           name,
		},
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
	}
}

func findEnvByID(w http.ResponseWriter, r *http.Request) *types.Environment {
	org := r.URL.Query().Get("org")
	envs, ok := store[org]
	if !ok {
		orgNotFound(w)
		return nil
	}
	id := r.PathValue("id")
	for i := range envs {
		if envs[i].ID == id {
			return &envs[i]
		}
	}
	envNotFound(w)
	return nil
}

// deleteDetached mirrors DELETE /api/v1/environment/<uuid>/detached: "detached-1" is a detached
// environment, "default-1" a base environment the API refuses, and anything else is unknown.
func (handler) deleteDetached(w http.ResponseWriter, r *http.Request) {
	org := r.URL.Query().Get("org")
	if _, ok := store[org]; !ok {
		orgNotFound(w)
		return
	}
	apiError := func(status int, title string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]any{{"title": title, "status": status}},
		})
	}
	switch r.PathValue("id") {
	case "detached-1":
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"application_uuid": "detached-1", "display_name": "ci-preview"},
		})
	case "default-1":
		apiError(http.StatusBadRequest, "Environment is not a detached environment")
	default:
		apiError(http.StatusNotFound, "Environment not found")
	}
}

func orgNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	_, _ = fmt.Fprintf(w, "user org not found")
}

func envNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = fmt.Fprintf(w, "environment not found")
}
