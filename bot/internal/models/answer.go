package models

type Answer struct {
    ID         int       `json:"id"`
    UserID     int64     `json:"user_id"`
    QuestionID int       `json:"question_id"`
    Answer      string    `json:"value"`
}
