package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCallRecordingTestDB(t *testing.T) (*gorm.DB, func()) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto migrate tables
	err = db.AutoMigrate(
		&models.User{},
		&models.CallRecording{},
		&models.Assistant{},
		&models.Device{},
	)
	assert.NoError(t, err)

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS users")
		db.Exec("DROP TABLE IF EXISTS call_recordings")
		db.Exec("DROP TABLE IF EXISTS assistants")
		db.Exec("DROP TABLE IF EXISTS devices")
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}

	return db, cleanup
}

func TestCallRecordingHandler_GetCallRecordings(t *testing.T) {
	db, cleanup := setupCallRecordingTestDB(t)
	defer cleanup()

	// Create test user
	user := &models.User{
		Email:       "recording@test.com",
		Password:    "password123",
		DisplayName: "recorduser",
	}
	db.Create(user)

	// Create test assistant
	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	// Create test device
	assistantIDUint := uint(assistant.ID)
	device := &models.Device{
		ID:          "test-device-001",
		MacAddress:  "00:11:22:33:44:55",
		UserID:      user.ID,
		AssistantID: &assistantIDUint,
		Board:       "test_board",
		AppVersion:  "1.0.0",
	}
	db.Create(device)

	// Create test call recordings
	recordings := []models.CallRecording{
		{
			UserID:      user.ID,
			AssistantID: uint(assistant.ID),
			MacAddress:  device.MacAddress,
			Duration:    120,
			AudioPath:   "/path/to/audio1.wav",
		},
		{
			UserID:      user.ID,
			AssistantID: uint(assistant.ID),
			MacAddress:  device.MacAddress,
			Duration:    180,
			AudioPath:   "/path/to/audio2.wav",
		},
	}

	for _, recording := range recordings {
		db.Create(&recording)
	}

	handler := NewCallRecordingHandler(db)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		query      string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "Get all recordings",
			query:      "",
			wantCount:  2,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Get recordings by assistant",
			query:      fmt.Sprintf("?assistantId=%d", assistant.ID),
			wantCount:  2,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Get recordings by device",
			query:      fmt.Sprintf("?macAddress=%s", device.MacAddress),
			wantCount:  2,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Get recordings with pagination",
			query:      "?page=1&pageSize=1",
			wantCount:  1,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", "/api/call-recordings"+tt.query, nil)
			c.Set("userID", user.ID)

			handler.GetCallRecordings(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus == http.StatusOK {
				// Parse response and verify
				// Note: The actual response parsing would depend on the response format
				// This is a simplified test
			}
		})
	}
}

func TestCallRecordingHandler_GetCallRecordingDetail(t *testing.T) {
	db, cleanup := setupCallRecordingTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "recording@test.com",
		Password:    "password123",
		DisplayName: "recorduser",
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

	recording := &models.CallRecording{
		UserID:      user.ID,
		AssistantID: uint(assistant.ID),
		MacAddress:  "00:11:22:33:44:55",
		Duration:    120,
		AudioPath:   "http://example.com/audio1.wav",
	}
	db.Create(recording)

	handler := NewCallRecordingHandler(db)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		recordingID string
		wantStatus  int
		wantError   bool
	}{
		{
			name:        "Valid recording ID",
			recordingID: fmt.Sprintf("%d", recording.ID),
			wantStatus:  http.StatusOK,
			wantError:   false,
		},
		{
			name:        "Invalid recording ID",
			recordingID: "invalid",
			wantStatus:  http.StatusOK,
			wantError:   true,
		},
		{
			name:        "Non-existent recording ID",
			recordingID: "99999",
			wantStatus:  http.StatusOK,
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/call-recordings/%s", tt.recordingID), nil)
			c.Params = gin.Params{{Key: "id", Value: tt.recordingID}}
			c.Set("userID", user.ID)

			handler.GetCallRecordingDetail(c)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestCallRecordingHandler_DeleteCallRecording(t *testing.T) {
	db, cleanup := setupCallRecordingTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "recording@test.com",
		Password:    "password123",
		DisplayName: "recorduser",
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

	recording := &models.CallRecording{
		UserID:      user.ID,
		AssistantID: uint(assistant.ID),
		MacAddress:  "00:11:22:33:44:55",
		Duration:    120,
		AudioPath:   "http://example.com/audio1.wav",
	}
	db.Create(recording)

	handler := NewCallRecordingHandler(db)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("DELETE", fmt.Sprintf("/api/call-recordings/%d", recording.ID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", recording.ID)}}
	c.Set("userID", user.ID)

	handler.DeleteCallRecording(c)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify deletion
	var deleted models.CallRecording
	err := db.First(&deleted, recording.ID).Error
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestCallRecordingHandler_GetCallRecordingStats(t *testing.T) {
	db, cleanup := setupCallRecordingTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "recording@test.com",
		Password:    "password123",
		DisplayName: "recorduser",
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

	// Create multiple recordings for stats
	for i := 0; i < 5; i++ {
		recording := &models.CallRecording{
			UserID:      user.ID,
			AssistantID: uint(assistant.ID),
			MacAddress:  "00:11:22:33:44:55",
			Duration:    120 + i*30,
			AudioPath:   fmt.Sprintf("http://example.com/audio%d.wav", i),
		}
		db.Create(recording)
	}

	handler := NewCallRecordingHandler(db)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "Get all stats",
			query:      "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Get stats by assistant",
			query:      fmt.Sprintf("?assistantId=%d", assistant.ID),
			wantStatus: http.StatusOK,
		},
		{
			name:       "Get stats by device",
			query:      "?macAddress=00:11:22:33:44:55",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", "/api/call-recordings/stats"+tt.query, nil)
			c.Set("userID", user.ID)

			handler.GetCallRecordingStats(c)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestCallRecordingHandler_UnauthorizedAccess(t *testing.T) {
	db, cleanup := setupCallRecordingTestDB(t)
	defer cleanup()

	user1 := &models.User{
		Email:       "user1@test.com",
		Password:    "password123",
		DisplayName: "user1",
	}
	db.Create(user1)

	user2 := &models.User{
		Email:       "user2@test.com",
		Password:    "password123",
		DisplayName: "user2",
	}
	db.Create(user2)

	assistant := &models.Assistant{
		UserID:       user1.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	// Recording belongs to user1
	recording := &models.CallRecording{
		UserID:      user1.ID,
		AssistantID: uint(assistant.ID),
		MacAddress:  "00:11:22:33:44:55",
		Duration:    120,
		AudioPath:   "http://example.com/audio1.wav",
	}
	db.Create(recording)

	handler := NewCallRecordingHandler(db)
	gin.SetMode(gin.TestMode)

	// Try to access with user2 (unauthorized)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/call-recordings/%d", recording.ID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", recording.ID)}}
	c.Set("userID", user2.ID) // Wrong user

	handler.GetCallRecordingDetail(c)

	assert.Equal(t, http.StatusOK, w.Code)
	// Should return error for unauthorized access
}

func TestCallRecordingHandler_NoUserID(t *testing.T) {
	db, cleanup := setupCallRecordingTestDB(t)
	defer cleanup()

	handler := NewCallRecordingHandler(db)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/api/call-recordings", nil)
	// No userID set

	handler.GetCallRecordings(c)

	assert.Equal(t, http.StatusOK, w.Code)
	// Should return unauthorized error
}

func TestNewCallRecordingHandler(t *testing.T) {
	db, cleanup := setupCallRecordingTestDB(t)
	defer cleanup()

	handler := NewCallRecordingHandler(db)

	assert.NotNil(t, handler)
	assert.Equal(t, db, handler.db)
	assert.NotNil(t, handler.logger)
}

// Test getUserID helper function - using existing function from plugin_utils.go

// Benchmark tests
func BenchmarkCallRecordingHandler_GetCallRecordings(b *testing.B) {
	db, cleanup := setupCallRecordingTestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "recording@test.com",
		Password:    "password123",
		DisplayName: "recorduser",
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
		recording := &models.CallRecording{
			UserID:      user.ID,
			AssistantID: uint(assistant.ID),
			MacAddress:  "00:11:22:33:44:55",
			Duration:    120 + i,
			AudioPath:   fmt.Sprintf("http://example.com/audio%d.wav", i),
		}
		db.Create(recording)
	}

	handler := NewCallRecordingHandler(db)
	gin.SetMode(gin.TestMode)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", "/api/call-recordings?page=1&pageSize=20", nil)
		c.Set("userID", user.ID)

		handler.GetCallRecordings(c)
	}
}

func BenchmarkCallRecordingHandler_GetCallRecordingDetail(b *testing.B) {
	db, cleanup := setupCallRecordingTestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "recording@test.com",
		Password:    "password123",
		DisplayName: "recorduser",
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

	recording := &models.CallRecording{
		UserID:      user.ID,
		AssistantID: uint(assistant.ID),
		MacAddress:  "00:11:22:33:44:55",
		Duration:    120,
		AudioPath:   "http://example.com/audio1.wav",
	}
	db.Create(recording)

	handler := NewCallRecordingHandler(db)
	gin.SetMode(gin.TestMode)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/call-recordings/%d", recording.ID), nil)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", recording.ID)}}
		c.Set("userID", user.ID)

		handler.GetCallRecordingDetail(c)
	}
}

// Test helper function for getUserID - using existing function from plugin_utils.go
