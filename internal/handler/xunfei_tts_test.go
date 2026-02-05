package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlers_XunfeiSynthesize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	tests := []struct {
		name         string
		request      XunfeiTTSRequest
		expectedCode int
	}{
		{
			name: "Valid synthesis request",
			request: XunfeiTTSRequest{
				AssetID:  "test_asset_id",
				Text:     "Hello world",
				Language: "zh",
			},
			expectedCode: 200,
		},
		{
			name: "Valid synthesis with custom key",
			request: XunfeiTTSRequest{
				AssetID:  "test_asset_id",
				Text:     "Hello world",
				Language: "zh",
				Key:      "custom/path/audio.wav",
			},
			expectedCode: 200,
		},
		{
			name: "Missing asset ID",
			request: XunfeiTTSRequest{
				Text:     "Hello world",
				Language: "zh",
			},
			expectedCode: 500,
		},
		{
			name: "Missing text",
			request: XunfeiTTSRequest{
				AssetID:  "test_asset_id",
				Language: "zh",
			},
			expectedCode: 500,
		},
		{
			name: "Missing language",
			request: XunfeiTTSRequest{
				AssetID: "test_asset_id",
				Text:    "Hello world",
			},
			expectedCode: 500,
		},
		{
			name: "Empty asset ID",
			request: XunfeiTTSRequest{
				AssetID:  "",
				Text:     "Hello world",
				Language: "zh",
			},
			expectedCode: 500,
		},
		{
			name: "Empty text",
			request: XunfeiTTSRequest{
				AssetID:  "test_asset_id",
				Text:     "",
				Language: "zh",
			},
			expectedCode: 500,
		},
		{
			name: "Empty language",
			request: XunfeiTTSRequest{
				AssetID:  "test_asset_id",
				Text:     "Hello world",
				Language: "",
			},
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/xunfei/synthesize", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.XunfeiSynthesize(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int               `json:"code"`
					Data XunfeiTTSResponse `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotEmpty(t, response.Data.URL)
			}
		})
	}
}

func TestHandlers_XunfeiCreateTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	tests := []struct {
		name         string
		request      XunfeiCreateTaskRequest
		expectedCode int
	}{
		{
			name: "Valid task creation",
			request: XunfeiCreateTaskRequest{
				TaskName: "Test Task",
				Sex:      1,
				AgeGroup: 2,
				Language: "zh",
			},
			expectedCode: 200,
		},
		{
			name: "Task with default values",
			request: XunfeiCreateTaskRequest{
				TaskName: "Default Task",
				// Sex, AgeGroup, Language will use defaults
			},
			expectedCode: 200,
		},
		{
			name: "Missing task name",
			request: XunfeiCreateTaskRequest{
				Sex:      1,
				AgeGroup: 2,
				Language: "zh",
			},
			expectedCode: 500,
		},
		{
			name: "Empty task name",
			request: XunfeiCreateTaskRequest{
				TaskName: "",
				Sex:      1,
				AgeGroup: 2,
				Language: "zh",
			},
			expectedCode: 500,
		},
		{
			name: "Valid female task",
			request: XunfeiCreateTaskRequest{
				TaskName: "Female Task",
				Sex:      2, // Female
				AgeGroup: 3, // Middle-aged
				Language: "en",
			},
			expectedCode: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/xunfei/create-task", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.XunfeiCreateTask(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                      `json:"code"`
					Data XunfeiCreateTaskResponse `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotEmpty(t, response.Data.TaskID)
			}
		})
	}
}

func TestHandlers_XunfeiSubmitAudio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	tests := []struct {
		name         string
		taskID       string
		textID       string
		textSegID    string
		language     string
		audioFile    bool
		expectedCode int
	}{
		{
			name:         "Valid audio submission",
			taskID:       "test_task_id",
			textID:       "5001",
			textSegID:    "1",
			language:     "zh",
			audioFile:    true,
			expectedCode: 200,
		},
		{
			name:         "Missing task ID",
			taskID:       "",
			textID:       "5001",
			textSegID:    "1",
			language:     "zh",
			audioFile:    true,
			expectedCode: 500,
		},
		{
			name:         "Missing text ID",
			taskID:       "test_task_id",
			textID:       "",
			textSegID:    "1",
			language:     "zh",
			audioFile:    true,
			expectedCode: 500,
		},
		{
			name:         "Missing text segment ID",
			taskID:       "test_task_id",
			textID:       "5001",
			textSegID:    "",
			language:     "zh",
			audioFile:    true,
			expectedCode: 500,
		},
		{
			name:         "Missing language",
			taskID:       "test_task_id",
			textID:       "5001",
			textSegID:    "1",
			language:     "",
			audioFile:    true,
			expectedCode: 500,
		},
		{
			name:         "Missing audio file",
			taskID:       "test_task_id",
			textID:       "5001",
			textSegID:    "1",
			language:     "zh",
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

			if tt.taskID != "" {
				writer.WriteField("taskId", tt.taskID)
			}
			if tt.textID != "" {
				writer.WriteField("textId", tt.textID)
			}
			if tt.textSegID != "" {
				writer.WriteField("textSegId", tt.textSegID)
			}
			if tt.language != "" {
				writer.WriteField("language", tt.language)
			}

			if tt.audioFile {
				part, _ := writer.CreateFormFile("audio", "test.wav")
				part.Write([]byte("fake audio data"))
			}

			writer.Close()

			req := httptest.NewRequest("POST", "/xunfei/submit-audio", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			c.Request = req

			h.XunfeiSubmitAudio(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}
func TestHandlers_XunfeiQueryTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	tests := []struct {
		name         string
		request      XunfeiQueryTaskRequest
		expectedCode int
	}{
		{
			name: "Valid query request",
			request: XunfeiQueryTaskRequest{
				TaskID: "test_task_id",
			},
			expectedCode: 200,
		},
		{
			name: "Missing task ID",
			request: XunfeiQueryTaskRequest{
				TaskID: "",
			},
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/xunfei/query-task", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.XunfeiQueryTask(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                     `json:"code"`
					Data XunfeiQueryTaskResponse `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotEmpty(t, response.Data.TrainID)
			}
		})
	}
}

func TestHandlers_XunfeiGetTrainingTexts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	tests := []struct {
		name         string
		textID       string
		expectedCode int
	}{
		{
			name:         "Valid text ID",
			textID:       "5001",
			expectedCode: 200,
		},
		{
			name:         "Default text ID (empty)",
			textID:       "",
			expectedCode: 200,
		},
		{
			name:         "Custom text ID",
			textID:       "5002",
			expectedCode: 200,
		},
		{
			name:         "Invalid text ID format",
			textID:       "invalid",
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/xunfei/training-texts"
			if tt.textID != "" {
				url += "?textId=" + tt.textID
			}

			req := httptest.NewRequest("GET", url, nil)
			c.Request = req

			h.XunfeiGetTrainingTexts(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int `json:"code"`
					Data struct {
						TextID   int64  `json:"textId"`
						TextName string `json:"textName"`
						TextSegs []struct {
							SegID   interface{} `json:"segId"`
							SegText string      `json:"segText"`
						} `json:"textSegs"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Greater(t, response.Data.TextID, int64(0))
			}
		})
	}
}

// Test request/response structures
func TestXunfeiTTSStructures(t *testing.T) {
	t.Run("XunfeiTTSRequest validation", func(t *testing.T) {
		tests := []struct {
			name    string
			request XunfeiTTSRequest
			valid   bool
		}{
			{
				name: "Valid request",
				request: XunfeiTTSRequest{
					AssetID:  "asset123",
					Text:     "Hello world",
					Language: "zh",
					Key:      "optional_key",
				},
				valid: true,
			},
			{
				name: "Missing AssetID",
				request: XunfeiTTSRequest{
					Text:     "Hello world",
					Language: "zh",
				},
				valid: false,
			},
			{
				name: "Missing Text",
				request: XunfeiTTSRequest{
					AssetID:  "asset123",
					Language: "zh",
				},
				valid: false,
			},
			{
				name: "Missing Language",
				request: XunfeiTTSRequest{
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

				var unmarshaled XunfeiTTSRequest
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

	t.Run("XunfeiCreateTaskRequest validation", func(t *testing.T) {
		request := XunfeiCreateTaskRequest{
			TaskName: "Test Task",
			Sex:      1,
			AgeGroup: 2,
			Language: "zh",
		}

		// Test JSON marshaling
		data, err := json.Marshal(request)
		assert.NoError(t, err)

		var unmarshaled XunfeiCreateTaskRequest
		err = json.Unmarshal(data, &unmarshaled)
		assert.NoError(t, err)

		assert.Equal(t, request.TaskName, unmarshaled.TaskName)
		assert.Equal(t, request.Sex, unmarshaled.Sex)
		assert.Equal(t, request.AgeGroup, unmarshaled.AgeGroup)
		assert.Equal(t, request.Language, unmarshaled.Language)
	})

	t.Run("XunfeiQueryTaskResponse structure", func(t *testing.T) {
		response := XunfeiQueryTaskResponse{
			TaskName:     "Test Task",
			ResourceName: "Test Resource",
			Sex:          1,
			AgeGroup:     2,
			TrainVID:     "train123",
			AssetID:      "asset123",
			TrainID:      "task123",
			AppID:        "app123",
			TrainStatus:  2,
			FailedDesc:   "",
		}

		// Test JSON marshaling
		data, err := json.Marshal(response)
		assert.NoError(t, err)

		var unmarshaled XunfeiQueryTaskResponse
		err = json.Unmarshal(data, &unmarshaled)
		assert.NoError(t, err)

		assert.Equal(t, response.TaskName, unmarshaled.TaskName)
		assert.Equal(t, response.TrainStatus, unmarshaled.TrainStatus)
		assert.Equal(t, response.AssetID, unmarshaled.AssetID)
	})
}

// Test error handling
func TestXunfeiErrorHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	t.Run("Invalid JSON in XunfeiSynthesize", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("POST", "/xunfei/synthesize", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.XunfeiSynthesize(c)

		assert.Equal(t, 500, w.Code)
	})

	t.Run("Invalid JSON in XunfeiCreateTask", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("POST", "/xunfei/create-task", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.XunfeiCreateTask(c)

		assert.Equal(t, 500, w.Code)
	})

	t.Run("Invalid form data in XunfeiSubmitAudio", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("POST", "/xunfei/submit-audio", bytes.NewBufferString("invalid form data"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		c.Request = req

		h.XunfeiSubmitAudio(c)

		assert.Equal(t, 500, w.Code)
	})

	t.Run("Invalid JSON in XunfeiQueryTask", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("POST", "/xunfei/query-task", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.XunfeiQueryTask(c)

		assert.Equal(t, 500, w.Code)
	})
}

// Test different parameter combinations
func TestXunfeiParameterCombinations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	t.Run("Synthesis with different languages", func(t *testing.T) {
		languages := []string{"zh", "en", "ja", "ko", "ru"}

		for _, lang := range languages {
			request := XunfeiTTSRequest{
				AssetID:  "test_asset",
				Text:     "Test text",
				Language: lang,
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, _ := json.Marshal(request)
			req := httptest.NewRequest("POST", "/xunfei/synthesize", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.XunfeiSynthesize(c)

			assert.Equal(t, 200, w.Code, "Failed for language: "+lang)
		}
	})

	t.Run("Task creation with different sex and age combinations", func(t *testing.T) {
		combinations := []struct {
			sex      int
			ageGroup int
		}{
			{1, 1}, // Male, Child
			{1, 2}, // Male, Young
			{1, 3}, // Male, Middle-aged
			{1, 4}, // Male, Senior
			{2, 1}, // Female, Child
			{2, 2}, // Female, Young
			{2, 3}, // Female, Middle-aged
			{2, 4}, // Female, Senior
		}

		for i, combo := range combinations {
			request := XunfeiCreateTaskRequest{
				TaskName: fmt.Sprintf("Task %d", i),
				Sex:      combo.sex,
				AgeGroup: combo.ageGroup,
				Language: "zh",
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, _ := json.Marshal(request)
			req := httptest.NewRequest("POST", "/xunfei/create-task", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.XunfeiCreateTask(c)

			assert.Equal(t, 200, w.Code, "Failed for sex: %d, ageGroup: %d", combo.sex, combo.ageGroup)
		}
	})
}

// Benchmark tests
func BenchmarkHandlers_XunfeiSynthesize(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	request := XunfeiTTSRequest{
		AssetID:  "benchmark_asset",
		Text:     "Benchmark test text",
		Language: "zh",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/xunfei/synthesize", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.XunfeiSynthesize(c)
	}
}

func BenchmarkHandlers_XunfeiCreateTask(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	request := XunfeiCreateTaskRequest{
		TaskName: "Benchmark Task",
		Sex:      1,
		AgeGroup: 2,
		Language: "zh",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/xunfei/create-task", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.XunfeiCreateTask(c)
	}
}

// Edge case tests
func TestXunfeiEdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	t.Run("Synthesis with very long text", func(t *testing.T) {
		longText := strings.Repeat("This is a very long text for synthesis testing. ", 100)

		request := XunfeiTTSRequest{
			AssetID:  "test_asset",
			Text:     longText,
			Language: "zh",
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/xunfei/synthesize", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.XunfeiSynthesize(c)

		assert.Equal(t, 200, w.Code)
	})

	t.Run("Task creation with very long task name", func(t *testing.T) {
		longName := strings.Repeat("Very Long Task Name ", 50)

		request := XunfeiCreateTaskRequest{
			TaskName: longName,
			Sex:      1,
			AgeGroup: 2,
			Language: "zh",
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/xunfei/create-task", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.XunfeiCreateTask(c)

		assert.Equal(t, 200, w.Code)
	})

	t.Run("Audio submission with empty file", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		writer.WriteField("taskId", "test_task")
		writer.WriteField("textId", "5001")
		writer.WriteField("textSegId", "1")
		writer.WriteField("language", "zh")

		// Create empty audio file
		part, _ := writer.CreateFormFile("audio", "empty.wav")
		part.Write([]byte{})
		writer.Close()

		req := httptest.NewRequest("POST", "/xunfei/submit-audio", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		c.Request = req

		h.XunfeiSubmitAudio(c)

		// Should still process (validation happens in voiceclone service)
		assert.Equal(t, 200, w.Code)
	})

	t.Run("Query task with special characters in task ID", func(t *testing.T) {
		request := XunfeiQueryTaskRequest{
			TaskID: "task@#$%^&*()_+-={}[]|\\:;\"'<>?,./",
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/xunfei/query-task", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.XunfeiQueryTask(c)

		// Should handle special characters gracefully
		assert.Equal(t, 200, w.Code)
	})

	t.Run("Get training texts with very large text ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("GET", "/xunfei/training-texts?textId=999999999999", nil)
		c.Request = req

		h.XunfeiGetTrainingTexts(c)

		// Should handle large text IDs
		assert.Equal(t, 200, w.Code)
	})
}
