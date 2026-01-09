package main

import (
	"sync"
	"time"
)

type DialogueSession struct {
	ConversationID  string
	LastMessageUUID string
	LastUsedTime    time.Time
	IsGenerating    bool
	GeneratingMutex sync.RWMutex
	StreamMode      bool
	SSEClosed       bool
}

type DialogueManager struct {
	sessions      map[string]*DialogueSession
	mutex         sync.RWMutex
	cleanupTicker *time.Ticker
	cleanupDone   chan bool
}

func NewDialogueManager() *DialogueManager {
	dm := &DialogueManager{
		sessions:      make(map[string]*DialogueSession),
		cleanupTicker: time.NewTicker(1 * time.Minute),
		cleanupDone:   make(chan bool),
	}

	go dm.cleanupExpiredSessions()
	return dm
}

func (dm *DialogueManager) cleanupExpiredSessions() {
	for {
		select {
		case <-dm.cleanupTicker.C:
			dm.mutex.Lock()
			now := time.Now()
			for id, session := range dm.sessions {
				session.GeneratingMutex.RLock()
				isGenerating := session.IsGenerating
				session.GeneratingMutex.RUnlock()

				timeout := 1 * time.Minute
				if session.StreamMode && session.SSEClosed {
					delete(dm.sessions, id)
					DebugLog("Cleaned up closed SSE dialogue session: %s", id)
				} else if !isGenerating && now.Sub(session.LastUsedTime) > timeout {
					delete(dm.sessions, id)
					DebugLog("Cleaned up expired dialogue session: %s (mode: %v)", id, map[bool]string{true: "stream", false: "http"}[session.StreamMode])
				}
			}
			dm.mutex.Unlock()
		case <-dm.cleanupDone:
			return
		}
	}
}

func (dm *DialogueManager) GetOrCreateSession(conversationID string) *DialogueSession {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	if session, exists := dm.sessions[conversationID]; exists {
		session.LastUsedTime = time.Now()
		return session
	}

	session := &DialogueSession{
		ConversationID:  conversationID,
		LastMessageUUID: "00000000-0000-4000-8000-000000000000",
		LastUsedTime:    time.Now(),
		IsGenerating:    false,
		StreamMode:      false,
		SSEClosed:       false,
	}
	dm.sessions[conversationID] = session
	return session
}

func (dm *DialogueManager) UpdateSession(conversationID, lastMessageUUID string) {
	dm.mutex.RLock()
	session, exists := dm.sessions[conversationID]
	dm.mutex.RUnlock()

	if exists {
		session.GeneratingMutex.Lock()
		session.LastMessageUUID = lastMessageUUID
		session.LastUsedTime = time.Now()
		session.GeneratingMutex.Unlock()
	}
}

func (dm *DialogueManager) DeleteSession(conversationID string) {
	dm.mutex.Lock()
	delete(dm.sessions, conversationID)
	dm.mutex.Unlock()
	DebugLog("Deleted dialogue session: %s", conversationID)
}

func (dm *DialogueManager) SetGenerating(conversationID string, generating bool) {
	dm.mutex.RLock()
	session, exists := dm.sessions[conversationID]
	dm.mutex.RUnlock()

	if exists {
		session.GeneratingMutex.Lock()
		session.IsGenerating = generating
		session.GeneratingMutex.Unlock()
	}
}

func (dm *DialogueManager) TouchSession(conversationID string) {
	dm.mutex.RLock()
	session, exists := dm.sessions[conversationID]
	dm.mutex.RUnlock()

	if exists {
		session.GeneratingMutex.Lock()
		session.LastUsedTime = time.Now()
		session.GeneratingMutex.Unlock()
		DebugLog("Keepalive touched session: %s", conversationID)
	}
}

func (dm *DialogueManager) SetStreamMode(conversationID string, streamMode bool) {
	dm.mutex.RLock()
	session, exists := dm.sessions[conversationID]
	dm.mutex.RUnlock()

	if exists {
		session.GeneratingMutex.Lock()
		session.StreamMode = streamMode
		session.GeneratingMutex.Unlock()
	}
}

func (dm *DialogueManager) MarkSSEClosed(conversationID string) {
	dm.mutex.RLock()
	session, exists := dm.sessions[conversationID]
	dm.mutex.RUnlock()

	if exists {
		session.GeneratingMutex.Lock()
		session.SSEClosed = true
		session.GeneratingMutex.Unlock()
		DebugLog("Marked SSE connection closed: %s", conversationID)
	}
}

func (dm *DialogueManager) Stop() {
	dm.cleanupTicker.Stop()
	dm.cleanupDone <- true
}

func (dm *DialogueManager) GetActiveDialogues() []map[string]interface{} {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	dialogues := make([]map[string]interface{}, 0, len(dm.sessions))
	for id, session := range dm.sessions {
		session.GeneratingMutex.RLock()
		isGenerating := session.IsGenerating
		session.GeneratingMutex.RUnlock()

		dialogues = append(dialogues, map[string]interface{}{
			"conversation_id": id,
			"last_used_time":  session.LastUsedTime,
			"is_generating":   isGenerating,
			"is_closed":       false,
			"message_count":   0,
		})
	}
	return dialogues
}
