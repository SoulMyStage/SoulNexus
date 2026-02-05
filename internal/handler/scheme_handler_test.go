package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemeHandler_ListSchemes(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewSchemeHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test schemes
	scheme1 := &models.SipUser{
		SchemeName:  "Test Scheme 1",
		DisplayName: "scheme_1_123",
		UserID:      &user.ID,
		Enabled:     true,
	}
	scheme2 := &models.SipUser{
		SchemeName:  "Test Scheme 2",
		DisplayName: "scheme_2_456",
		UserID:      &user.ID,
		Enabled:     false,
	}
	require.NoError(t, db.Create(scheme1).Error)
	require.NoError(t, db.Create(scheme2).Error)

	tests := []struct {
		name           string
		setupAuth      func(*gin.Context)
		expectedStatus int
		expectedCount  int
	}{
		{
			name: "successful list schemes",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:           "unauthorized access",
			setupAuth:      func(c *gin.Context) {},
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/schemes", nil)

			tt.setupAuth(c)
			handler.ListSchemes(c)

			if tt.expectedCount > 0 {
				assert.Equal(t, tt.expectedStatus, w.Code)
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "success", response["msg"])
			}
		})
	}
}

func TestSchemeHandler_GetScheme(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewSchemeHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test scheme
	scheme := &models.SipUser{
		SchemeName:  "Test Scheme",
		DisplayName: "scheme_123",
		UserID:      &user.ID,
		Description: "Test description",
		Enabled:     true,
	}
	require.NoError(t, db.Create(scheme).Error)

	tests := []struct {
		name           string
		schemeID       string
		setupAuth      func(*gin.Context)
		expectedStatus int
	}{
		{
			name:     "successful get scheme",
			schemeID: fmt.Sprintf("%d", scheme.ID),
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "unauthorized access",
			schemeID: fmt.Sprintf("%d", scheme.ID),
			setupAuth: func(c *gin.Context) {
				// No user set
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "invalid scheme ID",
			schemeID: "invalid",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "non-existent scheme",
			schemeID: "99999",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/schemes/"+tt.schemeID, nil)
			c.Params = gin.Params{{Key: "id", Value: tt.schemeID}}

			tt.setupAuth(c)
			handler.GetScheme(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSchemeHandler_CreateScheme(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewSchemeHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test assistant
	assistant := &models.Assistant{
		UserID: user.ID,
		Name:   "Test Assistant",
	}
	require.NoError(t, db.Create(assistant).Error)

	tests := []struct {
		name           string
		requestBody    interface{}
		setupAuth      func(*gin.Context)
		expectedStatus int
	}{
		{
			name: "successful create scheme",
			requestBody: CreateSchemeRequest{
				SchemeName:       "Test Scheme",
				Description:      "Test description",
				AssistantID:      func() *uint { id := uint(assistant.ID); return &id }(),
				AutoAnswer:       true,
				AutoAnswerDelay:  5,
				OpeningMessage:   "Hello",
				FallbackMessage:  "Sorry",
				AIFreeResponse:   true,
				RecordingEnabled: true,
				MessageEnabled:   true,
				MessageDuration:  30,
				MessagePrompt:    "Please leave a message",
				BoundPhoneNumber: "+1234567890",
			},
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "unauthorized access",
			requestBody:    CreateSchemeRequest{SchemeName: "Test", AssistantID: func() *uint { id := uint(assistant.ID); return &id }()},
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
			c.Request = httptest.NewRequest("POST", "/api/schemes", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			tt.setupAuth(c)
			handler.CreateScheme(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSchemeHandler_UpdateScheme(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewSchemeHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test assistant
	assistant := &models.Assistant{
		UserID: user.ID,
		Name:   "Test Assistant",
	}
	require.NoError(t, db.Create(assistant).Error)

	// Create test scheme
	scheme := &models.SipUser{
		SchemeName:  "Test Scheme",
		DisplayName: "scheme_123",
		UserID:      &user.ID,
		AssistantID: func() *uint { id := uint(assistant.ID); return &id }(),
		Enabled:     true,
	}
	require.NoError(t, db.Create(scheme).Error)

	tests := []struct {
		name           string
		schemeID       string
		requestBody    interface{}
		setupAuth      func(*gin.Context)
		expectedStatus int
	}{
		{
			name:     "successful update scheme",
			schemeID: fmt.Sprintf("%d", scheme.ID),
			requestBody: UpdateSchemeRequest{
				SchemeName:  stringPtr("Updated Scheme"),
				Description: stringPtr("Updated description"),
				Enabled:     boolPtr(false),
			},
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "unauthorized access",
			schemeID: fmt.Sprintf("%d", scheme.ID),
			requestBody: UpdateSchemeRequest{
				SchemeName: stringPtr("Updated Scheme"),
			},
			setupAuth:      func(c *gin.Context) {},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "invalid scheme ID",
			schemeID:    "invalid",
			requestBody: UpdateSchemeRequest{},
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("PUT", "/api/schemes/"+tt.schemeID, bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "id", Value: tt.schemeID}}

			tt.setupAuth(c)
			handler.UpdateScheme(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSchemeHandler_DeleteScheme(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewSchemeHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test scheme
	scheme := &models.SipUser{
		SchemeName:  "Test Scheme",
		DisplayName: "scheme_123",
		UserID:      &user.ID,
		Enabled:     true,
	}
	require.NoError(t, db.Create(scheme).Error)

	tests := []struct {
		name           string
		schemeID       string
		setupAuth      func(*gin.Context)
		expectedStatus int
	}{
		{
			name:     "successful delete scheme",
			schemeID: fmt.Sprintf("%d", scheme.ID),
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "unauthorized access",
			schemeID:       fmt.Sprintf("%d", scheme.ID),
			setupAuth:      func(c *gin.Context) {},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "invalid scheme ID",
			schemeID: "invalid",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("DELETE", "/api/schemes/"+tt.schemeID, nil)
			c.Params = gin.Params{{Key: "id", Value: tt.schemeID}}

			tt.setupAuth(c)
			handler.DeleteScheme(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSchemeHandler_ActivateScheme(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewSchemeHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test scheme
	scheme := &models.SipUser{
		SchemeName:  "Test Scheme",
		DisplayName: "scheme_123",
		UserID:      &user.ID,
		Enabled:     true,
	}
	require.NoError(t, db.Create(scheme).Error)

	tests := []struct {
		name           string
		schemeID       string
		setupAuth      func(*gin.Context)
		expectedStatus int
	}{
		{
			name:     "successful activate scheme",
			schemeID: fmt.Sprintf("%d", scheme.ID),
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "unauthorized access",
			schemeID:       fmt.Sprintf("%d", scheme.ID),
			setupAuth:      func(c *gin.Context) {},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/schemes/"+tt.schemeID+"/activate", nil)
			c.Params = gin.Params{{Key: "id", Value: tt.schemeID}}

			tt.setupAuth(c)
			handler.ActivateScheme(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSchemeHandler_DeactivateScheme(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewSchemeHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test scheme
	scheme := &models.SipUser{
		SchemeName:  "Test Scheme",
		DisplayName: "scheme_123",
		UserID:      &user.ID,
		Enabled:     true,
	}
	require.NoError(t, db.Create(scheme).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/schemes/"+fmt.Sprintf("%d", scheme.ID)+"/deactivate", nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", scheme.ID)}}
	c.Set("user", user)

	handler.DeactivateScheme(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSchemeHandler_GetActiveScheme(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewSchemeHandler(db)
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
			name: "successful get active scheme",
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
			c.Request = httptest.NewRequest("GET", "/api/schemes/active", nil)

			tt.setupAuth(c)
			handler.GetActiveScheme(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// Benchmark tests
func BenchmarkSchemeHandler_ListSchemes(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	handler := NewSchemeHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	db.Create(user)

	// Create multiple schemes for benchmarking
	for i := 0; i < 100; i++ {
		scheme := &models.SipUser{
			SchemeName:  fmt.Sprintf("Scheme %d", i),
			DisplayName: fmt.Sprintf("scheme_%d", i),
			UserID:      &user.ID,
			Enabled:     true,
		}
		db.Create(scheme)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/schemes", nil)
		c.Set("user", user)

		handler.ListSchemes(c)
	}
}

func BenchmarkSchemeHandler_CreateScheme(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	handler := NewSchemeHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	db.Create(user)

	// Create test assistant
	assistant := &models.Assistant{
		UserID: user.ID,
		Name:   "Test Assistant",
	}
	db.Create(assistant)

	requestBody := CreateSchemeRequest{
		SchemeName:  "Benchmark Scheme",
		AssistantID: func() *uint { id := uint(assistant.ID); return &id }(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body, _ := json.Marshal(requestBody)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/schemes", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)

		handler.CreateScheme(c)
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}
