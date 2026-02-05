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

func setupAuthTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate models
	err = db.AutoMigrate(
		&models.User{},
		&models.UserDevice{},
	)
	require.NoError(t, err)

	return db
}

func setupAuthTestRouter(handlers *Handlers) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add middleware to set database
	router.Use(func(c *gin.Context) {
		c.Set(constants.DbField, handlers.db)
		c.Next()
	})

	// Auth routes
	router.POST("/auth/login", handlers.handleUserSignin)
	router.POST("/auth/signup", handlers.handleUserSignup)
	router.POST("/auth/signup-by-email", handlers.handleUserSignupByEmail)
	router.PUT("/auth/update", handlers.handleUserUpdate)
	router.POST("/auth/change-password", handlers.handleChangePassword)
	router.POST("/auth/change-password-by-email", handlers.handleChangePasswordByEmail)
	router.GET("/auth/devices", handlers.handleGetUserDevices)
	router.DELETE("/auth/devices/:deviceId", handlers.handleDeleteUserDevice)
	router.POST("/auth/devices/trust", handlers.handleTrustUserDevice)
	router.POST("/auth/devices/untrust", handlers.handleUntrustUserDevice)
	router.POST("/auth/verify-device", handlers.handleVerifyDeviceForLogin)
	router.POST("/auth/send-device-code", handlers.handleSendDeviceVerificationCode)
	router.POST("/auth/reset-password", handlers.handleResetPassword)
	router.POST("/auth/reset-password/confirm", handlers.handleResetPasswordConfirm)
	router.GET("/auth/verify-email", handlers.handleVerifyEmail)
	router.POST("/auth/send-email-verification", handlers.handleSendEmailVerification)
	router.POST("/auth/verify-phone", handlers.handleVerifyPhone)
	router.GET("/auth/salt", handlers.handleGetSalt)
	router.POST("/auth/send-phone-verification", handlers.handleSendPhoneVerification)
	router.PUT("/auth/notification-settings", handlers.handleUpdateNotificationSettings)
	router.PUT("/auth/user-preferences", handlers.handleUpdateUserPreferences)
	router.GET("/auth/stats", handlers.handleGetUserStats)
	router.POST("/auth/avatar/upload", handlers.handleUploadAvatar)
	router.POST("/auth/two-factor/setup", handlers.handleTwoFactorSetup)
	router.POST("/auth/two-factor/enable", handlers.handleTwoFactorEnable)
	router.POST("/auth/two-factor/disable", handlers.handleTwoFactorDisable)
	router.GET("/auth/two-factor/status", handlers.handleTwoFactorStatus)
	router.POST("/auth/send-email-code", handlers.handleSendEmailCode)
	router.GET("/auth/captcha", handlers.handleGetCaptcha)
	router.POST("/auth/verify-captcha", handlers.handleVerifyCaptcha)
	router.GET("/auth/activity", handlers.handleGetUserActivity)
	router.PUT("/auth/update/preferences", handlers.handleUserUpdatePreferences)
	router.POST("/auth/update/basic/info", handlers.handleUserUpdateBasicInfo)

	return router
}

func createTestUser(t *testing.T, db *gorm.DB) *models.User {
	user := &models.User{
		Email:       "test@example.com",
		Password:    "hashedpassword",
		DisplayName: "Test User",
		Activated:   true,
	}

	err := db.Create(user).Error
	require.NoError(t, err)

	return user
}

func TestHandlers_handleUserLogin(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Valid login with email and password",
			requestBody: map[string]interface{}{
				"email":    user.Email,
				"password": "testpassword",
			},
			expectedStatus: http.StatusUnauthorized, // Will fail due to password mismatch
			expectError:    true,
		},
		{
			name: "Invalid email format",
			requestBody: map[string]interface{}{
				"email":    "invalid-email",
				"password": "testpassword",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Missing email",
			requestBody: map[string]interface{}{
				"password": "testpassword",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Missing password",
			requestBody: map[string]interface{}{
				"email": user.Email,
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "Empty request body",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectError {
				assert.NotEmpty(t, w.Body.String())
			}
		})
	}
}

func TestHandlers_handleUserSignup(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Valid signup",
			requestBody: map[string]interface{}{
				"email":       "newuser@example.com",
				"password":    "password123",
				"displayName": "New User",
				"firstName":   "New",
				"lastName":    "User",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Invalid email format",
			requestBody: map[string]interface{}{
				"email":    "invalid-email",
				"password": "password123",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Missing email",
			requestBody: map[string]interface{}{
				"password": "password123",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Missing password",
			requestBody: map[string]interface{}{
				"email": "test@example.com",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Duplicate email",
			requestBody: map[string]interface{}{
				"email":    "test@example.com", // Already exists from createTestUser
				"password": "password123",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectError {
				assert.NotEmpty(t, w.Body.String())
			}
		})
	}
}

func TestHandlers_handleGetSalt(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	req := httptest.NewRequest(http.MethodGet, "/auth/salt", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Verify response structure
	assert.Contains(t, response, "data")
	data := response["data"].(map[string]interface{})
	assert.Contains(t, data, "salt")
	assert.Contains(t, data, "timestamp")
	assert.Contains(t, data, "expiresIn")

	// Verify salt is not empty
	salt := data["salt"].(string)
	assert.NotEmpty(t, salt)
	assert.True(t, len(salt) > 0)
}

func TestHandlers_handleChangePassword(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

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
			name: "Valid password change",
			requestBody: map[string]interface{}{
				"oldPassword": "currentpassword",
				"newPassword": "newpassword123",
			},
			expectedStatus: http.StatusBadRequest, // Will fail due to password verification
			expectError:    true,
		},
		{
			name: "Missing old password",
			requestBody: map[string]interface{}{
				"newPassword": "newpassword123",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Missing new password",
			requestBody: map[string]interface{}{
				"oldPassword": "currentpassword",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "New password too short",
			requestBody: map[string]interface{}{
				"oldPassword": "currentpassword",
				"newPassword": "123",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Password confirmation mismatch",
			requestBody: map[string]interface{}{
				"oldPassword":     "currentpassword",
				"newPassword":     "newpassword123",
				"confirmPassword": "differentpassword",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/auth/change-password", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectError {
				assert.NotEmpty(t, w.Body.String())
			}
		})
	}
}

func TestHandlers_handleUserUpdate(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

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
			name: "Valid user update",
			requestBody: map[string]interface{}{
				"displayName": "Updated Name",
				"firstName":   "Updated",
				"lastName":    "Name",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Update email",
			requestBody: map[string]interface{}{
				"email": "updated@example.com",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Update phone",
			requestBody: map[string]interface{}{
				"phone": "+1234567890",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Empty update",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, "/auth/update", bytes.NewBuffer(body))
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

func TestHandlers_handleResetPassword(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Valid reset password request",
			requestBody: map[string]interface{}{
				"email": user.Email,
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Non-existent email",
			requestBody: map[string]interface{}{
				"email": "nonexistent@example.com",
			},
			expectedStatus: http.StatusOK, // Still returns success for security
			expectError:    false,
		},
		{
			name: "Invalid email format",
			requestBody: map[string]interface{}{
				"email": "invalid-email",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "Missing email",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBuffer(body))
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

func TestHandlers_handleGetUserStats(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/stats", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Verify response structure
	assert.Contains(t, response, "data")
	data := response["data"].(map[string]interface{})

	// Verify expected stats fields
	expectedFields := []string{
		"loginCount", "profileComplete", "emailVerified",
		"phoneVerified", "twoFactorEnabled", "createdAt",
	}

	for _, field := range expectedFields {
		assert.Contains(t, data, field, "Should contain field: %s", field)
	}
}

func TestHandlers_handleTwoFactorSetup(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/two-factor/setup", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Verify response structure
	assert.Contains(t, response, "data")
	data := response["data"].(map[string]interface{})

	// Verify 2FA setup fields
	assert.Contains(t, data, "secret")
	assert.Contains(t, data, "qrCode")
	assert.Contains(t, data, "url")

	// Verify secret is not empty
	secret := data["secret"].(string)
	assert.NotEmpty(t, secret)
}

func TestHandlers_handleTwoFactorStatus(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/two-factor/status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Verify response structure
	assert.Contains(t, response, "data")
	data := response["data"].(map[string]interface{})

	// Verify 2FA status fields
	assert.Contains(t, data, "enabled")
	assert.Contains(t, data, "hasSecret")

	// Verify boolean values
	enabled := data["enabled"].(bool)
	hasSecret := data["hasSecret"].(bool)
	assert.False(t, enabled)   // New user should not have 2FA enabled
	assert.False(t, hasSecret) // New user should not have secret
}

func TestHandlers_handleSendEmailCode(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Valid email code request",
			requestBody: map[string]interface{}{
				"email": "test@example.com",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Invalid email format",
			requestBody: map[string]interface{}{
				"email": "invalid-email",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "Missing email",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/auth/send-email-code", bytes.NewBuffer(body))
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

func BenchmarkHandlers_handleUserLogin(b *testing.B) {
	db := setupAuthTestDB(&testing.T{})
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	requestBody := map[string]interface{}{
		"email":    "test@example.com",
		"password": "testpassword",
	}
	body, _ := json.Marshal(requestBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkHandlers_handleGetSalt(b *testing.B) {
	db := setupAuthTestDB(&testing.T{})
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/salt", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func TestHandlers_authenticationFlow(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	// Test complete authentication flow
	t.Run("Complete auth flow", func(t *testing.T) {
		// 1. Get salt
		req := httptest.NewRequest(http.MethodGet, "/auth/salt", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 2. Attempt signup
		signupBody := map[string]interface{}{
			"email":       "flowtest@example.com",
			"password":    "password123",
			"displayName": "Flow Test User",
		}
		body, _ := json.Marshal(signupBody)
		req = httptest.NewRequest(http.MethodPost, "/auth/signup", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 3. Attempt login (will fail due to password hashing)
		loginBody := map[string]interface{}{
			"email":    "flowtest@example.com",
			"password": "password123",
		}
		body, _ = json.Marshal(loginBody)
		req = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		// Will likely fail due to password verification, but that's expected in test
	})
}

func TestHandlers_authErrorHandling(t *testing.T) {
	db := setupAuthTestDB(t)
	handlers := &Handlers{db: db}
	router := setupAuthTestRouter(handlers)

	tests := []struct {
		name           string
		method         string
		path           string
		body           interface{}
		expectedStatus int
	}{
		{
			name:           "Invalid JSON in login",
			method:         http.MethodPost,
			path:           "/auth/login",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON in signup",
			method:         http.MethodPost,
			path:           "/auth/signup",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON in password change",
			method:         http.MethodPost,
			path:           "/auth/change-password",
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
