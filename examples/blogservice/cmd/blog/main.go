package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"

	"example.com/blogservice/internal/blog/migrations"
	"example.com/blogservice/internal/blog/postgres"
	"example.com/blogservice/internal/blog/restapi"
)

const (
	envDatabaseURL    = "DATABASE_URL"
	envHTTPAddr       = "HTTP_ADDR"
	defaultHTTPAddr   = ":8080"
	gooseDialect      = "postgres"
	migrationsRoot    = "."
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
)

var errNoDatabaseURL = errors.New(envDatabaseURL + " is not set")

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(); err != nil {
		slog.Error("failed to run service", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := os.Getenv(envDatabaseURL)
	if dsn == "" {
		return errNoDatabaseURL
	}

	addr := os.Getenv(envHTTPAddr)
	if addr == "" {
		addr = defaultHTTPAddr
	}

	db, err := postgres.NewDB(dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("failed to close database", "error", err)
		}
	}()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	if err := migrate(ctx, db.DB); err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           newHandler(db),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	serveErr := make(chan error, 1)

	go func() {
		slog.Info("starting http server", "addr", addr)

		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down http server: %w", err)
	}

	return nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)

	if err := goose.SetDialect(gooseDialect); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, migrationsRoot); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

func newHandler(db *sqlx.DB) http.Handler {
	return restapi.NewRouter(restapi.Deps{
		Posts:    postgres.NewPostRepository(db),
		Authors:  postgres.NewAuthorRepository(db),
		Comments: postgres.NewCommentRepository(db),
		Tags:     postgres.NewTagRepository(db),
	})
}
