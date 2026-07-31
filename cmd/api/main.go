package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MD2SA/backup-manager/internal/app"
)

// @title Backup Manager API
// @version 1.0
// @description This is a backup management server for Postgres databases.
// @host localhost:8080
// @BasePath /api/v1

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	application, err := app.New(ctx)
	if err != nil {
		os.Stderr.WriteString("Fatal error while starting the application: " + err.Error() + "\n")
		os.Exit(1)
	}

	defer application.DB.Close()

	application.Start(ctx)

	server := &http.Server{
		Addr:    ":" + application.Config.Port,
		Handler: application.Router,
	}

	serverErrors := make(chan error, 1)

	go func() {
		application.Logger.Info("Starting server", "port", application.Config.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		application.Logger.Error("Critical error in HTTP server", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		application.Logger.Info("Shutting down server gracefully...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			application.Logger.Error("Graceful shutdown failed", "error", err)
			server.Close()
		}

		application.Logger.Info("Server shut down successfully")
	}
}
