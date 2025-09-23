package storage

import (
    "fmt"
	"context"

    // "github.com/jackc/pgx/v5"
)

// type Database struct {
// 	conn *pgx.Conn
// }

func (db *Database) InitSchema(ctx context.Context) error {
    queries := []string{
        `CREATE TABLE IF NOT EXISTS users (
            id BIGINT PRIMARY KEY,
            state TEXT NOT NULL DEFAULT 'new'
        );`,
        `CREATE TABLE IF NOT EXISTS answers (
            id SERIAL PRIMARY KEY,
            user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            question_id INT NOT NULL,
            answer TEXT NOT NULL
        );`,
    }

	for _, q := range queries {
		if _, err := db.conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("cannot execute query: %w", err)
		}
	}

    return nil
}