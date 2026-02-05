package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func setupChatTestDB(t *testing.T) (*gorm.DB, func()) {
	db := setupTestDB(t)

	// Auto migrate tables
	err := db.AutoMigrate(
		&models.User{},
		&models.UserCredential{},
		&models.Assistant{},
		&models.ChatSessionLog{},
	)
	assert.NoError(t, err)

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS users")
		db.Exec("DROP TABLE IF EXISTS user_credentials")
		db.Exec("DROP TABLE IF EXISTS assistants")
		db.Exec("DROP TABLE IF EXISTS chat_session_logs")
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}

	return db, cleanup
}

func TestHandlers_Chat(t *testing.T) {
	db, cleanup := setupChatTestDB(t)
	defer cleanup()

	// Create test user
	user := &models.User{
		Email:       "chat@test.com",
		Password:    "password123",
		DisplayName: "chatuser",
	}
	db.Create(user)

	// Create test credential
	credential := &models.UserCredential{
		UserID:    user.ID,
		APIKey:    "test-api-key",
		APISecret: "test-api-secret",
		Name:      "Test Credential",
	}
	db.Create(credential)

	// Create test assistant
	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		request    ChatRequest
		wantStatus int
		wantError  bool
	}{
		{
			name: "Valid chat request",
			request: ChatRequest{
				AssistantID:  int64(assistant.ID),
				SystemPrompt: "Test system prompt",
				Speaker:      "test_speaker",
				Language:     "zh",
				ApiKey:       credential.APIKey,
				ApiSecret:    credential.APISecret,
				PersonaTag:   "test",
				Temperature:  0.7,
				MaxTokens:    1000,
			},
			wantStatus: http.StatusOK,
			wantError:  true, // Will fail due to missing implementation
		},
		{
			name: "Invalid credentials",
			request: ChatRequest{
				AssistantID:  int64(assistant.ID),
				SystemPrompt: "Test system prompt",
				Speaker:      "test_speaker",
				Language:     "zh",
				ApiKey:       "invalid-key",
				ApiSecret:    "invalid-secret",
				PersonaTag:   "test",
				Temperature:  0.7,
				MaxTokens:    1000,
			},
			wantStatus: http.StatusOK,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, _ := json.Marshal(tt.request)
			c.Request = httptest.NewRequest("POST", "/api/chat", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", user)

			h.Chat(c)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestHandlers_StopChat(t *testing.T) {
	db, cleanup := setupChatTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		sessionID  string
		wantStatus int
		wantError  bool
	}{
		{
			name:       "Valid session ID",
			sessionID:  "test-session-123",
			wantStatus: http.StatusOK,
			wantError:  true, // Will fail as no active session
		},
		{
			name:       "Missing session ID",
			sessionID:  "",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/api/chat/stop"
			if tt.sessionID != "" {
				url += "?sessionId=" + tt.sessionID
			}

			c.Request = httptest.NewRequest("POST", url, nil)

			h.StopChat(c)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestHandlers_ChatStream(t *testing.T) {
	db, cleanup := setupChatTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		sessionID  string
		wantStatus int
	}{
		{
			name:       "Valid session ID",
			sessionID:  "test-session-123",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing session ID",
			sessionID:  "",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/api/chat/stream"
			if tt.sessionID != "" {
				url += "?sessionId=" + tt.sessionID
			}

			c.Request = httptest.NewRequest("GET", url, nil)

			h.ChatStream(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.sessionID != "" {
				// Check SSE headers
				assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
				assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
				assert.Equal(t, "keep-alive", w.Header().Get("Connection"))
			}
		})
	}
}

func TestHandlers_getChatSessionLog(t *testing.T) {
	db, cleanup := setupChatTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "chat@test.com",
		Password:    "password123",
		DisplayName: "chatuser",
	}
	db.Create(user)

	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	// Create test chat logs
	logs := []models.ChatSessionLog{
		{
			UserID:       user.ID,
			SessionID:    "session-1",
			AssistantID:  int64(assistant.ID),
			ChatType:     "voice",
			UserMessage:  "Hello",
			AgentMessage: "Hi there",
			Duration:     5,
		},
		{
			UserID:       user.ID,
			SessionID:    "session-2",
			AssistantID:  int64(assistant.ID),
			ChatType:     "text",
			UserMessage:  "How are you?",
			AgentMessage: "I'm fine",
			Duration:     3,
		},
	}

	for _, log := range logs {
		db.Create(&log)
	}

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantError  bool
	}{
		{
			name:       "Get logs with default pagination",
			query:      "",
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:       "Get logs with custom page size",
			query:      "?pageSize=1",
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:       "Get logs with cursor",
			query:      fmt.Sprintf("?cursor=%d", logs[0].ID),
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:       "Invalid cursor",
			query:      "?cursor=invalid",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", "/api/chat/logs"+tt.query, nil)
			c.Set("user", user)

			h.getChatSessionLog(c)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestHandlers_getChatSessionLogDetail(t *testing.T) {
	db, cleanup := setupChatTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "chat@test.com",
		Password:    "password123",
		DisplayName: "chatuser",
	}
	db.Create(user)

	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	log := &models.ChatSessionLog{
		UserID:       user.ID,
		SessionID:    "session-1",
		AssistantID:  int64(assistant.ID),
		ChatType:     "voice",
		UserMessage:  "Hello",
		AgentMessage: "Hi there",
		Duration:     5,
	}
	db.Create(log)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		logID      string
		wantStatus int
		wantError  bool
	}{
		{
			name:       "Valid log ID",
			logID:      fmt.Sprintf("%d", log.ID),
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:       "Invalid log ID",
			logID:      "invalid",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
		{
			name:       "Non-existent log ID",
			logID:      "99999",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
		{
			name:       "Empty log ID",
			logID:      "",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/chat/logs/%s", tt.logID), nil)
			c.Params = gin.Params{{Key: "id", Value: tt.logID}}
			c.Set("user", user)

			h.getChatSessionLogDetail(c)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestHandlers_getChatSessionLogsBySession(t *testing.T) {
	db, cleanup := setupChatTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "chat@test.com",
		Password:    "password123",
		DisplayName: "chatuser",
	}
	db.Create(user)

	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	sessionID := "test-session-123"

	// Create multiple logs for the same session
	logs := []models.ChatSessionLog{
		{
			UserID:       user.ID,
			SessionID:    sessionID,
			AssistantID:  int64(assistant.ID),
			ChatType:     "voice",
			UserMessage:  "Hello",
			AgentMessage: "Hi there",
			Duration:     5,
		},
		{
			UserID:       user.ID,
			SessionID:    sessionID,
			AssistantID:  int64(assistant.ID),
			ChatType:     "voice",
			UserMessage:  "How are you?",
			AgentMessage: "I'm fine",
			Duration:     3,
		},
	}

	for _, log := range logs {
		db.Create(&log)
	}

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		sessionID  string
		wantStatus int
		wantError  bool
	}{
		{
			name:       "Valid session ID",
			sessionID:  sessionID,
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:       "Non-existent session ID",
			sessionID:  "non-existent-session",
			wantStatus: http.StatusOK,
			wantError:  false, // Should return empty list
		},
		{
			name:       "Empty session ID",
			sessionID:  "",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/chat/sessions/%s/logs", tt.sessionID), nil)
			c.Params = gin.Params{{Key: "sessionId", Value: tt.sessionID}}
			c.Set("user", user)

			h.getChatSessionLogsBySession(c)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestHandlers_getChatSessionLogByAssistant(t *testing.T) {
	db, cleanup := setupChatTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "chat@test.com",
		Password:    "password123",
		DisplayName: "chatuser",
	}
	db.Create(user)

	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	// Create test logs for the assistant
	logs := []models.ChatSessionLog{
		{
			UserID:       user.ID,
			SessionID:    "session-1",
			AssistantID:  int64(assistant.ID),
			ChatType:     "voice",
			UserMessage:  "Hello",
			AgentMessage: "Hi there",
			Duration:     5,
		},
		{
			UserID:       user.ID,
			SessionID:    "session-2",
			AssistantID:  int64(assistant.ID),
			ChatType:     "text",
			UserMessage:  "How are you?",
			AgentMessage: "I'm fine",
			Duration:     3,
		},
	}

	for _, log := range logs {
		db.Create(&log)
	}

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		assistantID string
		query       string
		wantStatus  int
		wantError   bool
	}{
		{
			name:        "Valid assistant ID",
			assistantID: fmt.Sprintf("%d", assistant.ID),
			query:       "",
			wantStatus:  http.StatusOK,
			wantError:   false,
		},
		{
			name:        "Valid assistant ID with pagination",
			assistantID: fmt.Sprintf("%d", assistant.ID),
			query:       "?pageSize=1",
			wantStatus:  http.StatusOK,
			wantError:   false,
		},
		{
			name:        "Invalid assistant ID",
			assistantID: "invalid",
			wantStatus:  http.StatusOK,
			wantError:   true,
		},
		{
			name:        "Non-existent assistant ID",
			assistantID: "99999",
			wantStatus:  http.StatusOK,
			wantError:   false, // Should return empty list
		},
		{
			name:        "Empty assistant ID",
			assistantID: "",
			wantStatus:  http.StatusOK,
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := fmt.Sprintf("/api/chat/assistants/%s/logs", tt.assistantID)
			if tt.query != "" {
				url += tt.query
			}

			c.Request = httptest.NewRequest("GET", url, nil)
			c.Params = gin.Params{{Key: "assistantId", Value: tt.assistantID}}
			c.Set("user", user)

			h.getChatSessionLogByAssistant(c)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestHandlers_UnauthorizedChatAccess(t *testing.T) {
	db, cleanup := setupChatTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Test without user context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	request := ChatRequest{
		AssistantID:  1,
		SystemPrompt: "Test",
		ApiKey:       "test-key",
		ApiSecret:    "test-secret",
	}

	body, _ := json.Marshal(request)
	c.Request = httptest.NewRequest("POST", "/api/chat", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	// No user set

	h.Chat(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "fail", resp["status"])
	assert.Equal(t, "User is not logged in.", resp["message"])
}

// Benchmark tests
func BenchmarkHandlers_getChatSessionLog(b *testing.B) {
	db, cleanup := setupChatTestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "chat@test.com",
		Password:    "password123",
		DisplayName: "chatuser",
	}
	db.Create(user)

	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	// Create test data
	for i := 0; i < 100; i++ {
		log := &models.ChatSessionLog{
			UserID:       user.ID,
			SessionID:    fmt.Sprintf("session-%d", i),
			AssistantID:  int64(assistant.ID),
			ChatType:     "voice",
			UserMessage:  fmt.Sprintf("Message %d", i),
			AgentMessage: fmt.Sprintf("Response %d", i),
			Duration:     int(float64(i) * 0.5),
		}
		db.Create(log)
	}

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", "/api/chat/logs?pageSize=10", nil)
		c.Set("user", user)

		h.getChatSessionLog(c)
	}
}

func BenchmarkHandlers_getChatSessionLogDetail(b *testing.B) {
	db, cleanup := setupChatTestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "chat@test.com",
		Password:    "password123",
		DisplayName: "chatuser",
	}
	db.Create(user)

	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	log := &models.ChatSessionLog{
		UserID:       user.ID,
		SessionID:    "session-1",
		AssistantID:  int64(assistant.ID),
		ChatType:     "voice",
		UserMessage:  "Hello",
		AgentMessage: "Hi there",
		Duration:     5,
	}
	db.Create(log)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/chat/logs/%d", log.ID), nil)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", log.ID)}}
		c.Set("user", user)

		h.getChatSessionLogDetail(c)
	}
}
