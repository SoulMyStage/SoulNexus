package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/constants"
)

func setupAssistantsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate models
	err = db.AutoMigrate(
		&models.User{},
		&models.Assistant{},
		&models.AssistantTool{},
		&models.JSTemplate{},
	)
	require.NoError(t, err)

	return db
}

func setupAssistantsTestRouter(handlers *Handlers) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add middleware to set database
	router.Use(func(c *gin.Context) {
		c.Set(constants.DbField, handlers.db)
		c.Next()
	})

	// Assistant routes
	router.POST("/assistant/add", handlers.CreateAssistant)
	router.GET("/assistant", handlers.ListAssistants)
	router.GET("/assistant/:id", handlers.GetAssistant)
	router.PUT("/assistant/:id", handlers.UpdateAssistant)
	router.DELETE("/assistant/:id", handlers.DeleteAssistant)
	router.PUT("/assistant/:id/js", handlers.UpdateAssistantJS)
	router.GET("/assistant/lingecho/client/:id/loader.js", handlers.ServeVoiceSculptorLoaderJS)

	return router
}

func createTestAssistant(t *testing.T, db *gorm.DB, userID uint) *models.Assistant {
	assistant := &models.Assistant{
		Name:        "Test Assistant",
		Description: "A test assistant",
		Icon:        "test-icon",
		UserID:      userID,
		Language:    "en",
		Speaker:     "default",
		TtsProvider: "openai",
		LLMModel:    "gpt-3.5-turbo",
	}

	err := db.Create(assistant).Error
	require.NoError(t, err)

	return assistant
}

func TestHandlers_handleCreateAssistant(t *testing.T) {
	db := setupAssistantsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Valid assistant creation",
			requestBody: map[string]interface{}{
				"name":        "New Assistant",
				"description": "A new test assistant",
				"icon":        "new-icon",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Missing name",
			requestBody: map[string]interface{}{
				"description": "Assistant without name",
				"icon":        "icon",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Empty name",
			requestBody: map[string]interface{}{
				"name":        "",
				"description": "Assistant with empty name",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Valid minimal assistant",
			requestBody: map[string]interface{}{
				"name": "Minimal Assistant",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/assistant/add", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "id")
				assert.Contains(t, data, "name")
			}
		})
	}
}

func TestHandlers_handleListAssistants(t *testing.T) {
	db := setupAssistantsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test assistants
	assistant1 := createTestAssistant(t, db, user.ID)
	assistant2 := createTestAssistant(t, db, user.ID)
	assistant2.Name = "Second Assistant"

	// Verify assistants were created
	assert.NotNil(t, assistant1)
	assert.NotNil(t, assistant2)
	db.Save(assistant2)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	req := httptest.NewRequest(http.MethodGet, "/assistant", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Verify response structure
	assert.Contains(t, response, "data")
	data := response["data"].([]interface{})
	assert.Len(t, data, 2)

	// Verify assistant data
	firstAssistant := data[0].(map[string]interface{})
	assert.Contains(t, firstAssistant, "id")
	assert.Contains(t, firstAssistant, "name")
	assert.Contains(t, firstAssistant, "description")
}

func TestHandlers_handleGetAssistant(t *testing.T) {
	db := setupAssistantsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test assistant
	assistant := createTestAssistant(t, db, user.ID)

	// Verify assistant was created
	assert.NotNil(t, assistant)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		assistantID    string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Valid assistant ID",
			assistantID:    "1",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Invalid assistant ID",
			assistantID:    "999",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:           "Non-numeric assistant ID",
			assistantID:    "abc",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/assistant/"+tt.assistantID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "id")
				assert.Contains(t, data, "name")
				assert.Equal(t, assistant.Name, data["name"])
			}
		})
	}
}

func TestHandlers_handleUpdateAssistant(t *testing.T) {
	db := setupAssistantsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test assistant
	assistant := createTestAssistant(t, db, user.ID)

	// Verify assistant was created
	assert.NotNil(t, assistant)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		assistantID    string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name:        "Valid assistant update",
			assistantID: "1",
			requestBody: map[string]interface{}{
				"name":        "Updated Assistant",
				"description": "Updated description",
				"icon":        "updated-icon",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Partial update",
			assistantID: "1",
			requestBody: map[string]interface{}{
				"name": "Partially Updated",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Invalid assistant ID",
			assistantID: "999",
			requestBody: map[string]interface{}{
				"name": "Updated Name",
			},
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:        "Empty name",
			assistantID: "1",
			requestBody: map[string]interface{}{
				"name": "",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, "/assistant/"+tt.assistantID, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")
			}
		})
	}
}

func TestHandlers_handleDeleteAssistant(t *testing.T) {
	db := setupAssistantsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test assistant
	assistant := createTestAssistant(t, db, user.ID)

	// Verify assistant was created
	assert.NotNil(t, assistant)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		assistantID    string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Valid assistant deletion",
			assistantID:    "1",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Invalid assistant ID",
			assistantID:    "999",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:           "Non-numeric assistant ID",
			assistantID:    "abc",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/assistant/"+tt.assistantID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				// Verify assistant is deleted
				var count int64
				db.Model(&models.Assistant{}).Where("id = ?", tt.assistantID).Count(&count)
				assert.Equal(t, int64(0), count)
			}
		})
	}
}

func TestHandlers_handleUpdateAssistantJS(t *testing.T) {
	db := setupAssistantsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test assistant
	assistant := createTestAssistant(t, db, user.ID)

	// Verify assistant was created
	assert.NotNil(t, assistant)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		assistantID    string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name:        "Valid JS template update",
			assistantID: "1",
			requestBody: map[string]interface{}{
				"jsTemplateId": "template-123",
				"jsCode":       "console.log('Hello World');",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Empty JS code",
			assistantID: "1",
			requestBody: map[string]interface{}{
				"jsCode": "",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Invalid assistant ID",
			assistantID: "999",
			requestBody: map[string]interface{}{
				"jsCode": "console.log('test');",
			},
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, "/assistant/"+tt.assistantID+"/js", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandlers_handleGetAssistantLoader(t *testing.T) {
	db := setupAssistantsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test assistant
	assistant := createTestAssistant(t, db, user.ID)

	// Verify assistant was created
	assert.NotNil(t, assistant)

	tests := []struct {
		name           string
		assistantID    string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Valid assistant loader",
			assistantID:    "1",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Invalid assistant ID",
			assistantID:    "999",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:           "Non-numeric assistant ID",
			assistantID:    "abc",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/assistant/lingecho/client/"+tt.assistantID+"/loader.js", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				// Should return JavaScript content
				assert.Contains(t, w.Header().Get("Content-Type"), "javascript")
				assert.NotEmpty(t, w.Body.String())
			}
		})
	}
}

func BenchmarkHandlers_handleCreateAssistant(b *testing.B) {
	db := setupAssistantsTestDB(&testing.T{})
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	user := createTestUser(&testing.T{}, db)
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	requestBody := map[string]interface{}{
		"name":        "Benchmark Assistant",
		"description": "A benchmark test assistant",
	}
	body, _ := json.Marshal(requestBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/assistant/add", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkHandlers_handleListAssistants(b *testing.B) {
	db := setupAssistantsTestDB(&testing.T{})
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	user := createTestUser(&testing.T{}, db)
	createTestAssistant(&testing.T{}, db, user.ID)
	createTestAssistant(&testing.T{}, db, user.ID)

	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/assistant", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func TestHandlers_assistantErrorHandling(t *testing.T) {
	db := setupAssistantsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	user := createTestUser(t, db)
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		method         string
		path           string
		body           interface{}
		expectedStatus int
	}{
		{
			name:           "Invalid JSON in create assistant",
			method:         http.MethodPost,
			path:           "/assistant/add",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON in update assistant",
			method:         http.MethodPut,
			path:           "/assistant/1",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON in update JS",
			method:         http.MethodPut,
			path:           "/assistant/1/js",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if str, ok := tt.body.(string); ok {
				body = []byte(str)
			} else {
				body, _ = json.Marshal(tt.body)
			}

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestHandlers_assistantOwnershipValidation(t *testing.T) {
	db := setupAssistantsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	// Create two users
	user1 := createTestUser(t, db)
	user2 := &models.User{
		Email:       "user2@example.com",
		Password:    "hashedpassword",
		DisplayName: "User 2",
		Activated:   true,
	}
	db.Create(user2)

	// Create assistant for user1
	assistant := createTestAssistant(t, db, user1.ID)

	// Verify assistant was created
	assert.NotNil(t, assistant)

	// Try to access user1's assistant as user2
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user2)
		c.Next()
	})

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "Get other user's assistant",
			method:         http.MethodGet,
			path:           "/assistant/1",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Update other user's assistant",
			method:         http.MethodPut,
			path:           "/assistant/1",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Delete other user's assistant",
			method:         http.MethodDelete,
			path:           "/assistant/1",
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.method == http.MethodPut {
				requestBody := map[string]interface{}{"name": "Updated"}
				body, _ = json.Marshal(requestBody)
			}

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// May return different status codes depending on implementation
			// The key is that it should not return success
			assert.NotEqual(t, http.StatusOK, w.Code)
		})
	}
}

func TestHandlers_assistantCompleteFlow(t *testing.T) {
	db := setupAssistantsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantsTestRouter(handlers)

	user := createTestUser(t, db)
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	// Test complete assistant lifecycle
	t.Run("Complete assistant lifecycle", func(t *testing.T) {
		// 1. Create assistant
		createBody := map[string]interface{}{
			"name":        "Lifecycle Assistant",
			"description": "Testing complete lifecycle",
			"icon":        "lifecycle-icon",
		}
		body, _ := json.Marshal(createBody)
		req := httptest.NewRequest(http.MethodPost, "/assistant/add", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 2. List assistants
		req = httptest.NewRequest(http.MethodGet, "/assistant", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 3. Get specific assistant
		req = httptest.NewRequest(http.MethodGet, "/assistant/1", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 4. Update assistant
		updateBody := map[string]interface{}{
			"name":        "Updated Lifecycle Assistant",
			"description": "Updated description",
		}
		body, _ = json.Marshal(updateBody)
		req = httptest.NewRequest(http.MethodPut, "/assistant/1", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 5. Update JS template
		jsBody := map[string]interface{}{
			"jsCode": "console.log('Updated JS code');",
		}
		body, _ = json.Marshal(jsBody)
		req = httptest.NewRequest(http.MethodPut, "/assistant/1/js", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 6. Get loader script
		req = httptest.NewRequest(http.MethodGet, "/assistant/lingecho/client/1/loader.js", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 7. Delete assistant
		req = httptest.NewRequest(http.MethodDelete, "/assistant/1", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
