package services

import (
	"claudechat/models"
	"database/sql"
	"path/filepath"
	"sync"
	_ "modernc.org/sqlite"
)

type DatabaseService struct {
	db   *sql.DB
	mu   sync.Mutex
	path string
}

func NewDatabaseService(exeDir string) *DatabaseService {
	dbPath := filepath.Join(exeDir, "data.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		panic(err)
	}
	svc := &DatabaseService{db: db, path: dbPath}
	svc.initTables()
	return svc
}

func (d *DatabaseService) initTables() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.db.Exec(`CREATE TABLE IF NOT EXISTS conversations (
		id TEXT PRIMARY KEY,
		name TEXT DEFAULT '',
		first_message TEXT DEFAULT '',
		message_count INTEGER DEFAULT 0,
		last_used_time DATETIME DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	d.db.Exec(`CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
	)`)
	d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id)`)
	d.db.Exec(`CREATE TABLE IF NOT EXISTS cld_conversation (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uid TEXT NOT NULL UNIQUE,
		device_id INTEGER NOT NULL DEFAULT 0
	)`)
	d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_conversation_uid ON cld_conversation(uid)`)
	d.db.Exec(`CREATE TABLE IF NOT EXISTS cld_dialogue (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uid TEXT NOT NULL UNIQUE,
		conversation_id INTEGER NOT NULL,
		dialogue_order INTEGER NOT NULL DEFAULT 1,
		user_message TEXT NOT NULL,
		assistant_message TEXT,
		create_time DATETIME DEFAULT CURRENT_TIMESTAMP,
		finish_time DATETIME,
		request_time DATETIME,
		status TEXT NOT NULL DEFAULT 'processing',
		duration INTEGER,
		FOREIGN KEY (conversation_id) REFERENCES cld_conversation(id) ON DELETE CASCADE
	)`)
	d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_dialogue_conversation_id ON cld_dialogue(conversation_id)`)
	d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_dialogue_uid ON cld_dialogue(uid)`)
	d.db.Exec(`CREATE INDEX IF NOT EXISTS idx_cld_dialogue_status ON cld_dialogue(status)`)
}

func (d *DatabaseService) GetConversations() ([]models.LocalConversation, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rows, err := d.db.Query(`SELECT id, name, first_message, message_count, last_used_time FROM conversations ORDER BY last_used_time DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var convs []models.LocalConversation
	for rows.Next() {
		var c models.LocalConversation
		err := rows.Scan(&c.ConversationId, &c.Name, &c.FirstMessage, &c.MessageCount, &c.LastUsedTime)
		if err != nil {
			continue
		}
		c.IsGenerating = false
		convs = append(convs, c)
	}
	return convs, nil
}

func (d *DatabaseService) GetMessages(conversationId string) ([]models.LocalMessage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rows, err := d.db.Query(`SELECT id, conversation_id, role, content, timestamp FROM messages WHERE conversation_id = ? ORDER BY timestamp ASC`, conversationId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []models.LocalMessage
	for rows.Next() {
		var m models.LocalMessage
		err := rows.Scan(&m.Id, &m.ConversationId, &m.Role, &m.Content, &m.Timestamp)
		if err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (d *DatabaseService) SaveMessage(conversationId, role, content string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`INSERT INTO messages (conversation_id, role, content, timestamp) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`, conversationId, role, content)
	if err != nil {
		return err
	}
	_, err = d.db.Exec(`UPDATE conversations SET message_count = message_count + 1, last_used_time = CURRENT_TIMESTAMP WHERE id = ?`, conversationId)
	if err != nil {
		return err
	}
	if role == "user" {
		var count int
		d.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE conversation_id = ? AND role = 'user'`, conversationId).Scan(&count)
		if count == 1 {
			d.db.Exec(`UPDATE conversations SET first_message = ? WHERE id = ?`, content, conversationId)
		}
	}
	return nil
}

func (d *DatabaseService) CreateConversation(id, firstMessage string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`INSERT INTO conversations (id, first_message, message_count, last_used_time) VALUES (?, ?, 0, CURRENT_TIMESTAMP)`, id, firstMessage)
	return err
}

func (d *DatabaseService) RenameConversation(id, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`UPDATE conversations SET name = ? WHERE id = ?`, name, id)
	return err
}

func (d *DatabaseService) DeleteConversation(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.db.Exec(`DELETE FROM messages WHERE conversation_id = ?`, id)
	_, err := d.db.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	return err
}

func (d *DatabaseService) ConversationExists(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	var count int
	d.db.QueryRow(`SELECT COUNT(*) FROM conversations WHERE id = ?`, id).Scan(&count)
	return count > 0
}

func (d *DatabaseService) GetOrCreateCldConversation(uid string) (*models.LocalConv, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var conv models.LocalConv
	err := d.db.QueryRow(`SELECT id, uid, device_id FROM cld_conversation WHERE uid = ?`, uid).Scan(&conv.ID, &conv.UID, &conv.DeviceID)
	if err == nil {
		return &conv, nil
	}
	result, err := d.db.Exec(`INSERT INTO cld_conversation (uid, device_id) VALUES (?, 0)`, uid)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	conv.ID = int(id)
	conv.UID = uid
	conv.DeviceID = 0
	return &conv, nil
}

func (d *DatabaseService) GetCldConversationByUID(uid string) (*models.LocalConv, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var conv models.LocalConv
	err := d.db.QueryRow(`SELECT id, uid, device_id FROM cld_conversation WHERE uid = ?`, uid).Scan(&conv.ID, &conv.UID, &conv.DeviceID)
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

func (d *DatabaseService) CreateCldDialogue(dialogue *models.LocalDialogue) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	result, err := d.db.Exec(`INSERT INTO cld_dialogue (uid, conversation_id, dialogue_order, user_message, assistant_message, create_time, finish_time, request_time, status, duration) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		dialogue.UID, dialogue.ConversationID, dialogue.Order, dialogue.UserMessage, dialogue.AssistantMessage, dialogue.CreateTime, dialogue.FinishTime, dialogue.RequestTime, dialogue.Status, dialogue.Duration)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	dialogue.ID = int(id)
	return nil
}

func (d *DatabaseService) UpdateCldDialogue(dialogue *models.LocalDialogue) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`UPDATE cld_dialogue SET assistant_message = ?, finish_time = ?, status = ?, duration = ? WHERE id = ?`,
		dialogue.AssistantMessage, dialogue.FinishTime, dialogue.Status, dialogue.Duration, dialogue.ID)
	return err
}

func (d *DatabaseService) GetCldDialogueByID(id int) (*models.LocalDialogue, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var dialogue models.LocalDialogue
	err := d.db.QueryRow(`SELECT id, uid, conversation_id, dialogue_order, user_message, assistant_message, create_time, finish_time, request_time, status, duration FROM cld_dialogue WHERE id = ?`, id).Scan(
		&dialogue.ID, &dialogue.UID, &dialogue.ConversationID, &dialogue.Order, &dialogue.UserMessage, &dialogue.AssistantMessage, &dialogue.CreateTime, &dialogue.FinishTime, &dialogue.RequestTime, &dialogue.Status, &dialogue.Duration)
	if err != nil {
		return nil, err
	}
	return &dialogue, nil
}

func (d *DatabaseService) GetCldDialoguesByConversation(conversationID int) ([]models.LocalDialogue, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rows, err := d.db.Query(`SELECT id, uid, conversation_id, dialogue_order, user_message, assistant_message, create_time, finish_time, request_time, status, duration FROM cld_dialogue WHERE conversation_id = ? ORDER BY dialogue_order ASC`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dialogues []models.LocalDialogue
	for rows.Next() {
		var d models.LocalDialogue
		err := rows.Scan(&d.ID, &d.UID, &d.ConversationID, &d.Order, &d.UserMessage, &d.AssistantMessage, &d.CreateTime, &d.FinishTime, &d.RequestTime, &d.Status, &d.Duration)
		if err != nil {
			continue
		}
		dialogues = append(dialogues, d)
	}
	return dialogues, nil
}

func (d *DatabaseService) GetNextCldDialogueOrder(conversationID int) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var maxOrder sql.NullInt64
	err := d.db.QueryRow(`SELECT MAX(dialogue_order) FROM cld_dialogue WHERE conversation_id = ?`, conversationID).Scan(&maxOrder)
	if err != nil {
		return 1, err
	}
	if !maxOrder.Valid {
		return 1, nil
	}
	return int(maxOrder.Int64) + 1, nil
}

func (d *DatabaseService) DeleteCldConversation(id int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.db.Exec(`DELETE FROM cld_dialogue WHERE conversation_id = ?`, id)
	_, err := d.db.Exec(`DELETE FROM cld_conversation WHERE id = ?`, id)
	return err
}
