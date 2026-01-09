package main

import (
	"database/sql"
	"log"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type LocalStorage struct {
	db *sql.DB
}

type LocalConversation struct {
	ID           string    `json:"conversation_id"`
	Name         string    `json:"name"`
	FirstMessage string    `json:"first_message"`
	MessageCount int       `json:"message_count"`
	LastUsedTime time.Time `json:"last_used_time"`
	IsGenerating bool      `json:"is_generating"`
}

type LocalMessage struct {
	ID             int64     `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	Timestamp      time.Time `json:"timestamp"`
}

func NewLocalStorage() (*LocalStorage, error) {
	dbPath := filepath.Join(getAppDir(), "data.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	storage := &LocalStorage{db: db}
	if err := storage.init(); err != nil {
		return nil, err
	}

	log.Printf("本地存储初始化完成: %s", dbPath)
	return storage, nil
}

func (s *LocalStorage) init() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			name TEXT DEFAULT '',
			first_message TEXT DEFAULT '',
			message_count INTEGER DEFAULT 0,
			last_used_time DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id)
	`)
	return err
}

func (s *LocalStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *LocalStorage) GetConversations() ([]LocalConversation, error) {
	rows, err := s.db.Query(`
		SELECT id, name, first_message, message_count, last_used_time
		FROM conversations
		ORDER BY last_used_time DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []LocalConversation
	for rows.Next() {
		var conv LocalConversation
		var lastUsed string
		if err := rows.Scan(&conv.ID, &conv.Name, &conv.FirstMessage, &conv.MessageCount, &lastUsed); err != nil {
			continue
		}
		conv.LastUsedTime, _ = time.Parse("2006-01-02 15:04:05", lastUsed)
		conversations = append(conversations, conv)
	}
	return conversations, nil
}

func (s *LocalStorage) CreateConversation(id, firstMessage string) error {
	_, err := s.db.Exec(`
		INSERT INTO conversations (id, first_message, message_count, last_used_time)
		VALUES (?, ?, 0, CURRENT_TIMESTAMP)
	`, id, firstMessage)
	return err
}

func (s *LocalStorage) UpdateConversationName(id, name string) error {
	_, err := s.db.Exec(`
		UPDATE conversations SET name = ? WHERE id = ?
	`, name, id)
	return err
}

func (s *LocalStorage) DeleteConversation(id string) error {
	_, err := s.db.Exec(`DELETE FROM messages WHERE conversation_id = ?`, id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	return err
}

func (s *LocalStorage) GetMessages(conversationID string) ([]LocalMessage, error) {
	rows, err := s.db.Query(`
		SELECT id, conversation_id, role, content, timestamp
		FROM messages
		WHERE conversation_id = ?
		ORDER BY timestamp ASC
	`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []LocalMessage
	for rows.Next() {
		var msg LocalMessage
		var timestamp string
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &timestamp); err != nil {
			continue
		}
		msg.Timestamp, _ = time.Parse("2006-01-02 15:04:05", timestamp)
		messages = append(messages, msg)
	}
	return messages, nil
}

func (s *LocalStorage) AddMessage(conversationID, role, content string) error {
	_, err := s.db.Exec(`
		INSERT INTO messages (conversation_id, role, content, timestamp)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, conversationID, role, content)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		UPDATE conversations
		SET message_count = message_count + 1, last_used_time = CURRENT_TIMESTAMP
		WHERE id = ?
	`, conversationID)

	if role == "user" {
		var count int
		s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE conversation_id = ? AND role = 'user'`, conversationID).Scan(&count)
		if count == 1 {
			s.db.Exec(`UPDATE conversations SET first_message = ? WHERE id = ?`, content, conversationID)
		}
	}

	return err
}

func (s *LocalStorage) UpdateLastMessage(conversationID, content string) error {
	_, err := s.db.Exec(`
		UPDATE messages
		SET content = ?
		WHERE id = (
			SELECT id FROM messages
			WHERE conversation_id = ?
			ORDER BY timestamp DESC
			LIMIT 1
		)
	`, content, conversationID)
	return err
}

func (s *LocalStorage) ConversationExists(id string) bool {
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM conversations WHERE id = ?`, id).Scan(&count)
	return count > 0
}
