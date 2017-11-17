package gateway

import (
	"github.com/gorilla/mux"
)

// Router encapsulates an HTTP router for forwarding HTTP
// requests to Frugal endpoints
type Router struct {
	*mux.Router
}

// NewRouter returns a new router for serving HTTP requests
func NewRouter() *Router {
	return &Router{mux.NewRouter()}
}