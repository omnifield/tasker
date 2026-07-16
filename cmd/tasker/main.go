// Command tasker — REST-бэк «дерева разработки». Env-конфиг, идемпотентные миграции на старте,
// bind 0.0.0.0 (G1), graceful shutdown по SIGINT/SIGTERM, structured-логи (slog).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/omnifield/tasker/internal/config"
	"github.com/omnifield/tasker/internal/httpapi"
	"github.com/omnifield/tasker/internal/service"
	"github.com/omnifield/tasker/internal/store"
	"github.com/omnifield/tasker/internal/webhook"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Контекст жив, пока не пришёл сигнал — им же гасим приём новых соединений.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	log.Info("store ready", slog.String("db", cfg.DB))

	svc := service.New(st, webhook.LogEmitter{Logger: log})
	api := httpapi.New(svc, log)

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("tasker listening", slog.String("addr", cfg.Addr()))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutdown signal received, draining")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
