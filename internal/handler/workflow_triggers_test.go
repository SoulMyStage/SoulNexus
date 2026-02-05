package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlers_ExecutePublicWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test workflow with API trigger
	workflow := models.WorkflowDefinition{
		UserID:      1,
		Name:        "Public API Workflow",
		Slug:        "public-api-workflow",
		Description: "Test public API workflow",
		Status:      "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
				{ID: "end", Name: "End", Type: "end"},
			},
			Edges: []models.WorkflowEdgeSchema{
				{ID: "edge1", Source: "start", Target: "end"},
			},
		},
		Triggers: models.JSONMap{
			"api": map[string]interface{}{
				"enabled": true,
				"public":  true,
				"apiKey":  "test-api-key",
			},
		},
	}
	require.NoError(t, db.Create(&workflow).Error)

	// Create workflow with private API trigger
	privateWorkflow := models.WorkflowDefinition{
		UserID:      1,
		Name:        "Private API Workflow",
		Slug:        "private-api-workflow",
		Description: "Test private API workflow",
		Status:      "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
			},
		},
		Triggers: models.JSONMap{
			"api": map[string]interface{}{
				"enabled": true,
				"public":  false,
				"apiKey":  "private-api-key",
			},
		},
	}
	require.NoError(t, db.Create(&privateWorkflow).Error)

	// Create workflow without API trigger
	noAPIWorkflow := models.WorkflowDefinition{
		UserID:      1,
		Name:        "No API Workflow",
		Slug:        "no-api-workflow",
		Description: "Test workflow without API trigger",
		Status:      "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
			},
		},
		Triggers: models.JSONMap{
			"api": map[string]interface{}{
				"enabled": false,
			},
		},
	}
	require.NoError(t, db.Create(&noAPIWorkflow).Error)

	tests := []struct {
		name         string
		slug         string
		apiKey       string
		request      map[string]interface{}
		expectedCode int
	}{
		{
			name:   "Valid public API execution",
			slug:   "public-api-workflow",
			apiKey: "test-api-key",
			request: map[string]interface{}{
				"parameters": map[string]interface{}{
					"input": "test value",
				},
			},
			expectedCode: 200,
		},
		{
			name:   "Valid public API execution without API key",
			slug:   "public-api-workflow",
			apiKey: "",
			request: map[string]interface{}{
				"parameters": map[string]interface{}{
					"input": "test value",
				},
			},
			expectedCode: 500, // Should fail due to API key mismatch
		},
		{
			name:   "Invalid API key",
			slug:   "public-api-workflow",
			apiKey: "wrong-api-key",
			request: map[string]interface{}{
				"parameters": map[string]interface{}{
					"input": "test value",
				},
			},
			expectedCode: 500,
		},
		{
			name:         "Non-existent workflow",
			slug:         "non-existent-workflow",
			apiKey:       "test-api-key",
			request:      map[string]interface{}{},
			expectedCode: 500,
		},
		{
			name:         "Private workflow (not public)",
			slug:         "private-api-workflow",
			apiKey:       "private-api-key",
			request:      map[string]interface{}{},
			expectedCode: 500,
		},
		{
			name:         "Workflow without API trigger",
			slug:         "no-api-workflow",
			apiKey:       "",
			request:      map[string]interface{}{},
			expectedCode: 500,
		},
		{
			name:         "Empty slug",
			slug:         "",
			apiKey:       "",
			request:      map[string]interface{}{},
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "slug", Value: tt.slug}}

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/public/workflows/"+tt.slug+"/execute", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}
			c.Request = req

			h.ExecutePublicWorkflow(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

func TestHandlers_WebhookTriggerWorkflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test workflow with webhook trigger
	workflow := models.WorkflowDefinition{
		UserID:      1,
		Name:        "Webhook Workflow",
		Slug:        "webhook-workflow",
		Description: "Test webhook workflow",
		Status:      "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
				{ID: "end", Name: "End", Type: "end"},
			},
			Edges: []models.WorkflowEdgeSchema{
				{ID: "edge1", Source: "start", Target: "end"},
			},
		},
		Triggers: models.JSONMap{
			"webhook": map[string]interface{}{
				"enabled": true,
				"secret":  "webhook-secret",
				"method":  "POST",
			},
		},
	}
	require.NoError(t, db.Create(&workflow).Error)

	// Create workflow without webhook trigger
	noWebhookWorkflow := models.WorkflowDefinition{
		UserID:      1,
		Name:        "No Webhook Workflow",
		Slug:        "no-webhook-workflow",
		Description: "Test workflow without webhook trigger",
		Status:      "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
			},
		},
		Triggers: models.JSONMap{
			"webhook": map[string]interface{}{
				"enabled": false,
			},
		},
	}
	require.NoError(t, db.Create(&noWebhookWorkflow).Error)

	tests := []struct {
		name         string
		slug         string
		signature    string
		request      map[string]interface{}
		contentType  string
		expectedCode int
	}{
		{
			name: "Valid webhook execution with JSON",
			slug: "webhook-workflow",
			request: map[string]interface{}{
				"event": "test_event",
				"data":  "test_data",
			},
			contentType:  "application/json",
			expectedCode: 200,
		},
		{
			name:         "Valid webhook execution with form data",
			slug:         "webhook-workflow",
			request:      map[string]interface{}{},
			contentType:  "application/x-www-form-urlencoded",
			expectedCode: 200,
		},
		{
			name: "Webhook with signature (not implemented validation)",
			slug: "webhook-workflow",
			request: map[string]interface{}{
				"event": "signed_event",
			},
			signature:    "sha256=test-signature",
			contentType:  "application/json",
			expectedCode: 500, // Will fail signature validation (not implemented)
		},
		{
			name:         "Non-existent workflow",
			slug:         "non-existent-webhook",
			request:      map[string]interface{}{},
			contentType:  "application/json",
			expectedCode: 500,
		},
		{
			name:         "Workflow without webhook trigger",
			slug:         "no-webhook-workflow",
			request:      map[string]interface{}{},
			contentType:  "application/json",
			expectedCode: 500,
		},
		{
			name:         "Empty slug",
			slug:         "",
			request:      map[string]interface{}{},
			contentType:  "application/json",
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "slug", Value: tt.slug}}

			var req *http.Request
			if tt.contentType == "application/json" {
				body, _ := json.Marshal(tt.request)
				req = httptest.NewRequest("POST", "/public/workflows/webhook/"+tt.slug, bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				// Simulate form data
				req = httptest.NewRequest("POST", "/public/workflows/webhook/"+tt.slug, strings.NewReader("key=value"))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}

			if tt.signature != "" {
				req.Header.Set("X-Webhook-Signature", tt.signature)
			}

			c.Request = req

			h.WebhookTriggerWorkflow(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}
func TestHandlers_RegisterPublicWorkflowRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create a test router group
	router := gin.New()
	api := router.Group("/api")

	// Register the public workflow routes
	h.RegisterPublicWorkflowRoutes(api)

	// Test that routes are registered by making requests
	t.Run("Public workflow execute route exists", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/public/workflows/test-slug/execute", nil)
		router.ServeHTTP(w, req)

		// Should not return 404 (route exists)
		assert.NotEqual(t, 404, w.Code)
	})

	t.Run("Webhook trigger route exists", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/public/workflows/webhook/test-slug", nil)
		router.ServeHTTP(w, req)

		// Should not return 404 (route exists)
		assert.NotEqual(t, 404, w.Code)
	})
}

func TestGenerateAPIKey(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "Generate valid API key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiKey, err := GenerateAPIKey()

			assert.NoError(t, err)
			assert.NotEmpty(t, apiKey)
			assert.Equal(t, 64, len(apiKey)) // 32 bytes = 64 hex characters

			// Verify it's valid hex
			for _, char := range apiKey {
				assert.True(t, (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f'))
			}
		})
	}

	t.Run("Generate multiple unique keys", func(t *testing.T) {
		keys := make(map[string]bool)

		for i := 0; i < 100; i++ {
			key, err := GenerateAPIKey()
			assert.NoError(t, err)
			assert.False(t, keys[key], "Generated duplicate API key")
			keys[key] = true
		}
	})
}

// Test error handling scenarios
func TestWorkflowTriggerErrorHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	t.Run("Invalid JSON in ExecutePublicWorkflow", func(t *testing.T) {
		// Create test workflow
		workflow := models.WorkflowDefinition{
			UserID: 1,
			Name:   "Test Workflow",
			Slug:   "test-workflow",
			Status: "active",
			Definition: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
				},
			},
			Triggers: models.JSONMap{
				"api": map[string]interface{}{
					"enabled": true,
					"public":  true,
				},
			},
		}
		db.Create(&workflow)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "slug", Value: "test-workflow"}}

		req := httptest.NewRequest("POST", "/public/workflows/test-workflow/execute",
			strings.NewReader("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.ExecutePublicWorkflow(c)

		// Should handle invalid JSON gracefully
		assert.Equal(t, 200, w.Code) // EOF error is handled
	})

	t.Run("Invalid trigger config in ExecutePublicWorkflow", func(t *testing.T) {
		// Create workflow with invalid trigger config
		workflow := models.WorkflowDefinition{
			UserID: 1,
			Name:   "Invalid Trigger Workflow",
			Slug:   "invalid-trigger-workflow",
			Status: "active",
			Definition: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
				},
			},
			Triggers: models.JSONMap{
				"invalid": "config",
			},
		}
		db.Create(&workflow)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "slug", Value: "invalid-trigger-workflow"}}

		req := httptest.NewRequest("POST", "/public/workflows/invalid-trigger-workflow/execute", nil)
		c.Request = req

		h.ExecutePublicWorkflow(c)

		assert.Equal(t, 500, w.Code)
	})

	t.Run("Database error simulation", func(t *testing.T) {
		// Close database to simulate error
		sqlDB, _ := db.DB()
		sqlDB.Close()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "slug", Value: "any-slug"}}

		req := httptest.NewRequest("POST", "/public/workflows/any-slug/execute", nil)
		c.Request = req

		h.ExecutePublicWorkflow(c)

		assert.Equal(t, 500, w.Code)
	})
}

// Test different parameter formats
func TestWorkflowTriggerParameterFormats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test workflow
	workflow := models.WorkflowDefinition{
		UserID: 1,
		Name:   "Parameter Test Workflow",
		Slug:   "parameter-test-workflow",
		Status: "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
				{ID: "end", Name: "End", Type: "end"},
			},
			Edges: []models.WorkflowEdgeSchema{
				{ID: "edge1", Source: "start", Target: "end"},
			},
		},
		Triggers: models.JSONMap{
			"api": map[string]interface{}{
				"enabled": true,
				"public":  true,
			},
		},
	}
	require.NoError(t, db.Create(&workflow).Error)

	tests := []struct {
		name         string
		request      interface{}
		expectedCode int
	}{
		{
			name: "Complex nested parameters",
			request: map[string]interface{}{
				"parameters": map[string]interface{}{
					"user": map[string]interface{}{
						"name":  "John Doe",
						"email": "john@example.com",
						"preferences": map[string]interface{}{
							"theme":         "dark",
							"notifications": []string{"email", "sms"},
						},
					},
					"metadata": map[string]interface{}{
						"timestamp": "2023-01-01T00:00:00Z",
						"version":   1.0,
					},
				},
			},
			expectedCode: 200,
		},
		{
			name: "Array parameters",
			request: map[string]interface{}{
				"parameters": map[string]interface{}{
					"items": []interface{}{
						"item1",
						"item2",
						map[string]interface{}{
							"name":  "item3",
							"value": 42,
						},
					},
				},
			},
			expectedCode: 200,
		},
		{
			name: "Empty parameters",
			request: map[string]interface{}{
				"parameters": map[string]interface{}{},
			},
			expectedCode: 200,
		},
		{
			name:         "No parameters field",
			request:      map[string]interface{}{},
			expectedCode: 200,
		},
		{
			name:         "Null request",
			request:      nil,
			expectedCode: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "slug", Value: "parameter-test-workflow"}}

			var body []byte
			if tt.request != nil {
				body, _ = json.Marshal(tt.request)
			}

			req := httptest.NewRequest("POST", "/public/workflows/parameter-test-workflow/execute",
				bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.ExecutePublicWorkflow(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

// Test webhook with different content types and methods
func TestWebhookTriggerContentTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test workflow
	workflow := models.WorkflowDefinition{
		UserID: 1,
		Name:   "Webhook Content Type Test",
		Slug:   "webhook-content-test",
		Status: "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
			},
		},
		Triggers: models.JSONMap{
			"webhook": map[string]interface{}{
				"enabled": true,
				"method":  "POST",
			},
		},
	}
	require.NoError(t, db.Create(&workflow).Error)

	tests := []struct {
		name         string
		contentType  string
		body         string
		expectedCode int
	}{
		{
			name:         "JSON content type",
			contentType:  "application/json",
			body:         `{"key": "value"}`,
			expectedCode: 200,
		},
		{
			name:         "Form data content type",
			contentType:  "application/x-www-form-urlencoded",
			body:         "key1=value1&key2=value2",
			expectedCode: 200,
		},
		{
			name:         "Plain text content type",
			contentType:  "text/plain",
			body:         "plain text data",
			expectedCode: 200,
		},
		{
			name:         "XML content type",
			contentType:  "application/xml",
			body:         "<root><key>value</key></root>",
			expectedCode: 200,
		},
		{
			name:         "Empty body",
			contentType:  "application/json",
			body:         "",
			expectedCode: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "slug", Value: "webhook-content-test"}}

			req := httptest.NewRequest("POST", "/public/workflows/webhook/webhook-content-test",
				strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			c.Request = req

			h.WebhookTriggerWorkflow(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

// Benchmark tests
func BenchmarkHandlers_ExecutePublicWorkflow(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test workflow
	workflow := models.WorkflowDefinition{
		UserID: 1,
		Name:   "Benchmark Workflow",
		Slug:   "benchmark-workflow",
		Status: "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
				{ID: "end", Name: "End", Type: "end"},
			},
			Edges: []models.WorkflowEdgeSchema{
				{ID: "edge1", Source: "start", Target: "end"},
			},
		},
		Triggers: models.JSONMap{
			"api": map[string]interface{}{
				"enabled": true,
				"public":  true,
			},
		},
	}
	db.Create(&workflow)

	request := map[string]interface{}{
		"parameters": map[string]interface{}{
			"input": "benchmark test",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "slug", Value: "benchmark-workflow"}}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/public/workflows/benchmark-workflow/execute",
			bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.ExecutePublicWorkflow(c)
	}
}

func BenchmarkHandlers_WebhookTriggerWorkflow(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test workflow
	workflow := models.WorkflowDefinition{
		UserID: 1,
		Name:   "Benchmark Webhook",
		Slug:   "benchmark-webhook",
		Status: "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
			},
		},
		Triggers: models.JSONMap{
			"webhook": map[string]interface{}{
				"enabled": true,
			},
		},
	}
	db.Create(&workflow)

	request := map[string]interface{}{
		"event": "benchmark_event",
		"data":  "benchmark data",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "slug", Value: "benchmark-webhook"}}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/public/workflows/webhook/benchmark-webhook",
			bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.WebhookTriggerWorkflow(c)
	}
}

// Edge case tests
func TestWorkflowTriggerEdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	t.Run("Very long slug", func(t *testing.T) {
		longSlug := strings.Repeat("a", 1000)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "slug", Value: longSlug}}

		req := httptest.NewRequest("POST", "/public/workflows/"+longSlug+"/execute", nil)
		c.Request = req

		h.ExecutePublicWorkflow(c)

		assert.Equal(t, 500, w.Code) // Should handle gracefully
	})

	t.Run("Special characters in slug", func(t *testing.T) {
		specialSlug := "test-workflow-with-@#$%^&*()-symbols"

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "slug", Value: specialSlug}}

		req := httptest.NewRequest("POST", "/public/workflows/"+specialSlug+"/execute", nil)
		c.Request = req

		h.ExecutePublicWorkflow(c)

		assert.Equal(t, 500, w.Code) // Should handle gracefully
	})

	t.Run("Very large request body", func(t *testing.T) {
		// Create workflow
		workflow := models.WorkflowDefinition{
			UserID: 1,
			Name:   "Large Body Test",
			Slug:   "large-body-test",
			Status: "active",
			Definition: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
				},
			},
			Triggers: models.JSONMap{
				"api": map[string]interface{}{
					"enabled": true,
					"public":  true,
				},
			},
		}
		db.Create(&workflow)

		// Create large request
		largeData := strings.Repeat("x", 10000)
		request := map[string]interface{}{
			"parameters": map[string]interface{}{
				"largeData": largeData,
			},
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "slug", Value: "large-body-test"}}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/public/workflows/large-body-test/execute",
			bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.ExecutePublicWorkflow(c)

		assert.Equal(t, 200, w.Code) // Should handle large bodies
	})
}
