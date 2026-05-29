package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/user/obsidian-sync-f2p/internal/protocol"
	"github.com/user/obsidian-sync-f2p/internal/store"
)

// Server is the HTTP server for the obsidian-sync API.
type Server struct {
	router   *chi.Mux
	store    *store.Store
	handlers *Handlers
	addr     string
	logger   *log.Logger
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	Addr   string
	Store  *store.Store
	Logger *log.Logger
}

// NewServer creates a new Server with a Chi router, middleware chain,
// and all sync endpoint handlers mounted.
func NewServer(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	s := &Server{
		router: chi.NewRouter(),
		store:  cfg.Store,
		addr:   cfg.Addr,
		logger: cfg.Logger,
	}

	s.handlers = NewHandlers(s.store)
	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// setupMiddleware configures the middleware chain.
func (s *Server) setupMiddleware() {
	s.router.Use(chimw.RequestID)
	s.router.Use(chimw.RealIP)
	s.router.Use(RequestLogger(s.logger))
	s.router.Use(chimw.Recoverer)
}

// setupRoutes mounts all API routes with their respective middleware.
func (s *Server) setupRoutes() {
	s.router.Route(protocol.APIV1Base, func(r chi.Router) {
		// Health check (no auth required).
		r.Get("/health", s.handleHealth)

		// Sync endpoints (auth required).
		r.Route("/sync", func(r chi.Router) {
			r.Use(AuthValidation(s.store))
			r.Use(SizeLimit(protocol.MaxFileSize))

			r.Post("/begin", s.handlers.HandleBegin)
			r.Post("/manifest", s.handlers.HandleManifest)
			r.Post("/file", s.handlers.HandleFileUpload)
			r.Get("/file", s.handlers.HandleFileDownload)
			r.Post("/complete", s.handlers.HandleComplete)
		})
	})
}

// handleHealth returns a simple health check response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	type healthResponse struct {
		Status string `json:"status"`
	}
	writeJSON(w, healthResponse{Status: "ok"})
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	s.logger.Printf("osync server listening on %s", s.addr)
	return http.ListenAndServe(s.addr, s.router)
}

// Handler returns the HTTP handler for use with httptest.Server.
func (s *Server) Handler() http.Handler {
	return s.router
}

// Addr returns the server address.
func (s *Server) Addr() string {
	return s.addr
}

// FormatAddr returns a formatted address string for display.
func FormatAddr(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}
