package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/constants"
)

func setupAssistantToolsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate models
	err = db.AutoMigrate(
		&models.User{},
		&models.Assistant{},
		&models.AssistantTool{},
	)
	require.NoError(t, err)

	return db
}

func setupAssistantToolsTestRouter(handlers *Handlers) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Setup session store
	store := cookie.NewStore([]byte("test-secret"))
	router.Use(sessions.Sessions("test-session", store))

	// Add middleware to set database
	router.Use(func(c *gin.Context) {
		c.Set(constants.DbField, handlers.db)
		c.Next()
	})

	// Assistant tools routes
	router.GET("/assistant/:id/tools", handlers.ListAssistantTools)
	router.POST("/assistant/:id/tools", handlers.CreateAssistantTool)
	router.PUT("/assistant/:id/tools/:toolId", handlers.UpdateAssistantTool)
	router.DELETE("/assistant/:id/tools/:toolId", handlers.DeleteAssistantTool)
	router.POST("/assistant/:id/tools/:toolId/test", handlers.TestAssistantTool)

	return router
}

func createTestAssistantTool(t *testing.T, db *gorm.DB, assistantID int64) *models.AssistantTool {
	tool := &models.AssistantTool{
		AssistantID: assistantID,
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  `{"type": "object", "properties": {"input": {"type": "string"}}}`,
		Code:        "return 'Hello ' + args.input;",
		Enabled:     true,
	}

	err := db.Create(tool).Error
	require.NoError(t, err)

	return tool
}

func TestHandlers_handleCreateAssistantTool(t *testing.T) {
	db := setupAssistantToolsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantToolsTestRouter(handlers)

	// Create test user and assistant
	user := createTestUser(t, db)
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
			name:        "Valid tool creation",
			assistantID: "1",
			requestBody: map[string]interface{}{
				"name":        "new_tool",
				"description": "A new test tool",
				"parameters":  `{"type": "object", "properties": {"text": {"type": "string"}}}`,
				"code":        "return 'Processed: ' + args.text;",
				"enabled":     true,
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Tool with webhook URL",
			assistantID: "1",
			requestBody: map[string]interface{}{
				"name":        "webhook_tool",
				"description": "A webhook-based tool",
				"parameters":  `{"type": "object", "properties": {"data": {"type": "string"}}}`,
				"webhookUrl":  "https://example.com/webhook",
				"enabled":     true,
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Missing name",
			assistantID: "1",
			requestBody: map[string]interface{}{
				"description": "Tool without name",
				"parameters":  `{"type": "object"}`,
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:        "Invalid tool name (special characters)",
			assistantID: "1",
			requestBody: map[string]interface{}{
				"name":        "invalid-tool-name!",
				"description": "Tool with invalid name",
				"parameters":  `{"type": "object"}`,
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:        "Missing description",
			assistantID: "1",
			requestBody: map[string]interface{}{
				"name":       "tool_without_desc",
				"parameters": `{"type": "object"}`,
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:        "Invalid JSON parameters",
			assistantID: "1",
			requestBody: map[string]interface{}{
				"name":        "invalid_params_tool",
				"description": "Tool with invalid parameters",
				"parameters":  "invalid json",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:        "Invalid assistant ID",
			assistantID: "999",
			requestBody: map[string]interface{}{
				"name":        "test_tool",
				"description": "Test tool",
				"parameters":  `{"type": "object"}`,
			},
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/assistant/"+tt.assistantID+"/tools", bytes.NewBuffer(body))
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
				assert.Equal(t, tt.requestBody.(map[string]interface{})["name"], data["name"])
			}
		})
	}
}

func TestHandlers_handleUpdateAssistantTool(t *testing.T) {
	db := setupAssistantToolsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantToolsTestRouter(handlers)

	// Create test user, assistant, and tool
	user := createTestUser(t, db)
	assistant := createTestAssistant(t, db, user.ID)
	tool := createTestAssistantTool(t, db, assistant.ID)

	// Verify test data was created
	assert.NotNil(t, tool)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		assistantID    string
		toolID         string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name:        "Valid tool update",
			assistantID: "1",
			toolID:      "1",
			requestBody: map[string]interface{}{
				"name":        "updated_tool",
				"description": "Updated test tool",
				"parameters":  `{"type": "object", "properties": {"newParam": {"type": "string"}}}`,
				"code":        "return 'Updated: ' + args.newParam;",
				"enabled":     false,
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Partial update",
			assistantID: "1",
			toolID:      "1",
			requestBody: map[string]interface{}{
				"description": "Partially updated description",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Update with webhook URL",
			assistantID: "1",
			toolID:      "1",
			requestBody: map[string]interface{}{
				"webhookUrl": "https://updated.example.com/webhook",
				"code":       "", // Clear code when using webhook
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Invalid tool name",
			assistantID: "1",
			toolID:      "1",
			requestBody: map[string]interface{}{
				"name": "invalid-name!",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:        "Invalid JSON parameters",
			assistantID: "1",
			toolID:      "1",
			requestBody: map[string]interface{}{
				"parameters": "invalid json",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:        "Invalid assistant ID",
			assistantID: "999",
			toolID:      "1",
			requestBody: map[string]interface{}{
				"description": "Updated description",
			},
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:        "Invalid tool ID",
			assistantID: "1",
			toolID:      "999",
			requestBody: map[string]interface{}{
				"description": "Updated description",
			},
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, "/assistant/"+tt.assistantID+"/tools/"+tt.toolID, bytes.NewBuffer(body))
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

func TestHandlers_handleDeleteAssistantTool(t *testing.T) {
	db := setupAssistantToolsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantToolsTestRouter(handlers)

	// Create test user, assistant, and tool
	user := createTestUser(t, db)
	assistant := createTestAssistant(t, db, user.ID)
	tool := createTestAssistantTool(t, db, assistant.ID)

	// Verify test data was created
	assert.NotNil(t, tool)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		assistantID    string
		toolID         string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Valid tool deletion",
			assistantID:    "1",
			toolID:         "1",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Invalid assistant ID",
			assistantID:    "999",
			toolID:         "1",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:           "Invalid tool ID",
			assistantID:    "1",
			toolID:         "999",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:           "Non-numeric tool ID",
			assistantID:    "1",
			toolID:         "abc",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/assistant/"+tt.assistantID+"/tools/"+tt.toolID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				// Verify tool is deleted
				var count int64
				db.Model(&models.AssistantTool{}).Where("id = ?", tt.toolID).Count(&count)
				assert.Equal(t, int64(0), count)
			}
		})
	}
}

func TestHandlers_handleTestAssistantTool(t *testing.T) {
	db := setupAssistantToolsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantToolsTestRouter(handlers)

	// Create test user, assistant, and tool
	user := createTestUser(t, db)
	assistant := createTestAssistant(t, db, user.ID)
	tool := createTestAssistantTool(t, db, assistant.ID)

	// Verify test data was created
	assert.NotNil(t, tool)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		assistantID    string
		toolID         string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name:        "Valid tool test",
			assistantID: "1",
			toolID:      "1",
			requestBody: map[string]interface{}{
				"args": map[string]interface{}{
					"input": "test input",
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:        "Empty test arguments",
			assistantID: "1",
			toolID:      "1",
			requestBody: map[string]interface{}{
				"args": map[string]interface{}{},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Missing args field",
			assistantID:    "1",
			toolID:         "1",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:        "Invalid assistant ID",
			assistantID: "999",
			toolID:      "1",
			requestBody: map[string]interface{}{
				"args": map[string]interface{}{
					"input": "test",
				},
			},
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:        "Invalid tool ID",
			assistantID: "1",
			toolID:      "999",
			requestBody: map[string]interface{}{
				"args": map[string]interface{}{
					"input": "test",
				},
			},
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/assistant/"+tt.assistantID+"/tools/"+tt.toolID+"/test", bytes.NewBuffer(body))
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
				// Should contain either result or error
				assert.True(t,
					data["result"] != nil || data["error"] != nil,
					"Response should contain either result or error")
			}
		})
	}
}

func BenchmarkHandlers_handleCreateAssistantTool(b *testing.B) {
	db := setupAssistantToolsTestDB(&testing.T{})
	handlers := &Handlers{db: db}
	router := setupAssistantToolsTestRouter(handlers)

	user := createTestUser(&testing.T{}, db)
	assistant := createTestAssistant(&testing.T{}, db, user.ID)

	// Verify test data was created
	_ = assistant // Used for test setup

	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	requestBody := map[string]interface{}{
		"name":        "benchmark_tool",
		"description": "A benchmark test tool",
		"parameters":  `{"type": "object", "properties": {"input": {"type": "string"}}}`,
		"code":        "return 'Hello ' + args.input;",
		"enabled":     true,
	}
	body, _ := json.Marshal(requestBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/assistant/1/tools", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkHandlers_handleListAssistantTools(b *testing.B) {
	db := setupAssistantToolsTestDB(&testing.T{})
	handlers := &Handlers{db: db}
	router := setupAssistantToolsTestRouter(handlers)

	user := createTestUser(&testing.T{}, db)
	assistant := createTestAssistant(&testing.T{}, db, user.ID)
	createTestAssistantTool(&testing.T{}, db, assistant.ID)
	createTestAssistantTool(&testing.T{}, db, assistant.ID)

	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/assistant/1/tools", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func TestHandlers_assistantToolsErrorHandling(t *testing.T) {
	db := setupAssistantToolsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantToolsTestRouter(handlers)

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
			name:           "Invalid JSON in create tool",
			method:         http.MethodPost,
			path:           "/assistant/1/tools",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON in update tool",
			method:         http.MethodPut,
			path:           "/assistant/1/tools/1",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON in test tool",
			method:         http.MethodPost,
			path:           "/assistant/1/tools/1/test",
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

func TestHandlers_assistantToolsCompleteFlow(t *testing.T) {
	db := setupAssistantToolsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantToolsTestRouter(handlers)

	user := createTestUser(t, db)
	assistant := createTestAssistant(t, db, user.ID)

	// Verify test data was created
	_ = assistant // Used for test setup

	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	// Test complete assistant tools lifecycle
	t.Run("Complete assistant tools lifecycle", func(t *testing.T) {
		// 1. Create tool
		createBody := map[string]interface{}{
			"name":        "lifecycle_tool",
			"description": "Testing complete lifecycle",
			"parameters":  `{"type": "object", "properties": {"message": {"type": "string"}}}`,
			"code":        "return 'Processed: ' + args.message;",
			"enabled":     true,
		}
		body, _ := json.Marshal(createBody)
		req := httptest.NewRequest(http.MethodPost, "/assistant/1/tools", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 2. List tools
		req = httptest.NewRequest(http.MethodGet, "/assistant/1/tools", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 3. Update tool
		updateBody := map[string]interface{}{
			"description": "Updated lifecycle tool",
			"code":        "return 'Updated: ' + args.message;",
		}
		body, _ = json.Marshal(updateBody)
		req = httptest.NewRequest(http.MethodPut, "/assistant/1/tools/1", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 4. Test tool
		testBody := map[string]interface{}{
			"args": map[string]interface{}{
				"message": "Hello World",
			},
		}
		body, _ = json.Marshal(testBody)
		req = httptest.NewRequest(http.MethodPost, "/assistant/1/tools/1/test", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 5. Delete tool
		req = httptest.NewRequest(http.MethodDelete, "/assistant/1/tools/1", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHandlers_assistantToolsValidation(t *testing.T) {
	db := setupAssistantToolsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAssistantToolsTestRouter(handlers)

	user := createTestUser(t, db)
	assistant := createTestAssistant(t, db, user.ID)

	// Verify test data was created
	_ = assistant // Used for test setup

	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	// Test tool name validation
	tests := []struct {
		name        string
		toolName    string
		expectError bool
	}{
		{
			name:        "Valid alphanumeric name",
			toolName:    "valid_tool_123",
			expectError: false,
		},
		{
			name:        "Valid name with hyphens",
			toolName:    "valid-tool-name",
			expectError: false,
		},
		{
			name:        "Invalid name with spaces",
			toolName:    "invalid tool name",
			expectError: true,
		},
		{
			name:        "Invalid name with special chars",
			toolName:    "invalid@tool!",
			expectError: true,
		},
		{
			name:        "Empty name",
			toolName:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestBody := map[string]interface{}{
				"name":        tt.toolName,
				"description": "Test tool validation",
				"parameters":  `{"type": "object"}`,
			}
			body, _ := json.Marshal(requestBody)
			req := httptest.NewRequest(http.MethodPost, "/assistant/1/tools", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if tt.expectError {
				assert.Equal(t, http.StatusBadRequest, w.Code)
			} else {
				assert.Equal(t, http.StatusOK, w.Code)
			}
		})
	}
}
