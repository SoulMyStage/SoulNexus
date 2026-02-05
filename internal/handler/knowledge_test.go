package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/constants"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupKnowledgeTestDB(t *testing.T) (*gorm.DB, func()) {
	db := setupTestDB(t)

	// Auto migrate tables
	err := db.AutoMigrate(
		&models.User{},
		&models.Knowledge{},
		&models.Group{},
		&models.GroupMember{},
	)
	assert.NoError(t, err)

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS users")
		db.Exec("DROP TABLE IF EXISTS knowledges")
		db.Exec("DROP TABLE IF EXISTS groups")
		db.Exec("DROP TABLE IF EXISTS group_members")
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}

	return db, cleanup
}

func TestHandlers_CreateKnowledgeBase(t *testing.T) {
	db, cleanup := setupKnowledgeTestDB(t)
	defer cleanup()

	// Create test user
	user := &models.User{
		Email:       "knowledge@test.com",
		Password:    "password123",
		DisplayName: "knowledgeuser",
	}
	db.Create(user)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		setupForm  func() (*bytes.Buffer, string)
		wantStatus int
		wantError  bool
	}{
		{
			name: "Missing file",
			setupForm: func() (*bytes.Buffer, string) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)
				writer.WriteField(constants.FormFieldKnowledgeName, "Test Knowledge")
				writer.WriteField(constants.FormFieldProvider, "aliyun")
				writer.Close()
				return body, writer.FormDataContentType()
			},
			wantStatus: http.StatusOK,
			wantError:  true,
		},
		{
			name: "Missing knowledge name",
			setupForm: func() (*bytes.Buffer, string) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				// Add a dummy file
				part, _ := writer.CreateFormFile(constants.FormFieldFile, "test.txt")
				part.Write([]byte("test content"))

				writer.WriteField(constants.FormFieldProvider, "aliyun")
				writer.Close()
				return body, writer.FormDataContentType()
			},
			wantStatus: http.StatusOK,
			wantError:  true, // Will fail due to knowledge base not being enabled
		},
		{
			name: "Valid request with file",
			setupForm: func() (*bytes.Buffer, string) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				// Add a dummy file
				part, _ := writer.CreateFormFile(constants.FormFieldFile, "test.txt")
				part.Write([]byte("test content"))

				writer.WriteField(constants.FormFieldKnowledgeName, "Test Knowledge")
				writer.WriteField(constants.FormFieldProvider, "aliyun")
				writer.Close()
				return body, writer.FormDataContentType()
			},
			wantStatus: http.StatusOK,
			wantError:  true, // Will fail due to knowledge base not being enabled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, contentType := tt.setupForm()
			c.Request = httptest.NewRequest("POST", "/api/knowledge", body)
			c.Request.Header.Set("Content-Type", contentType)
			c.Set("user", user)

			h.CreateKnowledgeBase(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError {
				assert.Equal(t, "fail", resp["status"])
			} else {
				assert.Equal(t, "success", resp["status"])
			}
		})
	}
}

func TestHandlers_UploadFileToKnowledgeBase(t *testing.T) {
	db, cleanup := setupKnowledgeTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "knowledge@test.com",
		Password:    "password123",
		DisplayName: "knowledgeuser",
	}
	db.Create(user)

	// Create test knowledge base
	knowledge := &models.Knowledge{
		UserID:        int(user.ID),
		KnowledgeKey:  "test-knowledge-key",
		KnowledgeName: "Test Knowledge",
		Provider:      "aliyun",
		Config:        `{"test": "config"}`,
	}
	db.Create(knowledge)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		setupForm  func() (*bytes.Buffer, string)
		wantStatus int
		wantError  bool
	}{
		{
			name: "Missing file",
			setupForm: func() (*bytes.Buffer, string) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)
				writer.WriteField(constants.FormFieldKnowledgeKey, knowledge.KnowledgeKey)
				writer.Close()
				return body, writer.FormDataContentType()
			},
			wantStatus: http.StatusOK,
			wantError:  true,
		},
		{
			name: "Missing knowledge key",
			setupForm: func() (*bytes.Buffer, string) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				part, _ := writer.CreateFormFile(constants.FormFieldFile, "test.txt")
				part.Write([]byte("test content"))

				writer.Close()
				return body, writer.FormDataContentType()
			},
			wantStatus: http.StatusOK,
			wantError:  true,
		},
		{
			name: "Valid upload request",
			setupForm: func() (*bytes.Buffer, string) {
				body := &bytes.Buffer{}
				writer := multipart.NewWriter(body)

				part, _ := writer.CreateFormFile(constants.FormFieldFile, "test.txt")
				part.Write([]byte("test content"))

				writer.WriteField(constants.FormFieldKnowledgeKey, knowledge.KnowledgeKey)
				writer.Close()
				return body, writer.FormDataContentType()
			},
			wantStatus: http.StatusOK,
			wantError:  true, // Will fail due to knowledge base not being enabled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, contentType := tt.setupForm()
			c.Request = httptest.NewRequest("POST", "/api/knowledge/upload", body)
			c.Request.Header.Set("Content-Type", contentType)

			h.UploadFileToKnowledgeBase(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError {
				assert.Equal(t, "fail", resp["status"])
			} else {
				assert.Equal(t, "success", resp["status"])
			}
		})
	}
}

func TestHandlers_GetKnowledgeBase(t *testing.T) {
	db, cleanup := setupKnowledgeTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "knowledge@test.com",
		Password:    "password123",
		DisplayName: "knowledgeuser",
	}
	db.Create(user)

	// Create test knowledge bases
	knowledgeBases := []models.Knowledge{
		{
			UserID:        int(user.ID),
			KnowledgeKey:  "test-knowledge-1",
			KnowledgeName: "Knowledge 1",
			Provider:      "aliyun",
			Config:        `{"test": "config1"}`,
		},
		{
			UserID:        int(user.ID),
			KnowledgeKey:  "test-knowledge-2",
			KnowledgeName: "Knowledge 2",
			Provider:      "milvus",
			Config:        `{"test": "config2"}`,
		},
	}

	for _, kb := range knowledgeBases {
		db.Create(&kb)
	}

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/api/knowledge", nil)
	c.Set("user", user)

	h.GetKnowledgeBase(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])
	data := resp["data"].([]interface{})
	assert.Equal(t, 2, len(data))
}

func TestHandlers_DeleteKnowledgeBase(t *testing.T) {
	db, cleanup := setupKnowledgeTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "knowledge@test.com",
		Password:    "password123",
		DisplayName: "knowledgeuser",
	}
	db.Create(user)

	knowledge := &models.Knowledge{
		UserID:        int(user.ID),
		KnowledgeKey:  "test-knowledge-key",
		KnowledgeName: "Test Knowledge",
		Provider:      "aliyun",
		Config:        `{"test": "config"}`,
	}
	db.Create(knowledge)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		knowledgeKey string
		wantStatus   int
		wantError    bool
	}{
		{
			name:         "Valid knowledge key",
			knowledgeKey: knowledge.KnowledgeKey,
			wantStatus:   http.StatusOK,
			wantError:    true, // Will fail due to knowledge base not being enabled
		},
		{
			name:         "Missing knowledge key",
			knowledgeKey: "",
			wantStatus:   http.StatusOK,
			wantError:    true,
		},
		{
			name:         "Non-existent knowledge key",
			knowledgeKey: "non-existent-key",
			wantStatus:   http.StatusOK,
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/api/knowledge"
			if tt.knowledgeKey != "" {
				url += "?" + constants.QueryParamKnowledgeKey + "=" + tt.knowledgeKey
			}

			c.Request = httptest.NewRequest("DELETE", url, nil)

			h.DeleteKnowledgeBase(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError {
				assert.Equal(t, "fail", resp["status"])
			} else {
				assert.Equal(t, "success", resp["status"])
			}
		})
	}
}

func TestHandlers_GetKnowledgeBase_EmptyList(t *testing.T) {
	db, cleanup := setupKnowledgeTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "knowledge@test.com",
		Password:    "password123",
		DisplayName: "knowledgeuser",
	}
	db.Create(user)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/api/knowledge", nil)
	c.Set("user", user)

	h.GetKnowledgeBase(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])
	data := resp["data"].([]interface{})
	assert.Equal(t, 0, len(data))
}

func TestHandlers_CreateKnowledgeBase_WithGroup(t *testing.T) {
	db, cleanup := setupKnowledgeTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "knowledge@test.com",
		Password:    "password123",
		DisplayName: "knowledgeuser",
	}
	db.Create(user)

	// Create test group
	group := &models.Group{
		Name:      "Test Group",
		CreatorID: user.ID,
	}
	db.Create(group)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add a dummy file
	part, _ := writer.CreateFormFile(constants.FormFieldFile, "test.txt")
	part.Write([]byte("test content"))

	writer.WriteField(constants.FormFieldKnowledgeName, "Test Knowledge")
	writer.WriteField(constants.FormFieldProvider, "aliyun")
	writer.WriteField("group_id", fmt.Sprintf("%d", group.ID))
	writer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("POST", "/api/knowledge", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set("user", user)

	h.CreateKnowledgeBase(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Will fail due to knowledge base not being enabled
	assert.Equal(t, "fail", resp["status"])
}

func TestHandlers_CreateKnowledgeBase_UnauthorizedGroup(t *testing.T) {
	db, cleanup := setupKnowledgeTestDB(t)
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

	// Group belongs to user1
	group := &models.Group{
		Name:      "Test Group",
		CreatorID: user1.ID,
	}
	db.Create(group)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, _ := writer.CreateFormFile(constants.FormFieldFile, "test.txt")
	part.Write([]byte("test content"))

	writer.WriteField(constants.FormFieldKnowledgeName, "Test Knowledge")
	writer.WriteField(constants.FormFieldProvider, "aliyun")
	writer.WriteField("group_id", fmt.Sprintf("%d", group.ID))
	writer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("POST", "/api/knowledge", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set("user", user2) // Wrong user

	h.CreateKnowledgeBase(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "fail", resp["status"])
	assert.Equal(t, "Insufficient permissions", resp["message"])
}

// Test helper functions
func TestGetStringFromConfig(t *testing.T) {
	config := map[string]interface{}{
		"string_key": "test_value",
		"int_key":    123,
		"nil_key":    nil,
	}

	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "Valid string key",
			key:      "string_key",
			expected: "test_value",
		},
		{
			name:     "Non-string value",
			key:      "int_key",
			expected: "",
		},
		{
			name:     "Non-existent key",
			key:      "missing_key",
			expected: "",
		},
		{
			name:     "Nil value",
			key:      "nil_key",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStringFromConfig(config, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetKnowledgeBaseConfig(t *testing.T) {
	tests := []struct {
		name     string
		provider string
	}{
		{
			name:     "Aliyun provider",
			provider: "aliyun",
		},
		{
			name:     "Milvus provider",
			provider: "zilliz",
		},
		{
			name:     "Qdrant provider",
			provider: "qdrant",
		},
		{
			name:     "Elasticsearch provider",
			provider: "elasticsearch",
		},
		{
			name:     "Pinecone provider",
			provider: "pinecone",
		},
		{
			name:     "Default provider",
			provider: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := getKnowledgeBaseConfig(tt.provider)
			assert.NotNil(t, config)
			assert.IsType(t, map[string]interface{}{}, config)
		})
	}
}

func TestCalculateMD5(t *testing.T) {
	// Create a temporary file for testing
	content := "test content"
	tmpFile, err := os.CreateTemp("", "test_md5_*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write content to file
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)

	// Reset file pointer
	_, err = tmpFile.Seek(0, 0)
	require.NoError(t, err)

	md5Hash, err := CalculateMD5(tmpFile)

	assert.NoError(t, err)
	assert.NotEmpty(t, md5Hash)
	assert.Equal(t, 32, len(md5Hash)) // MD5 hash is 32 characters
}

func TestGetFileSize(t *testing.T) {
	header := &multipart.FileHeader{
		Filename: "test.txt",
		Size:     1024,
	}

	size, err := GetFileSize(header)

	assert.NoError(t, err)
	assert.Equal(t, "1024", size)
}

// Benchmark tests
func BenchmarkHandlers_GetKnowledgeBase(b *testing.B) {
	db, cleanup := setupKnowledgeTestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "knowledge@test.com",
		Password:    "password123",
		DisplayName: "knowledgeuser",
	}
	db.Create(user)

	// Create test data
	for i := 0; i < 10; i++ {
		knowledge := &models.Knowledge{
			UserID:        int(user.ID),
			KnowledgeKey:  fmt.Sprintf("test-knowledge-%d", i),
			KnowledgeName: fmt.Sprintf("Knowledge %d", i),
			Provider:      "aliyun",
			Config:        `{"test": "config"}`,
		}
		db.Create(knowledge)
	}

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", "/api/knowledge", nil)
		c.Set("user", user)

		h.GetKnowledgeBase(c)
	}
}

func BenchmarkCalculateMD5(b *testing.B) {
	content := "test content for MD5 calculation benchmark"

	// Create a temporary file for benchmarking
	tmpFile, err := os.CreateTemp("", "bench_md5_*.txt")
	require.NoError(b, err)
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write content to file
	_, err = tmpFile.WriteString(content)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset file pointer
		tmpFile.Seek(0, 0)
		CalculateMD5(tmpFile)
	}
}

// Mock multipart.File for testing
type mockFile struct {
	*strings.Reader
}

func (m *mockFile) Close() error {
	return nil
}

func newMockFile(content string) *mockFile {
	return &mockFile{
		Reader: strings.NewReader(content),
	}
}

func TestCalculateMD5_WithMockFile(t *testing.T) {
	content := "test content"
	file := newMockFile(content)

	md5Hash, err := CalculateMD5(file)

	assert.NoError(t, err)
	assert.NotEmpty(t, md5Hash)
	assert.Equal(t, 32, len(md5Hash))
}

// Test UploadFile function (would need actual HTTP server for full test)
func TestUploadFile_InvalidURL(t *testing.T) {
	file := newMockFile("test content")
	headers := map[string]string{
		"Content-Type": "application/octet-stream",
	}

	err := UploadFile("invalid-url", headers, file)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send request")
}
