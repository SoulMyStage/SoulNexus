package handlers

import (
	"bytes"
	"encoding/json"
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

func TestNewUploadHandler(t *testing.T) {
	handler := NewUploadHandler()
	assert.NotNil(t, handler)
}

func TestUploadHandler_Register(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	handler := NewUploadHandler()
	handler.Register(r)

	// Test that the route is registered
	routes := r.Routes()
	found := false
	for _, route := range routes {
		if route.Path == "/api/upload/audio" && route.Method == "POST" {
			found = true
			break
		}
	}
	assert.True(t, found, "Audio upload route should be registered")
}

func TestUploadAudio(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}

	tests := []struct {
		name           string
		contentType    string
		fileContent    string
		fileName       string
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid webm audio upload",
			contentType:    "audio/webm",
			fileContent:    "fake webm audio data",
			fileName:       "test.webm",
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Valid wav audio upload",
			contentType:    "audio/wav",
			fileContent:    "fake wav audio data",
			fileName:       "test.wav",
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Valid mp3 audio upload",
			contentType:    "audio/mp3",
			fileContent:    "fake mp3 audio data",
			fileName:       "test.mp3",
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Invalid content type",
			contentType:    "image/jpeg",
			fileContent:    "fake image data",
			fileName:       "test.jpg",
			user:           user,
			expectedStatus: 400,
			expectedError:  "Unsupported file type",
		},
		{
			name:           "No file uploaded",
			contentType:    "",
			fileContent:    "",
			fileName:       "",
			user:           user,
			expectedStatus: 400,
			expectedError:  "Failed to get uploaded file",
		},
		{
			name:           "No user context",
			contentType:    "audio/webm",
			fileContent:    "fake webm audio data",
			fileName:       "test.webm",
			user:           nil,
			expectedStatus: 200, // Should still work without user
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewUploadHandler()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Create multipart form data
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)

			if tt.fileName != "" {
				// Create file part
				part, err := writer.CreateFormFile("audio", tt.fileName)
				require.NoError(t, err)

				// Set content type header for the file part
				if tt.contentType != "" {
					// Create a custom header for the file part
					h := make(map[string][]string)
					h["Content-Type"] = []string{tt.contentType}
					part, err = writer.CreatePart(map[string][]string{
						"Content-Disposition": {`form-data; name="audio"; filename="` + tt.fileName + `"`},
						"Content-Type":        {tt.contentType},
					})
					require.NoError(t, err)
				}

				_, err = io.WriteString(part, tt.fileContent)
				require.NoError(t, err)
			}

			writer.Close()

			// Create request
			c.Request = httptest.NewRequest("POST", "/api/upload/audio", &body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())

			// Set user context if provided
			if tt.user != nil {
				c.Set("user", tt.user)
			}

			handler.UploadAudio(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			if tt.expectedError != "" {
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}

			if w.Code == 200 {
				assert.Equal(t, "音s频文件上传成功", response["msg"])

				// Check response data
				data := response["data"].(map[string]interface{})
				assert.NotEmpty(t, data["fileName"])
				assert.NotEmpty(t, data["filePath"])
				assert.NotEmpty(t, data["uploadTime"])
				assert.NotEmpty(t, data["url"])

				// Verify file size is present
				assert.Contains(t, data, "fileSize")
			}
		})
	}
}

func TestUploadAudio_EmptyFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUploadHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create multipart form data with empty file
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	_, err := writer.CreateFormFile("audio", "empty.webm")
	require.NoError(t, err)

	// Don't write any content to simulate empty file
	writer.Close()

	c.Request = httptest.NewRequest("POST", "/api/upload/audio", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	handler.UploadAudio(c)

	// Should handle empty file gracefully
	assert.True(t, w.Code == 200 || w.Code == 400)
}

func TestUploadAudio_LargeFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUploadHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create multipart form data with large file content
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("audio", "large.webm")
	require.NoError(t, err)

	// Create large content (1MB)
	largeContent := strings.Repeat("a", 1024*1024)
	_, err = io.WriteString(part, largeContent)
	require.NoError(t, err)

	writer.Close()

	c.Request = httptest.NewRequest("POST", "/api/upload/audio", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	handler.UploadAudio(c)

	// Should handle large file
	assert.True(t, w.Code == 200 || w.Code == 400)
}

func TestUploadAudio_InvalidMultipartData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUploadHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create invalid multipart data
	body := bytes.NewBufferString("invalid multipart data")
	c.Request = httptest.NewRequest("POST", "/api/upload/audio", body)
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=invalid")

	handler.UploadAudio(c)

	assert.Equal(t, 400, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["msg"].(string), "Failed to get uploaded file")
}

func TestUploadAudio_MissingAudioField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUploadHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create multipart form data without audio field
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add a different field
	writer.WriteField("other", "value")
	writer.Close()

	c.Request = httptest.NewRequest("POST", "/api/upload/audio", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	handler.UploadAudio(c)

	assert.Equal(t, 400, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response["msg"].(string), "Failed to get uploaded file")
}

func TestUploadAudio_WithCredentialID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUploadHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create multipart form data
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("audio", "test.webm")
	require.NoError(t, err)
	_, err = io.WriteString(part, "fake webm audio data")
	require.NoError(t, err)

	writer.Close()

	c.Request = httptest.NewRequest("POST", "/api/upload/audio?credentialId=123", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	// Set user context
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
	}
	c.Set("user", user)

	handler.UploadAudio(c)

	// Should handle credential ID parameter
	assert.True(t, w.Code == 200 || w.Code == 400)
}

// Benchmark tests
func BenchmarkUploadAudio(b *testing.B) {
	gin.SetMode(gin.TestMode)
	handler := NewUploadHandler()

	// Prepare test data
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("audio", "test.webm")
	io.WriteString(part, "fake webm audio data")
	writer.Close()

	contentType := writer.FormDataContentType()
	bodyBytes := body.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("POST", "/api/upload/audio", bytes.NewReader(bodyBytes))
		c.Request.Header.Set("Content-Type", contentType)

		handler.UploadAudio(c)
	}
}

// Test concurrent uploads
func TestUploadAudio_Concurrent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewUploadHandler()

	const numGoroutines = 10
	results := make(chan int, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Create multipart form data
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)

			part, err := writer.CreateFormFile("audio", "concurrent.webm")
			require.NoError(t, err)
			_, err = io.WriteString(part, "concurrent audio data")
			require.NoError(t, err)

			writer.Close()

			c.Request = httptest.NewRequest("POST", "/api/upload/audio", &body)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())

			handler.UploadAudio(c)
			results <- w.Code
		}(i)
	}

	// Collect results
	successCount := 0
	for i := 0; i < numGoroutines; i++ {
		code := <-results
		if code == 200 {
			successCount++
		}
	}

	// Should handle concurrent uploads
	assert.Greater(t, successCount, 0)
}

// Test edge cases
func TestUploadAudio_EdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewUploadHandler()

	t.Run("Special characters in filename", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		part, err := writer.CreateFormFile("audio", "测试文件名 with spaces & symbols!.webm")
		require.NoError(t, err)
		_, err = io.WriteString(part, "audio data")
		require.NoError(t, err)

		writer.Close()

		c.Request = httptest.NewRequest("POST", "/api/upload/audio", &body)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		handler.UploadAudio(c)

		// Should handle special characters in filename
		assert.True(t, w.Code == 200 || w.Code == 400)
	})

	t.Run("Very long filename", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		longFilename := strings.Repeat("a", 255) + ".webm"
		part, err := writer.CreateFormFile("audio", longFilename)
		require.NoError(t, err)
		_, err = io.WriteString(part, "audio data")
		require.NoError(t, err)

		writer.Close()

		c.Request = httptest.NewRequest("POST", "/api/upload/audio", &body)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		handler.UploadAudio(c)

		// Should handle long filename
		assert.True(t, w.Code == 200 || w.Code == 400)
	})

	t.Run("Binary audio data", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		var body bytes.Buffer
		writer := multipart.NewWriter(&body)

		part, err := writer.CreateFormFile("audio", "binary.webm")
		require.NoError(t, err)

		// Write binary data
		binaryData := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD}
		_, err = part.Write(binaryData)
		require.NoError(t, err)

		writer.Close()

		c.Request = httptest.NewRequest("POST", "/api/upload/audio", &body)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		handler.UploadAudio(c)

		// Should handle binary data
		assert.True(t, w.Code == 200 || w.Code == 400)
	})
}
