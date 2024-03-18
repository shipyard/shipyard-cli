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
	_ = findEnvByID(w, r)
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

func orgNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, "user org not found")
}

func envNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, "environment not found")
}
