package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func (p *Postgres) Up(ctx context.Context, cfg Config) error {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	dsn, err := buildPostgresDSN(cfg)
	if err != nil {
		return fmt.Errorf("build dsn: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	dirAbs, err := filepath.Abs(cfg.MigrationsDir)
	if err != nil {
		return fmt.Errorf("resolve migrations dir: %w", err)
	}

	goose.SetDialect("postgres")
	if err := goose.UpContext(ctx, db, dirAbs); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}

func Down(ctx context.Context, cfg Config) error {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	dsn, err := buildPostgresDSN(cfg)
	if err != nil {
		return fmt.Errorf("build dsn: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	dirAbs, err := filepath.Abs(cfg.MigrationsDir)
	if err != nil {
		return fmt.Errorf("resolve migrations dir: %w", err)
	}

	goose.SetDialect("postgres")
	if err := goose.DownContext(ctx, db, dirAbs); err != nil {
		return fmt.Errorf("goose down: %w", err)
	}
	return nil
}

func buildPostgresDSN(cfg Config) (string, error) {
	if cfg.Host == "" || cfg.Port == 0 || cfg.User == "" || cfg.DBName == "" {
		return "", fmt.Errorf("missing required postgres config (host/port/user/dbname)")
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   cfg.DBName,
	}

	q := url.Values{}
	if cfg.SSLMode != "" {
		q.Set("sslmode", cfg.SSLMode)
	}

	u.RawQuery = q.Encode()

	return u.String(), nil
}
