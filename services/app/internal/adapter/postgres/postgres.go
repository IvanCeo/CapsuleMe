package postgres

import (
	"capsule-me/internal/domain/catalog"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	pool *pgxpool.Pool
}

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string

	MigrationsDir string
	Timeout       time.Duration
}

func LoadConfigFromEnv() Config {
	port, err := strconv.Atoi(getEnv("POSTGRES_PORT", "5432"))
	if err != nil {
		port = 5432
	}
	return Config{
		MigrationsDir: getEnv("MIGRATIONS_DIR", "./internal/adapter/postgres/migrations"),
		Timeout:       mustDuration("MIGRATIONS_TIMEOUT", 30*time.Second),

		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     port,
		User:     getEnv("POSTGRES_USER", "postgres"),
		Password: getEnv("POSTGRES_PASSWORD", "postgres"),
		DBName:   getEnv("POSTGRES_DB", "postgres"),
		SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
	}
}

func NewPostgres(ctx context.Context) (*Postgres, error) {
	cfg := LoadConfigFromEnv()
	url := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName, cfg.SSLMode,
	)

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("errNewPool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping failed: %w", err)
	}

	db := &Postgres{pool: pool}
	// logger: connected to postgres

	return db, nil
}

func (p *Postgres) Close() { p.pool.Close() }

func (p *Postgres) GetItemByFeatures(ctx context.Context, in *catalog.IncomingFeature) ([]*catalog.ImageItem, error) {
	baseQuery := `
	SELECT 
	id, object_id, gender, category, style, color, season, material, description
	FROM image_items
	`
	var (
		conds []string
		args  []any
	)

	if in.Gender != "" {
		args = append(args, in.Gender)
		conds = append(conds, fmt.Sprintf("gender = $%d", len(args)))
	}
	if in.Category != "" {
		args = append(args, in.Category)
		conds = append(conds, fmt.Sprintf("category = $%d", len(args)))
	}
	if in.Style != "" {
		args = append(args, in.Style)
		conds = append(conds, fmt.Sprintf("style = $%d", len(args)))
	}
	if in.Color != "" {
		args = append(args, in.Color)
		conds = append(conds, fmt.Sprintf("color = $%d", len(args)))
	}
	if in.Season != "" {
		args = append(args, in.Season)
		conds = append(conds, fmt.Sprintf("season = $%d", len(args)))
	}
	if in.Material != "" {
		args = append(args, in.Material)
		conds = append(conds, fmt.Sprintf("material = $%d", len(args)))
	}

	query := baseQuery
	if len(conds) > 0 {
		query += "WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY id LIMIT 50;"

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var items []*catalog.ImageItem
	for rows.Next() {
		var i catalog.ImageItem
		if err := rows.Scan(
			&i.ID,
			&i.ObjectID,
			&i.Gender,
			&i.Category,
			&i.Style,
			&i.Color,
			&i.Season,
			&i.Material,
			&i.Description,
		); err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		items = append(items, &i)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return items, nil
}

func (p *Postgres) SaveFeedback(ctx context.Context, key, value string) error {
	q := fmt.Sprintf(`insert into feedback (score, capsule)	values ('%s', '%s');`, key, value)
	_, err := p.pool.Exec(ctx, q)
	return err
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func mustDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		panic(fmt.Sprintf("invalid duration in %s: %s", key, v))
	}
	return d
}
