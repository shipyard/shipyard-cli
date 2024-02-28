package server

import "net/http"

func NewHandler() http.Handler {
	var h handler
	mux := http.NewServeMux()
	mux.HandleFunc("GET /environment", h.getAllEnvironments)
	mux.HandleFunc("GET /environment/{id}", h.getEnvironmentByID)
	mux.HandleFunc("POST /environment/{id}/rebuild", h.rebuildEnvironment)
	return mux
}

type handler struct {
}
