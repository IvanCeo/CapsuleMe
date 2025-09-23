package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type User struct {
	ID    int64
	State string
}

// type Database struct {
// 	conn *pgx.Conn
// }

func (db *Database) CreateUser(ctx context.Context, u *User) error {
	_, err := db.conn.Exec(ctx, `
        INSERT INTO users (id, state)
        VALUES ($1, $2)
        ON CONFLICT (id) DO NOTHING
    `, u.ID, u.State)
	if err != nil {
		return fmt.Errorf("CreateUser failed: %w", err)
	}
	return nil
}

func (db *Database) GetUser(ctx context.Context, id int64) (*User, error) {
	row := db.conn.QueryRow(ctx, `
        SELECT id, state FROM users WHERE id=$1
    `, id)

	u := &User{}
	if err := row.Scan(&u.ID, &u.State); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetUser failed: %w", err)
	}

	return u, nil
}

func (db *Database) UpdateUserState(ctx context.Context, id int64, state string) error {
	_, err := db.conn.Exec(ctx, `
        UPDATE users SET state=$1 WHERE id=$2
    `, state, id)
	if err != nil {
		return fmt.Errorf("UpdateUserState failed: %w", err)
	}
	return nil
}