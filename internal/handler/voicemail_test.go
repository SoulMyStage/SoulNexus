package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNewVoicemailHandler(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	assert.NotNil(t, handler)
	assert.Equal(t, db, handler.db)
	assert.NotNil(t, handler.processor)
}

func TestVoicemailHandler_ListVoicemails(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voicemails
	for i := 0; i < 5; i++ {
		voicemail := &models.Voicemail{
			UserID:       user.ID,
			CallerNumber: fmt.Sprintf("1234567890%d", i),
			AudioPath:    fmt.Sprintf("/path/to/audio%d.wav", i),
			Duration:     30 + i,
			Status:       models.VoicemailStatusNew,
			IsRead:       false,
		}
		require.NoError(t, db.Create(voicemail).Error)
	}

	tests := []struct {
		name           string
		queryParams    string
		user           *models.User
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "List all voicemails",
			queryParams:    "",
			user:           user,
			expectedStatus: 200,
			expectedCount:  5,
		},
		{
			name:           "List with pagination",
			queryParams:    "?page=1&size=3",
			user:           user,
			expectedStatus: 200,
			expectedCount:  3,
		},
		{
			name:           "Filter by status",
			queryParams:    "?status=unread",
			user:           user,
			expectedStatus: 200,
			expectedCount:  5,
		},
		{
			name:           "Filter by caller number",
			queryParams:    "?caller_number=12345678900",
			user:           user,
			expectedStatus: 200,
			expectedCount:  1,
		},
		{
			name:           "No user",
			queryParams:    "",
			user:           nil,
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", "/voicemails"+tt.queryParams, nil)
			if tt.user != nil {
				c.Set("user", tt.user)
			}

			handler.ListVoicemails(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data := response["data"].(map[string]interface{})
				voicemails := data["list"].([]interface{})
				assert.Equal(t, tt.expectedCount, len(voicemails))
			}
		})
	}
}

func TestVoicemailHandler_GetVoicemail(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voicemail
	voicemail := &models.Voicemail{
		UserID:       user.ID,
		CallerNumber: "1234567890",
		AudioPath:    "/path/to/audio.wav",
		Duration:     30,
		Status:       models.VoicemailStatusNew,
		IsRead:       false,
	}
	require.NoError(t, db.Create(voicemail).Error)

	tests := []struct {
		name           string
		voicemailID    string
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid voicemail ID",
			voicemailID:    fmt.Sprintf("%d", voicemail.ID),
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Invalid voicemail ID",
			voicemailID:    "999",
			user:           user,
			expectedStatus: 400,
			expectedError:  "留言不存在",
		},
		{
			name:           "Non-numeric ID",
			voicemailID:    "invalid",
			user:           user,
			expectedStatus: 400,
			expectedError:  "无效的留言ID",
		},
		{
			name:           "No user",
			voicemailID:    fmt.Sprintf("%d", voicemail.ID),
			user:           nil,
			expectedStatus: 400,
			expectedError:  "未授权",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", fmt.Sprintf("/voicemails/%s", tt.voicemailID), nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.voicemailID}}
			if tt.user != nil {
				c.Set("user", tt.user)
			}

			handler.GetVoicemail(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}

			if w.Code == 200 {
				// Verify voicemail was marked as read
				var updatedVoicemail models.Voicemail
				err := db.First(&updatedVoicemail, voicemail.ID).Error
				require.NoError(t, err)
				assert.True(t, updatedVoicemail.IsRead)
			}
		})
	}
}

func TestVoicemailHandler_MarkAsRead(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voicemail
	voicemail := &models.Voicemail{
		UserID:       user.ID,
		CallerNumber: "1234567890",
		AudioPath:    "/path/to/audio.wav",
		Duration:     30,
		Status:       models.VoicemailStatusNew,
		IsRead:       false,
	}
	require.NoError(t, db.Create(voicemail).Error)

	tests := []struct {
		name           string
		voicemailID    string
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid mark as read",
			voicemailID:    fmt.Sprintf("%d", voicemail.ID),
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Invalid voicemail ID",
			voicemailID:    "999",
			user:           user,
			expectedStatus: 400,
			expectedError:  "留言不存在",
		},
		{
			name:           "No user",
			voicemailID:    fmt.Sprintf("%d", voicemail.ID),
			user:           nil,
			expectedStatus: 400,
			expectedError:  "未授权",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("POST", fmt.Sprintf("/voicemails/%s/read", tt.voicemailID), nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.voicemailID}}
			if tt.user != nil {
				c.Set("user", tt.user)
			}

			handler.MarkAsRead(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}
		})
	}
}

func TestVoicemailHandler_DeleteVoicemail(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voicemail
	voicemail := &models.Voicemail{
		UserID:       user.ID,
		CallerNumber: "1234567890",
		AudioPath:    "/path/to/audio.wav",
		Duration:     30,
		Status:       models.VoicemailStatusNew,
		IsRead:       false,
	}
	require.NoError(t, db.Create(voicemail).Error)

	tests := []struct {
		name           string
		voicemailID    string
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid deletion",
			voicemailID:    fmt.Sprintf("%d", voicemail.ID),
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Invalid voicemail ID",
			voicemailID:    "999",
			user:           user,
			expectedStatus: 400,
			expectedError:  "留言不存在",
		},
		{
			name:           "No user",
			voicemailID:    fmt.Sprintf("%d", voicemail.ID),
			user:           nil,
			expectedStatus: 400,
			expectedError:  "未授权",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("DELETE", fmt.Sprintf("/voicemails/%s", tt.voicemailID), nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.voicemailID}}
			if tt.user != nil {
				c.Set("user", tt.user)
			}

			handler.DeleteVoicemail(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}

			if w.Code == 200 {
				// Verify voicemail was deleted
				var deletedVoicemail models.Voicemail
				err := db.First(&deletedVoicemail, voicemail.ID).Error
				assert.Error(t, err)
				assert.Equal(t, gorm.ErrRecordNotFound, err)
			}
		})
	}
}

func TestVoicemailHandler_GetUnreadCount(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voicemails (3 unread, 2 read)
	for i := 0; i < 5; i++ {
		voicemail := &models.Voicemail{
			UserID:       user.ID,
			CallerNumber: fmt.Sprintf("1234567890%d", i),
			AudioPath:    fmt.Sprintf("/path/to/audio%d.wav", i),
			Duration:     30,
			Status:       models.VoicemailStatusNew,
			IsRead:       i >= 3, // First 3 are unread
		}
		if i >= 3 {
			voicemail.Status = models.VoicemailStatusRead
		}
		require.NoError(t, db.Create(voicemail).Error)
	}

	tests := []struct {
		name           string
		user           *models.User
		expectedStatus int
		expectedCount  int64
	}{
		{
			name:           "Get unread count",
			user:           user,
			expectedStatus: 200,
			expectedCount:  3,
		},
		{
			name:           "No user",
			user:           nil,
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", "/voicemails/unread/count", nil)
			if tt.user != nil {
				c.Set("user", tt.user)
			}

			handler.GetUnreadCount(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data := response["data"].(map[string]interface{})
				count := int64(data["count"].(float64))
				assert.Equal(t, tt.expectedCount, count)
			}
		})
	}
}

func TestVoicemailHandler_UpdateVoicemail(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voicemail
	voicemail := &models.Voicemail{
		UserID:       user.ID,
		CallerNumber: "1234567890",
		AudioPath:    "/path/to/audio.wav",
		Duration:     30,
		Status:       models.VoicemailStatusNew,
		IsRead:       false,
		IsImportant:  false,
	}
	require.NoError(t, db.Create(voicemail).Error)

	tests := []struct {
		name           string
		voicemailID    string
		updates        map[string]interface{}
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name:        "Valid update - mark as important",
			voicemailID: fmt.Sprintf("%d", voicemail.ID),
			updates: map[string]interface{}{
				"isImportant": true,
				"notes":       "Important voicemail",
			},
			user:           user,
			expectedStatus: 200,
		},
		{
			name:        "Valid update - change status",
			voicemailID: fmt.Sprintf("%d", voicemail.ID),
			updates: map[string]interface{}{
				"status": "archived",
			},
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Invalid voicemail ID",
			voicemailID:    "999",
			updates:        map[string]interface{}{"isImportant": true},
			user:           user,
			expectedStatus: 400,
			expectedError:  "留言不存在",
		},
		{
			name:           "No user",
			voicemailID:    fmt.Sprintf("%d", voicemail.ID),
			updates:        map[string]interface{}{"isImportant": true},
			user:           nil,
			expectedStatus: 400,
			expectedError:  "未授权",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			jsonData, _ := json.Marshal(tt.updates)
			c.Request = httptest.NewRequest("PUT", fmt.Sprintf("/voicemails/%s", tt.voicemailID), bytes.NewBuffer(jsonData))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = []gin.Param{{Key: "id", Value: tt.voicemailID}}
			if tt.user != nil {
				c.Set("user", tt.user)
			}

			handler.UpdateVoicemail(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}
		})
	}
}

func TestVoicemailHandler_TranscribeVoicemail(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voicemail
	voicemail := &models.Voicemail{
		UserID:       user.ID,
		CallerNumber: "1234567890",
		AudioPath:    "/path/to/audio.wav",
		Duration:     30,
		Status:       models.VoicemailStatusNew,
	}
	require.NoError(t, db.Create(voicemail).Error)

	// Create already transcribed voicemail
	transcribedVoicemail := &models.Voicemail{
		UserID:           user.ID,
		CallerNumber:     "0987654321",
		AudioPath:        "/path/to/audio2.wav",
		Duration:         45,
		Status:           models.VoicemailStatusRead,
		TranscribeStatus: "completed",
		TranscribedText:  "Hello, this is a test message",
	}
	require.NoError(t, db.Create(transcribedVoicemail).Error)

	tests := []struct {
		name           string
		voicemailID    string
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid transcription request",
			voicemailID:    fmt.Sprintf("%d", voicemail.ID),
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Already transcribed voicemail",
			voicemailID:    fmt.Sprintf("%d", transcribedVoicemail.ID),
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Invalid voicemail ID",
			voicemailID:    "999",
			user:           user,
			expectedStatus: 400,
			expectedError:  "留言不存在",
		},
		{
			name:           "No user",
			voicemailID:    fmt.Sprintf("%d", voicemail.ID),
			user:           nil,
			expectedStatus: 400,
			expectedError:  "未授权",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("POST", fmt.Sprintf("/voicemails/%s/transcribe", tt.voicemailID), nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.voicemailID}}
			if tt.user != nil {
				c.Set("user", tt.user)
			}

			handler.TranscribeVoicemail(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}
		})
	}
}

func TestVoicemailHandler_GenerateSummary(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create transcribed voicemail
	voicemail := &models.Voicemail{
		UserID:           user.ID,
		CallerNumber:     "1234567890",
		AudioPath:        "/path/to/audio.wav",
		Duration:         30,
		Status:           models.VoicemailStatusRead,
		TranscribedText:  "Hello, this is a test message about scheduling a meeting",
		TranscribeStatus: "completed",
	}
	require.NoError(t, db.Create(voicemail).Error)

	// Create voicemail with summary
	voicemailWithSummary := &models.Voicemail{
		UserID:           user.ID,
		CallerNumber:     "0987654321",
		AudioPath:        "/path/to/audio2.wav",
		Duration:         45,
		Status:           models.VoicemailStatusRead,
		TranscribedText:  "This is another test message",
		TranscribeStatus: "completed",
		Summary:          "Test message summary",
	}
	require.NoError(t, db.Create(voicemailWithSummary).Error)

	// Create non-transcribed voicemail
	nonTranscribedVoicemail := &models.Voicemail{
		UserID:       user.ID,
		CallerNumber: "5555555555",
		AudioPath:    "/path/to/audio3.wav",
		Duration:     20,
		Status:       models.VoicemailStatusNew,
	}
	require.NoError(t, db.Create(nonTranscribedVoicemail).Error)

	tests := []struct {
		name           string
		voicemailID    string
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid summary generation",
			voicemailID:    fmt.Sprintf("%d", voicemail.ID),
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Already has summary",
			voicemailID:    fmt.Sprintf("%d", voicemailWithSummary.ID),
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Not transcribed yet",
			voicemailID:    fmt.Sprintf("%d", nonTranscribedVoicemail.ID),
			user:           user,
			expectedStatus: 400,
			expectedError:  "请先转录留言",
		},
		{
			name:           "Invalid voicemail ID",
			voicemailID:    "999",
			user:           user,
			expectedStatus: 400,
			expectedError:  "留言不存在",
		},
		{
			name:           "No user",
			voicemailID:    fmt.Sprintf("%d", voicemail.ID),
			user:           nil,
			expectedStatus: 400,
			expectedError:  "未授权",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("POST", fmt.Sprintf("/voicemails/%s/summary", tt.voicemailID), nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.voicemailID}}
			if tt.user != nil {
				c.Set("user", tt.user)
			}

			handler.GenerateSummary(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}
		})
	}
}

func TestVoicemailHandler_BatchProcessVoicemails(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create another user
	otherUser := &models.User{
		Email:       "other@example.com",
		DisplayName: "otheruser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(otherUser).Error)

	// Create test voicemails
	var voicemailIDs []uint
	for i := 0; i < 3; i++ {
		voicemail := &models.Voicemail{
			UserID:       user.ID,
			CallerNumber: fmt.Sprintf("1234567890%d", i),
			AudioPath:    fmt.Sprintf("/path/to/audio%d.wav", i),
			Duration:     30,
			Status:       models.VoicemailStatusNew,
		}
		require.NoError(t, db.Create(voicemail).Error)
		voicemailIDs = append(voicemailIDs, voicemail.ID)
	}

	// Create voicemail belonging to other user
	otherVoicemail := &models.Voicemail{
		UserID:       otherUser.ID,
		CallerNumber: "9999999999",
		AudioPath:    "/path/to/other.wav",
		Duration:     30,
		Status:       models.VoicemailStatusNew,
	}
	require.NoError(t, db.Create(otherVoicemail).Error)

	tests := []struct {
		name           string
		request        map[string]interface{}
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Valid batch processing",
			request: map[string]interface{}{
				"voicemailIds": voicemailIDs,
			},
			user:           user,
			expectedStatus: 200,
		},
		{
			name: "Empty voicemail IDs",
			request: map[string]interface{}{
				"voicemailIds": []uint{},
			},
			user:           user,
			expectedStatus: 400,
			expectedError:  "请选择要处理的留言",
		},
		{
			name: "Include other user's voicemail",
			request: map[string]interface{}{
				"voicemailIds": []uint{voicemailIDs[0], otherVoicemail.ID},
			},
			user:           user,
			expectedStatus: 400,
			expectedError:  "无权操作部分留言",
		},
		{
			name: "Invalid request format",
			request: map[string]interface{}{
				"invalid": "data",
			},
			user:           user,
			expectedStatus: 400,
			expectedError:  "请求参数错误",
		},
		{
			name: "No user",
			request: map[string]interface{}{
				"voicemailIds": voicemailIDs,
			},
			user:           nil,
			expectedStatus: 400,
			expectedError:  "未授权",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			jsonData, _ := json.Marshal(tt.request)
			c.Request = httptest.NewRequest("POST", "/voicemails/batch-process", bytes.NewBuffer(jsonData))
			c.Request.Header.Set("Content-Type", "application/json")
			if tt.user != nil {
				c.Set("user", tt.user)
			}

			handler.BatchProcessVoicemails(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}
		})
	}
}

func TestVoicemailHandler_GetVoicemailStats(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voicemails with different properties
	today := time.Now().Truncate(24 * time.Hour)

	voicemails := []models.Voicemail{
		{UserID: user.ID, CallerNumber: "1111111111", Status: models.VoicemailStatusNew, IsRead: false, IsImportant: false, CreatedAt: today.Add(time.Hour)},
		{UserID: user.ID, CallerNumber: "2222222222", Status: models.VoicemailStatusNew, IsRead: false, IsImportant: true, CreatedAt: today.Add(2 * time.Hour)},
		{UserID: user.ID, CallerNumber: "3333333333", Status: models.VoicemailStatusRead, IsRead: true, IsImportant: false, CreatedAt: today.Add(-24 * time.Hour)},
		{UserID: user.ID, CallerNumber: "4444444444", Status: models.VoicemailStatusRead, IsRead: true, IsImportant: true, CreatedAt: today.Add(3 * time.Hour)},
		{UserID: user.ID, CallerNumber: "5555555555", Status: models.VoicemailStatusNew, IsRead: false, IsImportant: false, CreatedAt: today.Add(-48 * time.Hour)},
	}

	for _, vm := range voicemails {
		require.NoError(t, db.Create(&vm).Error)
	}

	tests := []struct {
		name           string
		user           *models.User
		expectedStatus int
		expectedStats  map[string]int64
	}{
		{
			name:           "Get voicemail statistics",
			user:           user,
			expectedStatus: 200,
			expectedStats: map[string]int64{
				"total":     5,
				"unread":    3,
				"important": 2,
				"today":     3,
			},
		},
		{
			name:           "No user",
			user:           nil,
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", "/voicemails/stats", nil)
			if tt.user != nil {
				c.Set("user", tt.user)
			}

			handler.GetVoicemailStats(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data := response["data"].(map[string]interface{})
				for key, expectedValue := range tt.expectedStats {
					actualValue := int64(data[key].(float64))
					assert.Equal(t, expectedValue, actualValue, "Mismatch for %s", key)
				}
			}
		})
	}
}

// Benchmark tests
func BenchmarkVoicemailHandler_ListVoicemails(b *testing.B) {
	db := setupTestDB(&testing.T{})

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	db.Create(user)

	// Create test voicemails
	for i := 0; i < 100; i++ {
		voicemail := &models.Voicemail{
			UserID:       user.ID,
			CallerNumber: fmt.Sprintf("123456789%02d", i),
			AudioPath:    fmt.Sprintf("/path/to/audio%d.wav", i),
			Duration:     30,
			Status:       models.VoicemailStatusNew,
		}
		db.Create(voicemail)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", "/voicemails", nil)
		c.Set("user", user)

		handler.ListVoicemails(c)
	}
}

// Test concurrent access
func TestVoicemailHandler_ConcurrentAccess(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voicemail
	voicemail := &models.Voicemail{
		UserID:       user.ID,
		CallerNumber: "1234567890",
		AudioPath:    "/path/to/audio.wav",
		Duration:     30,
		Status:       models.VoicemailStatusNew,
		IsRead:       false,
	}
	require.NoError(t, db.Create(voicemail).Error)

	const numGoroutines = 10
	results := make(chan int, numGoroutines)

	// Test concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", fmt.Sprintf("/voicemails/%d", voicemail.ID), nil)
			c.Params = []gin.Param{{Key: "id", Value: fmt.Sprintf("%d", voicemail.ID)}}
			c.Set("user", user)

			handler.GetVoicemail(c)
			results <- w.Code
		}()
	}

	// Collect results
	successCount := 0
	for i := 0; i < numGoroutines; i++ {
		code := <-results
		if code == 200 {
			successCount++
		}
	}

	assert.Equal(t, numGoroutines, successCount)
}

// Test edge cases
func TestVoicemailHandler_EdgeCases(t *testing.T) {
	db := setupTestDB(t)

	handler := NewVoicemailHandler(db)
	gin.SetMode(gin.TestMode)

	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
	}
	db.Create(user)

	t.Run("Very large voicemail list", func(t *testing.T) {
		// Create many voicemails
		for i := 0; i < 1000; i++ {
			voicemail := &models.Voicemail{
				UserID:       user.ID,
				CallerNumber: fmt.Sprintf("123456789%03d", i),
				AudioPath:    fmt.Sprintf("/path/to/audio%d.wav", i),
				Duration:     30,
				Status:       models.VoicemailStatusNew,
			}
			db.Create(voicemail)
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", "/voicemails?size=100", nil)
		c.Set("user", user)

		handler.ListVoicemails(c)

		assert.Equal(t, 200, w.Code)
	})

	t.Run("Invalid pagination parameters", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", "/voicemails?page=-1&size=0", nil)
		c.Set("user", user)

		handler.ListVoicemails(c)

		// Should handle invalid parameters gracefully
		assert.Equal(t, 200, w.Code)
	})
}
