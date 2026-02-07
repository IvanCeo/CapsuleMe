package postgres

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/pressly/goose"
)

func init() {
	goose.AddMigration(upLoadMetadata, downLoadMetadata)
}

func upLoadMetadata(tx *sql.Tx) error {
	filePath := "data/clean/metadata_uuid.csv"

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReader(f))
	r.Comma = ','

	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("read csv: %w", err)
	}

	ctx := context.Background()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO image_items (
			id, object_id, ext, gender, category, style, color, season, material, description
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO NOTHING;
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for i, rec := range records {
		if len(rec) < 9 {
			return fmt.Errorf("row %d: not enough columns", i)
		}

		id, err := uuid.Parse(strings.TrimSpace(rec[0]))
		if err != nil {
			return fmt.Errorf("row %d invalid uuid: %w", i, err)
		}

		objectID := id

		if _, err := stmt.ExecContext(ctx,
			id, objectID, rec[1], rec[2], rec[3],
			rec[4], rec[5], rec[6], rec[7], rec[8],
		); err != nil {
			return fmt.Errorf("insert row %d: %w", i, err)
		}
	}

	return nil
}

func downLoadMetadata(tx *sql.Tx) error {
	_, err := tx.Exec("DELETE FROM image_items;")
	return err
}
