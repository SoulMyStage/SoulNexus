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
	"gorm.io/gorm"
)

func setupCredentialsTestDB(t *testing.T) (*gorm.DB, func()) {
	db := setupTestDB(t)

	// Auto migrate tables
	err := db.AutoMigrate(
		&models.User{},
		&models.UserCredential{},
	)
	assert.NoError(t, err)

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS users")
		db.Exec("DROP TABLE IF EXISTS user_credentials")
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}

	return db, cleanup
}

func TestHandlers_handleCreateCredential(t *testing.T) {
	db, cleanup := setupCredentialsTestDB(t)
	defer cleanup()

	// Create test user
	user := &models.User{
		Email:       "credentials@test.com",
		Password:    "password123",
		DisplayName: "creduser",
	}
	db.Create(user)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		request    models.UserCredentialRequest
		wantStatus int
		wantError  bool
	}{
		{
			name: "Valid credential creation",
			request: models.UserCredentialRequest{
				Name: "Test Credential",
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name: "Empty name",
			request: models.UserCredentialRequest{
				Name: "",
			},
			wantStatus: http.StatusOK,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, _ := json.Marshal(tt.request)
			c.Request = httptest.NewRequest("POST", "/api/credentials", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", user)

			h.handleCreateCredential(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError {
				assert.NotEqual(t, "success", resp["status"])
			} else {
				assert.Equal(t, "success", resp["status"])

				// Verify response contains required fields
				data := resp["data"].(map[string]interface{})
				assert.NotEmpty(t, data["apiKey"])
				assert.NotEmpty(t, data["apiSecret"])
				assert.Equal(t, tt.request.Name, data["name"])
			}
		})
	}
}

func TestHandlers_handleGetCredential(t *testing.T) {
	db, cleanup := setupCredentialsTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "credentials@test.com",
		Password:    "password123",
		DisplayName: "creduser",
	}
	db.Create(user)

	// Create test credentials
	credentials := []models.UserCredential{
		{
			UserID:    user.ID,
			APIKey:    "test-key-1",
			APISecret: "test-secret-1",
			Name:      "Credential 1",
		},
		{
			UserID:    user.ID,
			APIKey:    "test-key-2",
			APISecret: "test-secret-2",
			Name:      "Credential 2",
		},
	}

	for _, cred := range credentials {
		db.Create(&cred)
	}

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/api/credentials", nil)
	c.Set("user", user)

	h.handleGetCredential(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])

	// Verify response contains credentials
	data := resp["data"].([]interface{})
	assert.Equal(t, 2, len(data))
}

func TestHandlers_handleDeleteCredential(t *testing.T) {
	db, cleanup := setupCredentialsTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "credentials@test.com",
		Password:    "password123",
		DisplayName: "creduser",
	}
	db.Create(user)

	credential := &models.UserCredential{
		UserID:    user.ID,
		APIKey:    "test-key",
		APISecret: "test-secret",
		Name:      "Test Credential",
	}
	db.Create(credential)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		credentialID string
		wantStatus   int
		wantError    bool
	}{
		{
			name:         "Valid credential deletion",
			credentialID: fmt.Sprintf("%d", credential.ID),
			wantStatus:   http.StatusOK,
			wantError:    false,
		},
		{
			name:         "Invalid credential ID",
			credentialID: "invalid",
			wantStatus:   http.StatusOK,
			wantError:    true,
		},
		{
			name:         "Non-existent credential ID",
			credentialID: "99999",
			wantStatus:   http.StatusOK,
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("DELETE", fmt.Sprintf("/api/credentials/%s", tt.credentialID), nil)
			c.Params = gin.Params{{Key: "id", Value: tt.credentialID}}
			c.Set("user", user)

			h.handleDeleteCredential(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError {
				assert.Equal(t, "fail", resp["status"])
			} else {
				assert.Equal(t, "success", resp["status"])

				// Verify deletion
				var deleted models.UserCredential
				err := db.First(&deleted, credential.ID).Error
				assert.Error(t, err)
				assert.Equal(t, gorm.ErrRecordNotFound, err)
			}
		})
	}
}

func TestHandlers_handleCreateCredential_UnauthorizedUser(t *testing.T) {
	db, cleanup := setupCredentialsTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	request := models.UserCredentialRequest{
		Name: "Test Credential",
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(request)
	c.Request = httptest.NewRequest("POST", "/api/credentials", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	// No user set

	h.handleCreateCredential(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "fail", resp["status"])
	assert.Equal(t, "User is not logged in.", resp["message"])
}

func TestHandlers_handleDeleteCredential_UnauthorizedUser(t *testing.T) {
	db, cleanup := setupCredentialsTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "credentials@test.com",
		Password:    "password123",
		DisplayName: "creduser",
	}
	db.Create(user)

	credential := &models.UserCredential{
		UserID:    user.ID,
		APIKey:    "test-key",
		APISecret: "test-secret",
		Name:      "Test Credential",
	}
	db.Create(credential)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("DELETE", fmt.Sprintf("/api/credentials/%d", credential.ID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", credential.ID)}}
	// No user set

	h.handleDeleteCredential(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "fail", resp["status"])
	assert.Equal(t, "User is not logged in.", resp["message"])
}

func TestHandlers_handleDeleteCredential_WrongUser(t *testing.T) {
	db, cleanup := setupCredentialsTestDB(t)
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

	// Credential belongs to user1
	credential := &models.UserCredential{
		UserID:    user1.ID,
		APIKey:    "test-key",
		APISecret: "test-secret",
		Name:      "Test Credential",
	}
	db.Create(credential)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Try to delete with user2
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("DELETE", fmt.Sprintf("/api/credentials/%d", credential.ID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", credential.ID)}}
	c.Set("user", user2) // Wrong user

	h.handleDeleteCredential(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "fail", resp["status"])
	assert.Equal(t, "Failed to delete credential", resp["message"])

	// Verify credential still exists
	var stillExists models.UserCredential
	err := db.First(&stillExists, credential.ID).Error
	assert.NoError(t, err)
}

func TestHandlers_handleGetCredential_EmptyList(t *testing.T) {
	db, cleanup := setupCredentialsTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "credentials@test.com",
		Password:    "password123",
		DisplayName: "creduser",
	}
	db.Create(user)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/api/credentials", nil)
	c.Set("user", user)

	h.handleGetCredential(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])

	// Should return empty list
	data := resp["data"].([]interface{})
	assert.Equal(t, 0, len(data))
}

func TestHandlers_handleCreateCredential_InvalidJSON(t *testing.T) {
	db, cleanup := setupCredentialsTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "credentials@test.com",
		Password:    "password123",
		DisplayName: "creduser",
	}
	db.Create(user)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Invalid JSON
	c.Request = httptest.NewRequest("POST", "/api/credentials", bytes.NewBuffer([]byte("invalid json")))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", user)

	h.handleCreateCredential(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "fail", resp["status"])
	assert.Equal(t, "Invalid request", resp["message"])
}

// Benchmark tests
func BenchmarkHandlers_handleCreateCredential(b *testing.B) {
	db, cleanup := setupCredentialsTestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "credentials@test.com",
		Password:    "password123",
		DisplayName: "creduser",
	}
	db.Create(user)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	request := models.UserCredentialRequest{
		Name: "Benchmark Credential",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body, _ := json.Marshal(request)
		c.Request = httptest.NewRequest("POST", "/api/credentials", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)

		h.handleCreateCredential(c)
	}
}

func BenchmarkHandlers_handleGetCredential(b *testing.B) {
	db, cleanup := setupCredentialsTestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "credentials@test.com",
		Password:    "password123",
		DisplayName: "creduser",
	}
	db.Create(user)

	// Create test data
	for i := 0; i < 10; i++ {
		credential := &models.UserCredential{
			UserID:    user.ID,
			APIKey:    fmt.Sprintf("test-key-%d", i),
			APISecret: fmt.Sprintf("test-secret-%d", i),
			Name:      fmt.Sprintf("Credential %d", i),
		}
		db.Create(credential)
	}

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", "/api/credentials", nil)
		c.Set("user", user)

		h.handleGetCredential(c)
	}
}

func BenchmarkHandlers_handleDeleteCredential(b *testing.B) {
	db, cleanup := setupCredentialsTestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "credentials@test.com",
		Password:    "password123",
		DisplayName: "creduser",
	}
	db.Create(user)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create credential for each iteration
		credential := &models.UserCredential{
			UserID:    user.ID,
			APIKey:    fmt.Sprintf("test-key-%d", i),
			APISecret: fmt.Sprintf("test-secret-%d", i),
			Name:      fmt.Sprintf("Credential %d", i),
		}
		db.Create(credential)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("DELETE", fmt.Sprintf("/api/credentials/%d", credential.ID), nil)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", credential.ID)}}
		c.Set("user", user)

		h.handleDeleteCredential(c)
	}
}
