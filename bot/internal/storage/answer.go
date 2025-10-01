package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"bot/internal/models"
)

func (db *Database) AddAnswer(ctx context.Context, a *models.Answer) error {
	_, err := db.conn.Exec(ctx, `
        INSERT INTO answers (user_id, question_id, answer)
        VALUES ($1, $2, $3)
    `, a.UserID, a.QuestionID, a.Answer)
	if err != nil {
		return fmt.Errorf("AddAnswer failed: %w", err)
	}
	return nil
}

func (db *Database) GetAnswersByUser(ctx context.Context, userID int64) ([]*models.Answer, error) {
	rows, err := db.conn.Query(ctx, `
        SELECT id, user_id, question_id, answer
        FROM answers
        WHERE user_id=$1
        ORDER BY question_id ASC
    `, userID)
	if err != nil {
		return nil, fmt.Errorf("GetAnswersByUser failed: %w", err)
	}
	defer rows.Close()

	var answers []*models.Answer
	for rows.Next() {
		a := &models.Answer{}
		if err := rows.Scan(&a.ID, &a.UserID, &a.QuestionID, &a.Answer); err != nil {
			return nil, fmt.Errorf("row scan failed: %w", err)
		}
		answers = append(answers, a)
	}

	return answers, nil
}

	func (db *Database) GetLastAnswerByUser(ctx context.Context, userID int64) (*models.Answer, error) {
		row := db.conn.QueryRow(ctx, `
			SELECT id, user_id, question_id, answer
			FROM answers
			WHERE user_id=$1
			ORDER BY question_id DESC
			LIMIT 1
		`, userID)

		a := &models.Answer{}
		if err := row.Scan(&a.ID, &a.UserID, &a.QuestionID, &a.Answer); err != nil {
			if err == pgx.ErrNoRows {
				return nil, nil
			}
			return nil, fmt.Errorf("row scan failed: %w", err)
		}

		return a, nil
	}