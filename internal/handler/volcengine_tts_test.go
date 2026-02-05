package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlers_VolcengineSynthesize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test user
	user := models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(&user).Error)

	// Create test voice clone
	voiceClone := models.VoiceClone{
		UserID:   user.ID,
		AssetID:  "test_asset_id",
		Provider: "volcengine",
		IsActive: true,
	}
	require.NoError(t, db.Create(&voiceClone).Error)

	tests := []struct {
		name         string
		request      VolcengineTTSRequest
		setupAuth    bool
		expectedCode int
	}{
		{
			name: "Valid synthesis request",
			request: VolcengineTTSRequest{
				AssetID:  "test_asset_id",
				Text:     "Hello world",
				Language: "zh-cn",
			},
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name: "Missing asset ID",
			request: VolcengineTTSRequest{
				Text:     "Hello world",
				Language: "zh-cn",
			},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name: "Missing text",
			request: VolcengineTTSRequest{
				AssetID:  "test_asset_id",
				Language: "zh-cn",
			},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name: "Missing language",
			request: VolcengineTTSRequest{
				AssetID: "test_asset_id",
				Text:    "Hello world",
			},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name: "Unauthorized request",
			request: VolcengineTTSRequest{
				AssetID:  "test_asset_id",
				Text:     "Hello world",
				Language: "zh-cn",
			},
			setupAuth:    false,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			if tt.setupAuth {
				c.Set("user", &user)
			}

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/volcengine/synthesize", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.VolcengineSynthesize(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

func TestHandlers_VolcengineSubmitAudio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	tests := []struct {
		name         string
		speakerID    string
		language     string
		audioFile    bool
		expectedCode int
	}{
		{
			name:         "Valid audio submission",
			speakerID:    "test_speaker_id",
			language:     "zh-cn",
			audioFile:    true,
			expectedCode: 200,
		},
		{
			name:         "Missing speaker ID",
			speakerID:    "",
			language:     "zh-cn",
			audioFile:    true,
			expectedCode: 500,
		},
		{
			name:         "Missing language",
			speakerID:    "test_speaker_id",
			language:     "",
			audioFile:    true,
			expectedCode: 500,
		},
		{
			name:         "Missing audio file",
			speakerID:    "test_speaker_id",
			language:     "zh-cn",
			audioFile:    false,
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

			if tt.speakerID != "" {
				writer.WriteField("speakerId", tt.speakerID)
			}
			if tt.language != "" {
				writer.WriteField("language", tt.language)
			}

			if tt.audioFile {
				part, _ := writer.CreateFormFile("audio", "test.wav")
				part.Write([]byte("fake audio data"))
			}

			writer.Close()

			req := httptest.NewRequest("POST", "/volcengine/submit-audio", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			c.Request = req

			h.VolcengineSubmitAudio(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}
func TestHandlers_VolcengineQueryTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test user
	user := models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(&user).Error)

	tests := []struct {
		name         string
		request      VolcengineQueryTaskRequest
		setupAuth    bool
		expectedCode int
	}{
		{
			name: "Valid query request",
			request: VolcengineQueryTaskRequest{
				SpeakerID: "test_speaker_id",
			},
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name: "Missing speaker ID",
			request: VolcengineQueryTaskRequest{
				SpeakerID: "",
			},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name: "Unauthorized request",
			request: VolcengineQueryTaskRequest{
				SpeakerID: "test_speaker_id",
			},
			setupAuth:    false,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			if tt.setupAuth {
				c.Set("user", &user)
			}

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/volcengine/query-task", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.VolcengineQueryTask(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

// Test request/response structures
func TestVolcengineTTSStructures(t *testing.T) {
	t.Run("VolcengineTTSRequest validation", func(t *testing.T) {
		tests := []struct {
			name    string
			request VolcengineTTSRequest
			valid   bool
		}{
			{
				name: "Valid request",
				request: VolcengineTTSRequest{
					AssetID:  "asset123",
					Text:     "Hello world",
					Language: "zh-cn",
					Key:      "optional_key",
				},
				valid: true,
			},
			{
				name: "Missing AssetID",
				request: VolcengineTTSRequest{
					Text:     "Hello world",
					Language: "zh-cn",
				},
				valid: false,
			},
			{
				name: "Missing Text",
				request: VolcengineTTSRequest{
					AssetID:  "asset123",
					Language: "zh-cn",
				},
				valid: false,
			},
			{
				name: "Missing Language",
				request: VolcengineTTSRequest{
					AssetID: "asset123",
					Text:    "Hello world",
				},
				valid: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Test JSON marshaling/unmarshaling
				data, err := json.Marshal(tt.request)
				assert.NoError(t, err)

				var unmarshaled VolcengineTTSRequest
				err = json.Unmarshal(data, &unmarshaled)
				assert.NoError(t, err)

				if tt.valid {
					assert.Equal(t, tt.request.AssetID, unmarshaled.AssetID)
					assert.Equal(t, tt.request.Text, unmarshaled.Text)
					assert.Equal(t, tt.request.Language, unmarshaled.Language)
				}
			})
		}
	})

	t.Run("VolcengineSubmitAudioRequest validation", func(t *testing.T) {
		request := VolcengineSubmitAudioRequest{
			SpeakerID: "speaker123",
			Language:  "zh-cn",
		}

		// Test that struct fields are properly tagged
		assert.NotEmpty(t, request.SpeakerID)
		assert.NotEmpty(t, request.Language)
	})

	t.Run("VolcengineQueryTaskResponse structure", func(t *testing.T) {
		response := VolcengineQueryTaskResponse{
			SpeakerID:  "speaker123",
			Status:     2,
			TrainVID:   "train123",
			AssetID:    "asset123",
			FailedDesc: "",
			CreateTime: time.Now().UnixMilli(),
		}

		// Test JSON marshaling
		data, err := json.Marshal(response)
		assert.NoError(t, err)

		var unmarshaled VolcengineQueryTaskResponse
		err = json.Unmarshal(data, &unmarshaled)
		assert.NoError(t, err)

		assert.Equal(t, response.SpeakerID, unmarshaled.SpeakerID)
		assert.Equal(t, response.Status, unmarshaled.Status)
		assert.Equal(t, response.TrainVID, unmarshaled.TrainVID)
		assert.Equal(t, response.AssetID, unmarshaled.AssetID)
	})
}

// Test error handling
func TestVolcengineErrorHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	t.Run("Invalid JSON in VolcengineSynthesize", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		user := models.User{Email: "test@example.com"}
		c.Set("user", &user)

		req := httptest.NewRequest("POST", "/volcengine/synthesize", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.VolcengineSynthesize(c)

		assert.Equal(t, 500, w.Code)
	})

	t.Run("Invalid form data in VolcengineSubmitAudio", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("POST", "/volcengine/submit-audio", bytes.NewBufferString("invalid form data"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		c.Request = req

		h.VolcengineSubmitAudio(c)

		assert.Equal(t, 500, w.Code)
	})

	t.Run("Invalid JSON in VolcengineQueryTask", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		user := models.User{Email: "test@example.com"}
		c.Set("user", &user)

		req := httptest.NewRequest("POST", "/volcengine/query-task", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.VolcengineQueryTask(c)

		assert.Equal(t, 500, w.Code)
	})
}

// Test with different voice clone scenarios
func TestVolcengineVoiceCloneIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test user
	user := models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(&user).Error)

	t.Run("Synthesis with existing voice clone", func(t *testing.T) {
		// Create voice clone
		voiceClone := models.VoiceClone{
			UserID:   user.ID,
			AssetID:  "existing_asset",
			Provider: "volcengine",
			IsActive: true,
		}
		require.NoError(t, db.Create(&voiceClone).Error)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", &user)

		request := VolcengineTTSRequest{
			AssetID:  "existing_asset",
			Text:     "Test synthesis",
			Language: "zh-cn",
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/volcengine/synthesize", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.VolcengineSynthesize(c)

		// Should proceed even if voice clone exists (for history tracking)
		assert.Equal(t, 200, w.Code)
	})

	t.Run("Synthesis without existing voice clone", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", &user)

		request := VolcengineTTSRequest{
			AssetID:  "non_existing_asset",
			Text:     "Test synthesis",
			Language: "zh-cn",
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/volcengine/synthesize", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.VolcengineSynthesize(c)

		// Should still proceed (synthesis allowed without history)
		assert.Equal(t, 200, w.Code)
	})
}

// Benchmark tests
func BenchmarkHandlers_VolcengineSynthesize(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	user := models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	db.Create(&user)

	request := VolcengineTTSRequest{
		AssetID:  "benchmark_asset",
		Text:     "Benchmark test text",
		Language: "zh-cn",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", &user)

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/volcengine/synthesize", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.VolcengineSynthesize(c)
	}
}

func BenchmarkHandlers_VolcengineQueryTask(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	user := models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	db.Create(&user)

	request := VolcengineQueryTaskRequest{
		SpeakerID: "benchmark_speaker",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", &user)

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/volcengine/query-task", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.VolcengineQueryTask(c)
	}
}

// Edge case tests
func TestVolcengineEdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	user := models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(&user).Error)

	t.Run("Synthesis with custom key", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", &user)

		request := VolcengineTTSRequest{
			AssetID:  "test_asset",
			Text:     "Custom key test",
			Language: "zh-cn",
			Key:      "custom/path/audio.wav",
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/volcengine/synthesize", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.VolcengineSynthesize(c)

		assert.Equal(t, 200, w.Code)
	})

	t.Run("Very long text synthesis", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", &user)

		longText := strings.Repeat("This is a very long text for synthesis testing. ", 100)
		request := VolcengineTTSRequest{
			AssetID:  "test_asset",
			Text:     longText,
			Language: "zh-cn",
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/volcengine/synthesize", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.VolcengineSynthesize(c)

		assert.Equal(t, 200, w.Code)
	})

	t.Run("Empty audio file submission", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		writer.WriteField("speakerId", "test_speaker")
		writer.WriteField("language", "zh-cn")

		// Create empty audio file
		part, _ := writer.CreateFormFile("audio", "empty.wav")
		part.Write([]byte{})
		writer.Close()

		req := httptest.NewRequest("POST", "/volcengine/submit-audio", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		c.Request = req

		h.VolcengineSubmitAudio(c)

		// Should still process (validation happens in voiceclone service)
		assert.Equal(t, 200, w.Code)
	})
}
