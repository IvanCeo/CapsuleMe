package storage

import (
	"context"
	"fmt"

	// "github.com/jackc/pgx/v5"
)

// type Database struct {
// 	conn *pgx.Conn
// }

type Answer struct {
	ID         int
	UserID     int64
	QuestionID int
	Answer     string
}

func (db *Database) AddAnswer(ctx context.Context, userID int64, questionID int, answer string) error {
	_, err := db.conn.Exec(ctx, `
        INSERT INTO answers (user_id, question_id, answer)
        VALUES ($1, $2, $3)
    `, userID, questionID, answer)
	if err != nil {
		return fmt.Errorf("AddAnswer failed: %w", err)
	}
	return nil
}

func (db *Database) GetAnswersByUser(ctx context.Context, userID int64) ([]Answer, error) {
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

	var answers []Answer
	for rows.Next() {
		var a Answer
		if err := rows.Scan(&a.ID, &a.UserID, &a.QuestionID, &a.Answer); err != nil {
			return nil, fmt.Errorf("row scan failed: %w", err)
		}
		answers = append(answers, a)
	}

	return answers, nil
}
