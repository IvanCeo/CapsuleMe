package storage

import "github.com/jackc/pgx/v5"

type Database struct {
	conn *pgx.Conn
}

func New(conn *pgx.Conn) *Database {
	return &Database{conn: conn}
}