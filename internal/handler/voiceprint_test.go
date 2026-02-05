package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlers_GetVoiceprints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test data
	voiceprint1 := models.Voiceprint{
		SpeakerID:   "speaker_1",
		AssistantID: "assistant_1",
		SpeakerName: "Test Speaker 1",
	}
	voiceprint2 := models.Voiceprint{
		SpeakerID:   "speaker_2",
		AssistantID: "assistant_1",
		SpeakerName: "Test Speaker 2",
	}
	require.NoError(t, db.Create(&voiceprint1).Error)
	require.NoError(t, db.Create(&voiceprint2).Error)

	tests := []struct {
		name          string
		assistantID   string
		expectedCode  int
		expectedCount int
	}{
		{
			name:          "Valid assistant ID",
			assistantID:   "assistant_1",
			expectedCode:  200,
			expectedCount: 2,
		},
		{
			name:         "Missing assistant ID",
			assistantID:  "",
			expectedCode: 500,
		},
		{
			name:          "Non-existent assistant",
			assistantID:   "non_existent",
			expectedCode:  200,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			req := httptest.NewRequest("GET", "/voiceprints?assistant_id="+tt.assistantID, nil)
			c.Request = req

			h.GetVoiceprints(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int `json:"code"`
					Data struct {
						Total       int                         `json:"total"`
						Voiceprints []models.VoiceprintResponse `json:"voiceprints"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.expectedCount, response.Data.Total)
				assert.Len(t, response.Data.Voiceprints, tt.expectedCount)
			}
		})
	}
}
func TestHandlers_CreateVoiceprint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	tests := []struct {
		name         string
		request      models.VoiceprintCreateRequest
		expectedCode int
		setupFunc    func()
	}{
		{
			name: "Valid request",
			request: models.VoiceprintCreateRequest{
				SpeakerID:   "speaker_1",
				AssistantID: "assistant_1",
				SpeakerName: "Test Speaker",
			},
			expectedCode: 200,
		},
		{
			name: "Duplicate voiceprint",
			request: models.VoiceprintCreateRequest{
				SpeakerID:   "speaker_duplicate",
				AssistantID: "assistant_1",
				SpeakerName: "Duplicate Speaker",
			},
			expectedCode: 500,
			setupFunc: func() {
				existing := models.Voiceprint{
					SpeakerID:   "speaker_duplicate",
					AssistantID: "assistant_1",
					SpeakerName: "Existing Speaker",
				}
				db.Create(&existing)
			},
		},
		{
			name:         "Invalid JSON",
			request:      models.VoiceprintCreateRequest{},
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFunc != nil {
				tt.setupFunc()
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/voiceprints", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.CreateVoiceprint(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                       `json:"code"`
					Data models.VoiceprintResponse `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.request.SpeakerID, response.Data.SpeakerID)
				assert.Equal(t, tt.request.SpeakerName, response.Data.SpeakerName)
			}
		})
	}
}

func TestHandlers_RegisterVoiceprint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Mock voiceprint API
	originalURL := utils.GetEnv("VOICEPRINT_SERVICE_URL")
	defer func() {
		if originalURL != "" {
			// Restore original URL if it existed
		}
	}()

	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/voiceprint/register" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success": true}`))
		}
	}))
	defer mockServer.Close()

	// Set mock URL
	os.Setenv("VOICEPRINT_SERVICE_URL", mockServer.URL)

	tests := []struct {
		name         string
		assistantID  string
		speakerName  string
		audioFile    bool
		expectedCode int
		setupFunc    func()
	}{
		{
			name:         "Valid registration",
			assistantID:  "assistant_1",
			speakerName:  "Test Speaker",
			audioFile:    true,
			expectedCode: 200,
		},
		{
			name:         "Missing assistant ID",
			assistantID:  "",
			speakerName:  "Test Speaker",
			audioFile:    true,
			expectedCode: 500,
		},
		{
			name:         "Missing speaker name",
			assistantID:  "assistant_1",
			speakerName:  "",
			audioFile:    true,
			expectedCode: 500,
		},
		{
			name:         "Missing audio file",
			assistantID:  "assistant_1",
			speakerName:  "Test Speaker",
			audioFile:    false,
			expectedCode: 500,
		},
		{
			name:         "Duplicate speaker name",
			assistantID:  "assistant_1",
			speakerName:  "Duplicate Speaker",
			audioFile:    true,
			expectedCode: 500,
			setupFunc: func() {
				existing := models.Voiceprint{
					SpeakerID:   "existing_speaker",
					AssistantID: "assistant_1",
					SpeakerName: "Duplicate Speaker",
				}
				db.Create(&existing)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFunc != nil {
				tt.setupFunc()
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Create multipart form
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)

			if tt.assistantID != "" {
				writer.WriteField("assistant_id", tt.assistantID)
			}
			if tt.speakerName != "" {
				writer.WriteField("speaker_name", tt.speakerName)
			}

			if tt.audioFile {
				part, _ := writer.CreateFormFile("audio_file", "test.wav")
				part.Write([]byte("fake audio data"))
			}

			writer.Close()

			req := httptest.NewRequest("POST", "/voiceprints/register", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			c.Request = req

			h.RegisterVoiceprint(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}
func TestHandlers_UpdateVoiceprint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test voiceprint
	voiceprint := models.Voiceprint{
		SpeakerID:   "speaker_1",
		AssistantID: "assistant_1",
		SpeakerName: "Original Name",
	}
	require.NoError(t, db.Create(&voiceprint).Error)

	tests := []struct {
		name         string
		voiceprintID string
		request      models.VoiceprintUpdateRequest
		expectedCode int
	}{
		{
			name:         "Valid update",
			voiceprintID: strconv.Itoa(int(voiceprint.ID)),
			request: models.VoiceprintUpdateRequest{
				SpeakerName: "Updated Name",
			},
			expectedCode: 200,
		},
		{
			name:         "Invalid ID",
			voiceprintID: "invalid",
			request: models.VoiceprintUpdateRequest{
				SpeakerName: "Updated Name",
			},
			expectedCode: 500,
		},
		{
			name:         "Non-existent voiceprint",
			voiceprintID: "99999",
			request: models.VoiceprintUpdateRequest{
				SpeakerName: "Updated Name",
			},
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "id", Value: tt.voiceprintID}}

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("PUT", "/voiceprints/"+tt.voiceprintID, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.UpdateVoiceprint(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                       `json:"code"`
					Data models.VoiceprintResponse `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.request.SpeakerName, response.Data.SpeakerName)
			}
		})
	}
}

func TestHandlers_DeleteVoiceprint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Mock voiceprint API
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/voiceprint/") {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer mockServer.Close()

	os.Setenv("VOICEPRINT_SERVICE_URL", mockServer.URL)

	// Create test voiceprint
	voiceprint := models.Voiceprint{
		SpeakerID:   "speaker_1",
		AssistantID: "assistant_1",
		SpeakerName: "Test Speaker",
	}
	require.NoError(t, db.Create(&voiceprint).Error)

	tests := []struct {
		name         string
		voiceprintID string
		expectedCode int
	}{
		{
			name:         "Valid deletion",
			voiceprintID: strconv.Itoa(int(voiceprint.ID)),
			expectedCode: 200,
		},
		{
			name:         "Invalid ID",
			voiceprintID: "invalid",
			expectedCode: 500,
		},
		{
			name:         "Non-existent voiceprint",
			voiceprintID: "99999",
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "id", Value: tt.voiceprintID}}

			req := httptest.NewRequest("DELETE", "/voiceprints/"+tt.voiceprintID, nil)
			c.Request = req

			h.DeleteVoiceprint(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

func TestHandlers_VerifyVoiceprint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Mock voiceprint API
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/voiceprint/identify" {
			w.WriteHeader(http.StatusOK)
			response := map[string]interface{}{
				"speaker_id": "speaker_1",
				"score":      0.85,
			}
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	os.Setenv("VOICEPRINT_SERVICE_URL", mockServer.URL)

	// Create test voiceprints
	voiceprint1 := models.Voiceprint{
		SpeakerID:   "speaker_1",
		AssistantID: "assistant_1",
		SpeakerName: "Speaker 1",
	}
	voiceprint2 := models.Voiceprint{
		SpeakerID:   "speaker_2",
		AssistantID: "assistant_1",
		SpeakerName: "Speaker 2",
	}
	require.NoError(t, db.Create(&voiceprint1).Error)
	require.NoError(t, db.Create(&voiceprint2).Error)

	tests := []struct {
		name         string
		assistantID  string
		speakerID    string
		audioFile    bool
		expectedCode int
	}{
		{
			name:         "Valid verification",
			assistantID:  "assistant_1",
			speakerID:    "speaker_1",
			audioFile:    true,
			expectedCode: 200,
		},
		{
			name:         "Missing assistant ID",
			assistantID:  "",
			speakerID:    "speaker_1",
			audioFile:    true,
			expectedCode: 500,
		},
		{
			name:         "Missing speaker ID",
			assistantID:  "assistant_1",
			speakerID:    "",
			audioFile:    true,
			expectedCode: 500,
		},
		{
			name:         "Missing audio file",
			assistantID:  "assistant_1",
			speakerID:    "speaker_1",
			audioFile:    false,
			expectedCode: 500,
		},
		{
			name:         "Non-existent speaker",
			assistantID:  "assistant_1",
			speakerID:    "non_existent",
			audioFile:    true,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Create multipart form
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)

			if tt.assistantID != "" {
				writer.WriteField("assistant_id", tt.assistantID)
			}
			if tt.speakerID != "" {
				writer.WriteField("speaker_id", tt.speakerID)
			}

			if tt.audioFile {
				part, _ := writer.CreateFormFile("audio_file", "test.wav")
				part.Write([]byte("fake audio data"))
			}

			writer.Close()

			req := httptest.NewRequest("POST", "/voiceprints/verify", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			c.Request = req

			h.VerifyVoiceprint(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                             `json:"code"`
					Data models.VoiceprintVerifyResponse `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.speakerID, response.Data.TargetSpeakerID)
				assert.True(t, response.Data.Score > 0)
			}
		})
	}
}
func TestHandlers_IdentifyVoiceprint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Mock voiceprint API
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/voiceprint/identify" {
			w.WriteHeader(http.StatusOK)
			response := map[string]interface{}{
				"speaker_id": "speaker_1",
				"score":      0.75,
			}
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	os.Setenv("VOICEPRINT_SERVICE_URL", mockServer.URL)

	// Create test voiceprints
	voiceprint1 := models.Voiceprint{
		SpeakerID:   "speaker_1",
		AssistantID: "assistant_1",
		SpeakerName: "Speaker 1",
	}
	require.NoError(t, db.Create(&voiceprint1).Error)

	tests := []struct {
		name         string
		assistantID  string
		audioFile    bool
		expectedCode int
	}{
		{
			name:         "Valid identification",
			assistantID:  "assistant_1",
			audioFile:    true,
			expectedCode: 200,
		},
		{
			name:         "Missing assistant ID",
			assistantID:  "",
			audioFile:    true,
			expectedCode: 500,
		},
		{
			name:         "Missing audio file",
			assistantID:  "assistant_1",
			audioFile:    false,
			expectedCode: 500,
		},
		{
			name:         "No voiceprints for assistant",
			assistantID:  "assistant_empty",
			audioFile:    true,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Create multipart form
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)

			if tt.assistantID != "" {
				writer.WriteField("assistant_id", tt.assistantID)
			}

			if tt.audioFile {
				part, _ := writer.CreateFormFile("audio_file", "test.wav")
				part.Write([]byte("fake audio data"))
			}

			writer.Close()

			req := httptest.NewRequest("POST", "/voiceprints/identify", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			c.Request = req

			h.IdentifyVoiceprint(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                               `json:"code"`
					Data models.VoiceprintIdentifyResponse `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotEmpty(t, response.Data.SpeakerID)
				assert.True(t, response.Data.Score > 0)
			}
		})
	}
}

// Test API call functions
func TestHandlers_callVoiceprintRegisterAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	tests := []struct {
		name         string
		mockResponse int
		expectError  bool
	}{
		{
			name:         "Successful API call",
			mockResponse: http.StatusOK,
			expectError:  false,
		},
		{
			name:         "API error",
			mockResponse: http.StatusInternalServerError,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.mockResponse)
				if tt.mockResponse != http.StatusOK {
					w.Write([]byte("API Error"))
				}
			}))
			defer mockServer.Close()

			os.Setenv("VOICEPRINT_SERVICE_URL", mockServer.URL)

			err := h.callVoiceprintRegisterAPI("speaker_1", "assistant_1", []byte("audio data"))

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandlers_callVoiceprintIdentifyAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	tests := []struct {
		name            string
		mockResponse    int
		mockData        map[string]interface{}
		expectError     bool
		expectedSpeaker string
		expectedScore   float64
	}{
		{
			name:         "Successful identification",
			mockResponse: http.StatusOK,
			mockData: map[string]interface{}{
				"speaker_id": "speaker_1",
				"score":      0.85,
			},
			expectError:     false,
			expectedSpeaker: "speaker_1",
			expectedScore:   0.85,
		},
		{
			name:         "API error",
			mockResponse: http.StatusInternalServerError,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.mockResponse)
				if tt.mockResponse == http.StatusOK && tt.mockData != nil {
					json.NewEncoder(w).Encode(tt.mockData)
				} else if tt.mockResponse != http.StatusOK {
					w.Write([]byte("API Error"))
				}
			}))
			defer mockServer.Close()

			os.Setenv("VOICEPRINT_SERVICE_URL", mockServer.URL)

			speakerID, score, err := h.callVoiceprintIdentifyAPI(
				[]string{"speaker_1", "speaker_2"},
				"assistant_1",
				[]byte("audio data"),
			)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedSpeaker, speakerID)
				assert.Equal(t, tt.expectedScore, score)
			}
		})
	}
}

func TestHandlers_callVoiceprintDeleteAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	tests := []struct {
		name         string
		mockResponse int
		expectError  bool
	}{
		{
			name:         "Successful deletion",
			mockResponse: http.StatusOK,
			expectError:  false,
		},
		{
			name:         "Not found (acceptable)",
			mockResponse: http.StatusNotFound,
			expectError:  false,
		},
		{
			name:         "API error",
			mockResponse: http.StatusInternalServerError,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.mockResponse)
				if tt.mockResponse != http.StatusOK && tt.mockResponse != http.StatusNotFound {
					w.Write([]byte("API Error"))
				}
			}))
			defer mockServer.Close()

			os.Setenv("VOICEPRINT_SERVICE_URL", mockServer.URL)

			err := h.callVoiceprintDeleteAPI("speaker_1", "assistant_1")

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Benchmark tests
func BenchmarkHandlers_GetVoiceprints(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test data
	for i := 0; i < 100; i++ {
		voiceprint := models.Voiceprint{
			SpeakerID:   fmt.Sprintf("speaker_%d", i),
			AssistantID: "assistant_1",
			SpeakerName: fmt.Sprintf("Speaker %d", i),
		}
		db.Create(&voiceprint)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("GET", "/voiceprints?assistant_id=assistant_1", nil)
		c.Request = req

		h.GetVoiceprints(c)
	}
}

func BenchmarkHandlers_CreateVoiceprint(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		request := models.VoiceprintCreateRequest{
			SpeakerID:   fmt.Sprintf("speaker_%d_%d", i, time.Now().UnixNano()),
			AssistantID: "assistant_1",
			SpeakerName: fmt.Sprintf("Speaker %d", i),
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/voiceprints", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.CreateVoiceprint(c)
	}
}

// Edge case tests
func TestHandlers_VoiceprintEdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	t.Run("Invalid file format in RegisterVoiceprint", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		writer.WriteField("assistant_id", "assistant_1")
		writer.WriteField("speaker_name", "Test Speaker")

		part, _ := writer.CreateFormFile("audio_file", "test.mp3")
		part.Write([]byte("fake audio data"))
		writer.Close()

		req := httptest.NewRequest("POST", "/voiceprints/register", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		c.Request = req

		h.RegisterVoiceprint(c)

		assert.Equal(t, 500, w.Code)
	})

	t.Run("Empty speaker name update", func(t *testing.T) {
		voiceprint := models.Voiceprint{
			SpeakerID:   "speaker_1",
			AssistantID: "assistant_1",
			SpeakerName: "Original Name",
		}
		require.NoError(t, db.Create(&voiceprint).Error)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "id", Value: strconv.Itoa(int(voiceprint.ID))}}

		request := models.VoiceprintUpdateRequest{
			SpeakerName: "",
		}
		body, _ := json.Marshal(request)
		req := httptest.NewRequest("PUT", "/voiceprints/"+strconv.Itoa(int(voiceprint.ID)), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.UpdateVoiceprint(c)

		assert.Equal(t, 200, w.Code)

		// Verify name wasn't changed
		var updated models.Voiceprint
		db.First(&updated, voiceprint.ID)
		assert.Equal(t, "Original Name", updated.SpeakerName)
	})
}
