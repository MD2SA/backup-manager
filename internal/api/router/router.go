package router

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	_ "github.com/MD2SA/backup-manager/docs"
	"github.com/MD2SA/backup-manager/internal/api/handlers"
	apimiddleware "github.com/MD2SA/backup-manager/internal/api/middleware"
	"github.com/MD2SA/backup-manager/internal/monitor"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgtype"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

func New(
	logger *slog.Logger,
	repo repository.Repository,
	monitorService *monitor.Service,
	adminKey string,
	rateLimitRequests int,
	rateLimitWindow time.Duration,
	corsOrigins []string,
	trustedProxies []netip.Prefix,
	onTrigger func(pgtype.UUID) error,
	onRestore func(context.Context, pgtype.UUID) error,
	onActivate func(db.Profile),
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(apimiddleware.RequestID)
	r.Use(apimiddleware.ClientIP(trustedProxies))
	r.Use(apimiddleware.Logger(logger))

	r.Use(corsHandler(corsOrigins))

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	profileHandler := &handlers.ProfileHandler{
		Repo:       repo,
		OnTrigger:  onTrigger,
		OnActivate: onActivate,
	}
	executionHandler := &handlers.ExecutionHandler{Repo: repo}
	providerHandler := &handlers.ProviderHandler{Repo: repo}
	monitorHandler := &handlers.MonitorHandler{Service: monitorService}
	restoreHandler := &handlers.RestoreHandler{Repo: repo, Executor: onRestore}
	retentionHandler := &handlers.RetentionHandler{Repo: repo}

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(httprate.LimitBy(rateLimitRequests, rateLimitWindow, func(r *http.Request) (string, error) {
			return httprate.CanonicalizeIP(apimiddleware.GetClientIP(r.Context())), nil
		}))
		r.Use(apimiddleware.ApiKeyAuth(adminKey))

		r.Get("/health", handlers.Health)
		r.Get("/health/summary", monitorHandler.HealthSummary)

		r.Get("/retention-policies", retentionHandler.List)
		r.Post("/retention-policies", retentionHandler.Create)
		r.Route("/retention-policies/{id}", func(r chi.Router) {
			r.Put("/", retentionHandler.Update)
			r.Delete("/", retentionHandler.Delete)
		})

		r.Route("/storage-providers", func(r chi.Router) {
			r.Get("/", providerHandler.ListStorage)
			r.Post("/", providerHandler.CreateStorage)
			r.Route("/{id}", func(r chi.Router) {
				r.Put("/", providerHandler.UpdateStorage)
				r.Delete("/", providerHandler.DeleteStorage)
			})
		})

		r.Route("/notification-providers", func(r chi.Router) {
			r.Get("/", providerHandler.ListNotification)
			r.Post("/", providerHandler.CreateNotification)
			r.Route("/{id}", func(r chi.Router) {
				r.Put("/", providerHandler.UpdateNotification)
				r.Delete("/", providerHandler.DeleteNotification)
			})
		})

		r.Route("/executions", func(r chi.Router) {
			r.Get("/", executionHandler.GetAll)
			r.Get("/{id}", executionHandler.Get)
			r.Post("/{id}/pin", executionHandler.Pin)
		})

		r.Route("/profiles", func(r chi.Router) {
			r.Get("/", profileHandler.List)
			r.Post("/", profileHandler.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Put("/", profileHandler.Update)
				r.Delete("/", profileHandler.Delete)
				r.Post("/run", apimiddleware.Idempotency(profileHandler.RunNow))
				r.Post("/activate", profileHandler.Activate)
				r.Get("/executions", executionHandler.ListByProfile)
			})
		})

		r.Post("/executions/{id}/restore", apimiddleware.Idempotency(restoreHandler.Trigger))
	})

	return r
}

// corsHandler produces a CORS middleware from a list of allowed origins.
func corsHandler(origins []string) func(http.Handler) http.Handler {
	if len(origins) == 0 {
		origins = []string{"http://localhost:3000"}
	}
	return cors.Handler(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key", "X-Idempotency-Key", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
