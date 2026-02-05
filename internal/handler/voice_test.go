package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTrainingTask(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	tests := []struct {
		name           string
		request        CreateTrainingTaskRequest
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Valid training task",
			request: CreateTrainingTaskRequest{
				TaskName: "Test Voice",
				Sex:      models.SexMale,
				AgeGroup: models.AgeGroupYouth,
				Language: models.LanguageChinese,
			},
			user:           user,
			expectedStatus: 200,
		},
		{
			name: "Task with default values",
			request: CreateTrainingTaskRequest{
				TaskName: "Default Voice",
			},
			user:           user,
			expectedStatus: 200,
		},
		{
			name: "Empty task name",
			request: CreateTrainingTaskRequest{
				TaskName: "",
			},
			user:           user,
			expectedStatus: 400,
		},
		{
			name:           "No user",
			request:        CreateTrainingTaskRequest{TaskName: "Test"},
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
			c.Request = httptest.NewRequest("POST", "/voice/training/create", bytes.NewBuffer(jsonData))
			c.Request.Header.Set("Content-Type", "application/json")

			if tt.user != nil {
				c.Set("user", tt.user)
			}

			h.CreateTrainingTask(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Equal(t, "创建训练任务成功", response["msg"])

				// Verify task was created in database
				var task models.VoiceTrainingTask
				err = db.Where("user_id = ? AND task_name = ?", user.ID, tt.request.TaskName).First(&task).Error
				require.NoError(t, err)
				assert.Equal(t, tt.request.TaskName, task.TaskName)
			}
		})
	}
}

func TestSubmitAudio(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user and task
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	task := &models.VoiceTrainingTask{
		UserID:   user.ID,
		TaskID:   "test-task-id",
		TaskName: "Test Task",
		Status:   models.TrainingStatusQueued,
		TextID:   5001,
	}
	require.NoError(t, db.Create(task).Error)

	tests := []struct {
		name           string
		taskID         string
		textSegID      string
		audioContent   string
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid audio submission",
			taskID:         task.TaskID,
			textSegID:      "1",
			audioContent:   "fake audio data",
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Invalid task ID",
			taskID:         "invalid-task",
			textSegID:      "1",
			audioContent:   "fake audio data",
			user:           user,
			expectedStatus: 400,
			expectedError:  "训练任务不存在",
		},
		{
			name:           "No user",
			taskID:         task.TaskID,
			textSegID:      "1",
			audioContent:   "fake audio data",
			user:           nil,
			expectedStatus: 400,
			expectedError:  "未授权",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Create multipart form data
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)

			writer.WriteField("taskId", tt.taskID)
			writer.WriteField("textSegId", tt.textSegID)

			if tt.audioContent != "" {
				part, err := writer.CreateFormFile("audio", "test.wav")
				require.NoError(t, err)
				_, err = io.WriteString(part, tt.audioContent)
				require.NoError(t, err)
			}

			writer.Close()

			c.Request = httptest.NewRequest("POST", "/voice/training/submit-audio", &body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())

			if tt.user != nil {
				c.Set("user", tt.user)
			}

			h.SubmitAudio(c)

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

func TestQueryTaskStatus(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user and task
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	task := &models.VoiceTrainingTask{
		UserID:   user.ID,
		TaskID:   "test-task-id",
		TaskName: "Test Task",
		Status:   models.TrainingStatusInProgress,
	}
	require.NoError(t, db.Create(task).Error)

	tests := []struct {
		name           string
		request        QueryTaskStatusRequest
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Valid task query",
			request: QueryTaskStatusRequest{
				TaskID: task.TaskID,
			},
			user:           user,
			expectedStatus: 200,
		},
		{
			name: "Invalid task ID",
			request: QueryTaskStatusRequest{
				TaskID: "invalid-task",
			},
			user:           user,
			expectedStatus: 400,
			expectedError:  "训练任务不存在",
		},
		{
			name: "No user",
			request: QueryTaskStatusRequest{
				TaskID: task.TaskID,
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
			c.Request = httptest.NewRequest("POST", "/voice/training/query", bytes.NewBuffer(jsonData))
			c.Request.Header.Set("Content-Type", "application/json")

			if tt.user != nil {
				c.Set("user", tt.user)
			}

			h.QueryTaskStatus(c)

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

func TestGetUserVoiceClones(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voice clones
	for i := 0; i < 3; i++ {
		clone := &models.VoiceClone{
			UserID:           user.ID,
			Provider:         "xunfei",
			AssetID:          fmt.Sprintf("asset-%d", i),
			VoiceName:        fmt.Sprintf("Voice %d", i),
			VoiceDescription: fmt.Sprintf("Description %d", i),
			IsActive:         true,
		}
		require.NoError(t, db.Create(clone).Error)
	}

	tests := []struct {
		name           string
		provider       string
		user           *models.User
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "Get all voice clones",
			provider:       "",
			user:           user,
			expectedStatus: 200,
			expectedCount:  3,
		},
		{
			name:           "Filter by provider",
			provider:       "xunfei",
			user:           user,
			expectedStatus: 200,
			expectedCount:  3,
		},
		{
			name:           "No user",
			provider:       "",
			user:           nil,
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/voice/clones"
			if tt.provider != "" {
				url += "?provider=" + tt.provider
			}
			c.Request = httptest.NewRequest("GET", url, nil)

			if tt.user != nil {
				c.Set("user", tt.user)
			}

			h.GetUserVoiceClones(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				clones := response["data"].([]interface{})
				assert.Equal(t, tt.expectedCount, len(clones))
			}
		})
	}
}

func TestSynthesizeWithVoice(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voice clone
	clone := &models.VoiceClone{
		UserID:           user.ID,
		Provider:         "xunfei",
		AssetID:          "test-asset-id",
		VoiceName:        "Test Voice",
		VoiceDescription: "Test Description",
		IsActive:         true,
	}
	require.NoError(t, db.Create(clone).Error)

	tests := []struct {
		name           string
		request        SynthesizeRequest
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Valid synthesis request",
			request: SynthesizeRequest{
				VoiceCloneID: clone.ID,
				Text:         "Hello World",
				Language:     models.LanguageChinese,
			},
			user:           user,
			expectedStatus: 200,
		},
		{
			name: "Invalid voice clone ID",
			request: SynthesizeRequest{
				VoiceCloneID: 999,
				Text:         "Hello World",
				Language:     models.LanguageChinese,
			},
			user:           user,
			expectedStatus: 400,
			expectedError:  "音色不存在",
		},
		{
			name: "Empty text",
			request: SynthesizeRequest{
				VoiceCloneID: clone.ID,
				Text:         "",
				Language:     models.LanguageChinese,
			},
			user:           user,
			expectedStatus: 400,
		},
		{
			name: "No user",
			request: SynthesizeRequest{
				VoiceCloneID: clone.ID,
				Text:         "Hello World",
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
			c.Request = httptest.NewRequest("POST", "/voice/synthesize", bytes.NewBuffer(jsonData))
			c.Request.Header.Set("Content-Type", "application/json")

			if tt.user != nil {
				c.Set("user", tt.user)
			}

			h.SynthesizeWithVoice(c)

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

func TestGetSynthesisHistory(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test voice clone
	clone := &models.VoiceClone{
		UserID:   user.ID,
		Provider: "xunfei",
		AssetID:  "test-asset-id",
		IsActive: true,
	}
	require.NoError(t, db.Create(clone).Error)

	// Create test synthesis records
	for i := 0; i < 5; i++ {
		synthesis := &models.VoiceSynthesis{
			UserID:       user.ID,
			VoiceCloneID: clone.ID,
			Text:         fmt.Sprintf("Test text %d", i),
			Language:     models.LanguageChinese,
			AudioURL:     fmt.Sprintf("http://example.com/audio%d.wav", i),
			Status:       "success",
		}
		require.NoError(t, db.Create(synthesis).Error)
	}

	tests := []struct {
		name           string
		limit          string
		provider       string
		user           *models.User
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "Get all history",
			limit:          "",
			provider:       "",
			user:           user,
			expectedStatus: 200,
			expectedCount:  5,
		},
		{
			name:           "Limit results",
			limit:          "3",
			provider:       "",
			user:           user,
			expectedStatus: 200,
			expectedCount:  3,
		},
		{
			name:           "Filter by provider",
			limit:          "",
			provider:       "xunfei",
			user:           user,
			expectedStatus: 200,
			expectedCount:  5,
		},
		{
			name:           "No user",
			limit:          "",
			provider:       "",
			user:           nil,
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/voice/synthesis/history"
			params := []string{}
			if tt.limit != "" {
				params = append(params, "limit="+tt.limit)
			}
			if tt.provider != "" {
				params = append(params, "provider="+tt.provider)
			}
			if len(params) > 0 {
				url += "?" + strings.Join(params, "&")
			}

			c.Request = httptest.NewRequest("GET", url, nil)

			if tt.user != nil {
				c.Set("user", tt.user)
			}

			h.GetSynthesisHistory(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				history := response["data"].([]interface{})
				assert.Equal(t, tt.expectedCount, len(history))
			}
		})
	}
}

func TestGetVoiceOptions(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		provider       string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid provider",
			provider:       "tencent",
			expectedStatus: 200,
		},
		{
			name:           "Missing provider",
			provider:       "",
			expectedStatus: 400,
			expectedError:  "缺少provider参数",
		},
		{
			name:           "Unsupported provider",
			provider:       "unsupported",
			expectedStatus: 400,
			expectedError:  "加载音色列表失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/voice/options"
			if tt.provider != "" {
				url += "?provider=" + tt.provider
			}
			c.Request = httptest.NewRequest("GET", url, nil)

			h.GetVoiceOptions(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "provider")
				assert.Contains(t, data, "voices")
			}
		})
	}
}

func TestGetLanguageOptions(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		provider       string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid provider",
			provider:       "tencent",
			expectedStatus: 200,
		},
		{
			name:           "Missing provider",
			provider:       "",
			expectedStatus: 400,
			expectedError:  "缺少provider参数",
		},
		{
			name:           "Unsupported provider with defaults",
			provider:       "unknown",
			expectedStatus: 200, // Should return default languages
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/voice/language-options"
			if tt.provider != "" {
				url += "?provider=" + tt.provider
			}
			c.Request = httptest.NewRequest("GET", url, nil)

			h.GetLanguageOptions(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "provider")
				assert.Contains(t, data, "languages")
			}
		})
	}
}

func TestOneShotText(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user and credential
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	credential := &models.UserCredential{
		UserID:      user.ID,
		APIKey:      "test-api-key",
		APISecret:   "test-api-secret",
		LLMProvider: "openai",
		LLMApiKey:   "test-llm-key",
	}
	require.NoError(t, db.Create(credential).Error)

	// Create test assistant
	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "You are a helpful assistant",
		ApiKey:       credential.APIKey,
		ApiSecret:    credential.APISecret,
	}
	require.NoError(t, db.Create(assistant).Error)

	tests := []struct {
		name           string
		request        OneShotTextRequest
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Valid request",
			request: OneShotTextRequest{
				APIKey:      credential.APIKey,
				APISecret:   credential.APISecret,
				Text:        "Hello, how are you?",
				AssistantID: int(assistant.ID),
				Language:    models.LanguageChinese,
			},
			expectedStatus: 200,
		},
		{
			name: "Invalid credentials",
			request: OneShotTextRequest{
				APIKey:      "invalid-key",
				APISecret:   "invalid-secret",
				Text:        "Hello",
				AssistantID: int(assistant.ID),
			},
			expectedStatus: 400,
			expectedError:  "凭证不存在",
		},
		{
			name: "Empty text",
			request: OneShotTextRequest{
				APIKey:      credential.APIKey,
				APISecret:   credential.APISecret,
				Text:        "",
				AssistantID: int(assistant.ID),
			},
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			jsonData, _ := json.Marshal(tt.request)
			c.Request = httptest.NewRequest("POST", "/voice/oneshot_text", bytes.NewBuffer(jsonData))
			c.Request.Header.Set("Content-Type", "application/json")

			h.OneShotText(c)

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

func TestSimpleTextChat(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user and credential
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	credential := &models.UserCredential{
		UserID:      user.ID,
		APIKey:      "test-api-key",
		APISecret:   "test-api-secret",
		LLMProvider: "openai",
		LLMApiKey:   "test-llm-key",
	}
	require.NoError(t, db.Create(credential).Error)

	// Create test assistant
	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "You are a helpful assistant",
	}
	require.NoError(t, db.Create(assistant).Error)

	tests := []struct {
		name           string
		request        SimpleTextChatRequest
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Valid chat request",
			request: SimpleTextChatRequest{
				APIKey:      credential.APIKey,
				APISecret:   credential.APISecret,
				Text:        "Hello, how are you?",
				AssistantID: int(assistant.ID),
			},
			expectedStatus: 200,
		},
		{
			name: "Invalid assistant ID",
			request: SimpleTextChatRequest{
				APIKey:      credential.APIKey,
				APISecret:   credential.APISecret,
				Text:        "Hello",
				AssistantID: 999,
			},
			expectedStatus: 400,
			expectedError:  "助手不存在",
		},
		{
			name: "Empty text",
			request: SimpleTextChatRequest{
				APIKey:      credential.APIKey,
				APISecret:   credential.APISecret,
				Text:        "",
				AssistantID: int(assistant.ID),
			},
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			jsonData, _ := json.Marshal(tt.request)
			c.Request = httptest.NewRequest("POST", "/voice/simple_text_chat", bytes.NewBuffer(jsonData))
			c.Request.Header.Set("Content-Type", "application/json")

			h.SimpleTextChat(c)

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

func TestGetAudioStatus(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Set up test cache data
	testRequestID := "test-request-123"
	audioCacheMutex.Lock()
	audioProcessingCache[testRequestID] = AudioProcessResult{
		Status:   "completed",
		Text:     "Test text",
		AudioURL: "http://example.com/audio.wav",
	}
	audioCacheMutex.Unlock()

	tests := []struct {
		name           string
		requestID      string
		expectedStatus int
		expectedResult string
	}{
		{
			name:           "Valid request ID with completed status",
			requestID:      testRequestID,
			expectedStatus: 200,
			expectedResult: "completed",
		},
		{
			name:           "Invalid request ID",
			requestID:      "invalid-request",
			expectedStatus: 200,
			expectedResult: "processing",
		},
		{
			name:           "Empty request ID",
			requestID:      "",
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/voice/audio_status"
			if tt.requestID != "" {
				url += "?requestId=" + tt.requestID
			}
			c.Request = httptest.NewRequest("GET", url, nil)

			h.GetAudioStatus(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data := response["data"].(map[string]interface{})
				assert.Equal(t, tt.expectedResult, data["status"])
			}
		})
	}

	// Clean up cache
	audioCacheMutex.Lock()
	delete(audioProcessingCache, testRequestID)
	audioCacheMutex.Unlock()
}

func TestCleanTextForTTS(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Remove bold markdown",
			input:    "This is **bold** text",
			expected: "This is bold text",
		},
		{
			name:     "Remove italic markdown",
			input:    "This is *italic* text",
			expected: "This is italic text",
		},
		{
			name:     "Remove code markdown",
			input:    "This is `code` text",
			expected: "This is code text",
		},
		{
			name:     "Remove links",
			input:    "Visit [Google](https://google.com) for search",
			expected: "Visit Google for search",
		},
		{
			name:     "Remove headers",
			input:    "# Header\n## Subheader\nContent",
			expected: "Header\nSubheader\nContent",
		},
		{
			name:     "Remove list markers",
			input:    "- Item 1\n* Item 2\n+ Item 3",
			expected: "Item 1\nItem 2\nItem 3",
		},
		{
			name:     "Remove quotes",
			input:    "> This is a quote\nNormal text",
			expected: "This is a quote\nNormal text",
		},
		{
			name:     "Complex markdown",
			input:    "# Title\n\n**Bold** and *italic* with `code` and [link](url)\n\n- List item\n> Quote",
			expected: "Title\n\nBold and italic with code and link\n\nList item\nQuote",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanTextForTTS(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateWAVFile(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}

	tests := []struct {
		name       string
		pcmData    []byte
		sampleRate int
		channels   int
		bitDepth   int
		expectErr  bool
	}{
		{
			name:       "Valid WAV creation",
			pcmData:    []byte{0x00, 0x01, 0x02, 0x03},
			sampleRate: 44100,
			channels:   2,
			bitDepth:   16,
			expectErr:  false,
		},
		{
			name:       "Mono audio",
			pcmData:    []byte{0x00, 0x01},
			sampleRate: 22050,
			channels:   1,
			bitDepth:   16,
			expectErr:  false,
		},
		{
			name:       "Empty PCM data",
			pcmData:    []byte{},
			sampleRate: 44100,
			channels:   2,
			bitDepth:   16,
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wavData, err := h.createWAVFile(tt.pcmData, tt.sampleRate, tt.channels, tt.bitDepth)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, wavData)

				// WAV file should have 44-byte header + PCM data
				expectedSize := 44 + len(tt.pcmData)
				assert.Equal(t, expectedSize, len(wavData))

				// Check WAV header
				assert.Equal(t, "RIFF", string(wavData[0:4]))
				assert.Equal(t, "WAVE", string(wavData[8:12]))
				assert.Equal(t, "fmt ", string(wavData[12:16]))
				assert.Equal(t, "data", string(wavData[36:40]))
			}
		})
	}
}

// Benchmark tests
func BenchmarkCreateTrainingTask(b *testing.B) {
	db := setupTestDB(&testing.T{})

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	db.Create(user)

	request := CreateTrainingTaskRequest{
		TaskName: "Benchmark Task",
		Sex:      models.SexMale,
		AgeGroup: models.AgeGroupYouth,
		Language: models.LanguageChinese,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		request.TaskName = fmt.Sprintf("Benchmark Task %d", i)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		jsonData, _ := json.Marshal(request)
		c.Request = httptest.NewRequest("POST", "/voice/training/create", bytes.NewBuffer(jsonData))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)

		h.CreateTrainingTask(c)
	}
}

func BenchmarkCleanTextForTTS(b *testing.B) {
	text := "# Header\n\n**Bold** and *italic* with `code` and [link](url)\n\n- List item\n> Quote"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cleanTextForTTS(text)
	}
}

// Test edge cases
func TestVoiceHandlerEdgeCases(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	t.Run("Very long text synthesis", func(t *testing.T) {
		user := &models.User{
			Email:       "test@example.com",
			DisplayName: "testuser",
		}
		db.Create(user)

		clone := &models.VoiceClone{
			UserID:   user.ID,
			Provider: "xunfei",
			AssetID:  "test-asset",
			IsActive: true,
		}
		db.Create(clone)

		longText := strings.Repeat("This is a very long text for synthesis. ", 1000)
		request := SynthesizeRequest{
			VoiceCloneID: clone.ID,
			Text:         longText,
			Language:     models.LanguageChinese,
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		jsonData, _ := json.Marshal(request)
		c.Request = httptest.NewRequest("POST", "/voice/synthesize", bytes.NewBuffer(jsonData))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)

		h.SynthesizeWithVoice(c)

		// Should handle long text gracefully
		assert.True(t, w.Code == 200 || w.Code == 400)
	})

	t.Run("Concurrent voice clone access", func(t *testing.T) {
		user := &models.User{
			Email:       "test@example.com",
			DisplayName: "testuser",
		}
		db.Create(user)

		const numGoroutines = 10
		results := make(chan int, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)

				c.Request = httptest.NewRequest("GET", "/voice/clones", nil)
				c.Set("user", user)

				h.GetUserVoiceClones(c)
				results <- w.Code
			}()
		}

		// Collect results
		for i := 0; i < numGoroutines; i++ {
			code := <-results
			assert.Equal(t, 200, code)
		}
	})
}
