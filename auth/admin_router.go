package auth

import (
	"github.com/julienschmidt/httprouter"
)

// AdminRouter wraps httprouter with automatic RequireAdmin middleware
type AdminRouter struct {
	router *httprouter.Router
}

// NewAdminRouter creates a new AdminRouter
func NewAdminRouter(router *httprouter.Router) *AdminRouter {
	return &AdminRouter{router: router}
}

// GET registers a GET route with RequireAdmin middleware
func (ar *AdminRouter) GET(path string, handler httprouter.Handle) {
	ar.router.GET(path, RequireAdmin(handler))
}

// POST registers a POST route with RequireAdmin middleware
func (ar *AdminRouter) POST(path string, handler httprouter.Handle) {
	ar.router.POST(path, RequireAdmin(handler))
}

// PUT registers a PUT route with RequireAdmin middleware
func (ar *AdminRouter) PUT(path string, handler httprouter.Handle) {
	ar.router.PUT(path, RequireAdmin(handler))
}

// DELETE registers a DELETE route with RequireAdmin middleware
func (ar *AdminRouter) DELETE(path string, handler httprouter.Handle) {
	ar.router.DELETE(path, RequireAdmin(handler))
}

// PATCH registers a PATCH route with RequireAdmin middleware
func (ar *AdminRouter) PATCH(path string, handler httprouter.Handle) {
	ar.router.PATCH(path, RequireAdmin(handler))
}
