package router

import (
	"context"
	"log/slog"

	_ "github.com/MD2SA/backup-manager/docs"
	"github.com/MD2SA/backup-manager/internal/api/handlers"
	apimiddleware "github.com/MD2SA/backup-manager/internal/api/middleware"
	"github.com/MD2SA/backup-manager/internal/monitor"
	"github.com/MD2SA/backup-manager/internal/repository"
	"github.com/MD2SA/backup-manager/internal/repository/db"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgtype"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

func New(
	logger *slog.Logger,
	repo repository.Repository,
	monitorService *monitor.Service,
	onTrigger func(pgtype.UUID) error,
	onRestore func(context.Context, pgtype.UUID) error,
	onActivate func(db.Profile),
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(apimiddleware.Logger(logger))
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

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
			r.Get("/{id}", executionHandler.Get)
			r.Post("/{id}/restore", restoreHandler.Trigger)
			r.Post("/{id}/pin", executionHandler.Pin)
		})

		r.Route("/profiles", func(r chi.Router) {
			r.Get("/", profileHandler.List)
			r.Post("/", profileHandler.Create)
			r.Route("/{id}", func(r chi.Router) {
				r.Put("/", profileHandler.Update)
				r.Delete("/", profileHandler.Delete)
				r.Post("/run", profileHandler.RunNow)
				r.Post("/activate", profileHandler.Activate)
				r.Get("/executions", executionHandler.ListByProfile)
			})
		})
	})

	return r
}
