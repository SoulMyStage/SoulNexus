package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlers_GetSearchStatus(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		expectedStatus int
	}{
		{
			name:           "successful get search status",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/system/search/status", nil)

			handlers.GetSearchStatus(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, "Get search status", response["msg"])

			data, ok := response["data"].(map[string]interface{})
			assert.True(t, ok)
			assert.Contains(t, data, "enabled")
			assert.Contains(t, data, "searchPath")
			assert.Contains(t, data, "batchSize")
			assert.Contains(t, data, "schedule")
		})
	}
}

func TestHandlers_UpdateSearchConfig(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handlers := &Handlers{db: db}
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
		requestBody    interface{}
		setupAuth      func(*gin.Context)
		expectedStatus int
	}{
		{
			name: "successful update search config",
			requestBody: map[string]interface{}{
				"enabled":   true,
				"path":      "/test/search",
				"batchSize": 50,
				"schedule":  "0 */6 * * *",
			},
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "partial update search config",
			requestBody: map[string]interface{}{
				"enabled": false,
			},
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "unauthorized access",
			requestBody:    map[string]interface{}{"enabled": true},
			setupAuth:      func(c *gin.Context) {},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "invalid request body",
			requestBody: "invalid json",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("PUT", "/api/system/search/config", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			tt.setupAuth(c)
			handlers.UpdateSearchConfig(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestHandlers_EnableSearch(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handlers := &Handlers{db: db}
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
		setupAuth      func(*gin.Context)
		expectedStatus int
	}{
		{
			name: "successful enable search",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "unauthorized access",
			setupAuth:      func(c *gin.Context) {},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/system/search/enable", nil)

			tt.setupAuth(c)
			handlers.EnableSearch(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestHandlers_DisableSearch(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handlers := &Handlers{db: db}
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
		setupAuth      func(*gin.Context)
		expectedStatus int
	}{
		{
			name: "successful disable search",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "unauthorized access",
			setupAuth:      func(c *gin.Context) {},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/system/search/disable", nil)

			tt.setupAuth(c)
			handlers.DisableSearch(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// Edge case tests
func TestHandlers_SearchConfig_EdgeCases(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	t.Run("empty request body", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/api/system/search/config", bytes.NewBuffer([]byte("{}")))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)

		handlers.UpdateSearchConfig(c)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("null values in request", func(t *testing.T) {
		requestBody := map[string]interface{}{
			"enabled":   nil,
			"path":      nil,
			"batchSize": nil,
			"schedule":  nil,
		}
		body, _ := json.Marshal(requestBody)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/api/system/search/config", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)

		handlers.UpdateSearchConfig(c)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid data types", func(t *testing.T) {
		requestBody := map[string]interface{}{
			"enabled":   "not_a_boolean",
			"batchSize": "not_a_number",
		}
		body, _ := json.Marshal(requestBody)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/api/system/search/config", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)

		handlers.UpdateSearchConfig(c)

		// Should still return OK as the handler is lenient
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// Benchmark tests
func BenchmarkHandlers_GetSearchStatus(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/system/search/status", nil)

		handlers.GetSearchStatus(c)
	}
}

func BenchmarkHandlers_UpdateSearchConfig(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	db.Create(user)

	requestBody := map[string]interface{}{
		"enabled":   true,
		"path":      "/benchmark/search",
		"batchSize": 100,
	}
	body, _ := json.Marshal(requestBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/api/system/search/config", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)

		handlers.UpdateSearchConfig(c)
	}
}

func BenchmarkHandlers_EnableSearch(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	db.Create(user)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/system/search/enable", nil)
		c.Set("user", user)

		handlers.EnableSearch(c)
	}
}

// Integration tests
func TestHandlers_SearchConfigIntegration(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Test complete workflow: get status -> update config -> enable -> disable
	t.Run("complete search config workflow", func(t *testing.T) {
		// 1. Get initial status
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/system/search/status", nil)
		handlers.GetSearchStatus(c)
		assert.Equal(t, http.StatusOK, w.Code)

		// 2. Update configuration
		requestBody := map[string]interface{}{
			"enabled":   true,
			"path":      "/integration/test",
			"batchSize": 75,
			"schedule":  "0 */4 * * *",
		}
		body, _ := json.Marshal(requestBody)

		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/api/system/search/config", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)
		handlers.UpdateSearchConfig(c)
		assert.Equal(t, http.StatusOK, w.Code)

		// 3. Enable search
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/system/search/enable", nil)
		c.Set("user", user)
		handlers.EnableSearch(c)
		assert.Equal(t, http.StatusOK, w.Code)

		// 4. Disable search
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/system/search/disable", nil)
		c.Set("user", user)
		handlers.DisableSearch(c)
		assert.Equal(t, http.StatusOK, w.Code)

		// 5. Verify final status
		w = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/system/search/status", nil)
		handlers.GetSearchStatus(c)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
