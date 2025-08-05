package storage

import (
	"database/sql"
	_ "github.com/lib/pq"
	"time"
)

type Message struct {
	ID			int
	Username	string
	Content		string
	CreatedAt	time.Time
}

type Storage struct {
	db *sql.DB
}

func NewStorage(connStr string) (*Storage, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	return &Storage{db: db}, nil
}

func (s *Storage) SaveMessage(msg Message) error {
	_, err := s.db.Exec(
		"INSERT INTO messages (username, content) VALUES ($1, $2)",
		msg.Username, msg.Content,
	)
	return err
}

func (s *Storage) GetRecentMessages(limit int) ([]Message, error) {
    rows, err := s.db.Query(
        "SELECT id, username, content, created_at FROM messages ORDER BY created_at DESC LIMIT $1", limit,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var messages []Message
    for rows.Next() {
        var m Message
        if err := rows.Scan(&m.ID, &m.Username, &m.Content, &m.CreatedAt); err != nil {
            return nil, err
        }
        messages = append(messages, m)
    }
    return messages, nil
}

func (s *Storage) Close() error {
    return s.db.Close()
}