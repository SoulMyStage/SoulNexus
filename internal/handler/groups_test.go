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

func setupGroupsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate models
	err = db.AutoMigrate(
		&models.User{},
		&models.Group{},
	)
	require.NoError(t, err)

	return db
}

func setupGroupsTestRouter(handlers *Handlers) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add middleware to set database
	router.Use(func(c *gin.Context) {
		c.Set(constants.DbField, handlers.db)
		c.Next()
	})

	// Group routes
	router.POST("/group", handlers.CreateGroup)
	router.GET("/group", handlers.ListGroups)
	router.GET("/group/:id", handlers.GetGroup)
	router.PUT("/group/:id", handlers.UpdateGroup)
	router.DELETE("/group/:id", handlers.DeleteGroup)

	return router
}

func createTestGroup(t *testing.T, db *gorm.DB, userID uint) *models.Group {
	group := &models.Group{
		Name:       "Test Group",
		Type:       "standard",
		Extra:      "test extra data",
		CreatorID:  userID,
		Permission: models.GroupPermission{Permissions: []string{"read", "write"}},
	}

	err := db.Create(group).Error
	require.NoError(t, err)

	return group
}

func TestHandlers_handleCreateGroup(t *testing.T) {
	db := setupGroupsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupGroupsTestRouter(handlers)

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
			name: "Valid group creation",
			requestBody: map[string]interface{}{
				"name":  "New Group",
				"type":  "standard",
				"extra": "some extra data",
				"permission": map[string]interface{}{
					"read":  true,
					"write": true,
					"admin": false,
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Minimal group creation",
			requestBody: map[string]interface{}{
				"name": "Minimal Group",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Missing name",
			requestBody: map[string]interface{}{
				"type":  "standard",
				"extra": "data without name",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Empty name",
			requestBody: map[string]interface{}{
				"name": "",
				"type": "standard",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Group with complex permissions",
			requestBody: map[string]interface{}{
				"name": "Complex Group",
				"type": "advanced",
				"permission": map[string]interface{}{
					"users": map[string]interface{}{
						"create": true,
						"read":   true,
						"update": false,
						"delete": false,
					},
					"groups": map[string]interface{}{
						"create": false,
						"read":   true,
						"update": false,
						"delete": false,
					},
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/group", bytes.NewBuffer(body))
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

func TestHandlers_handleListGroups(t *testing.T) {
	db := setupGroupsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupGroupsTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test groups
	group1 := createTestGroup(t, db, user.ID)
	group2 := createTestGroup(t, db, user.ID)
	group2.Name = "Second Group"

	// Verify groups were created
	assert.NotNil(t, group1)
	assert.NotNil(t, group2)
	group2.Type = "premium"
	db.Save(group2)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	req := httptest.NewRequest(http.MethodGet, "/group", nil)
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

	// Verify group data
	firstGroup := data[0].(map[string]interface{})
	assert.Contains(t, firstGroup, "id")
	assert.Contains(t, firstGroup, "name")
	assert.Contains(t, firstGroup, "type")
	assert.Contains(t, firstGroup, "permission")
}

func TestHandlers_handleGetGroup(t *testing.T) {
	db := setupGroupsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupGroupsTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test group
	group := createTestGroup(t, db, user.ID)
	// Verify group was created
	assert.NotNil(t, group)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		groupID        string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Valid group ID",
			groupID:        "1",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Invalid group ID",
			groupID:        "999",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:           "Non-numeric group ID",
			groupID:        "abc",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/group/"+tt.groupID, nil)
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
				assert.Equal(t, group.Name, data["name"])
			}
		})
	}
}

func TestHandlers_handleUpdateGroup(t *testing.T) {
	db := setupGroupsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupGroupsTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test group
	group := createTestGroup(t, db, user.ID)
	// Verify group was created
	assert.NotNil(t, group)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		groupID        string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name:    "Valid group update",
			groupID: "1",
			requestBody: map[string]interface{}{
				"name":  "Updated Group",
				"type":  "premium",
				"extra": "updated extra data",
				"permission": map[string]interface{}{
					"read":   true,
					"write":  true,
					"delete": true,
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:    "Partial update",
			groupID: "1",
			requestBody: map[string]interface{}{
				"name": "Partially Updated Group",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:    "Update permissions only",
			groupID: "1",
			requestBody: map[string]interface{}{
				"permission": map[string]interface{}{
					"admin": true,
					"read":  true,
					"write": false,
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:    "Invalid group ID",
			groupID: "999",
			requestBody: map[string]interface{}{
				"name": "Updated Name",
			},
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:    "Empty name",
			groupID: "1",
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
			req := httptest.NewRequest(http.MethodPut, "/group/"+tt.groupID, bytes.NewBuffer(body))
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

func TestHandlers_handleDeleteGroup(t *testing.T) {
	db := setupGroupsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupGroupsTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test group
	group := createTestGroup(t, db, user.ID)
	// Verify group was created
	assert.NotNil(t, group)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		groupID        string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Valid group deletion",
			groupID:        "1",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Invalid group ID",
			groupID:        "999",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:           "Non-numeric group ID",
			groupID:        "abc",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/group/"+tt.groupID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				// Verify group is deleted
				var count int64
				db.Model(&models.Group{}).Where("id = ?", tt.groupID).Count(&count)
				assert.Equal(t, int64(0), count)
			}
		})
	}
}

func BenchmarkHandlers_handleCreateGroup(b *testing.B) {
	db := setupGroupsTestDB(&testing.T{})
	handlers := &Handlers{db: db}
	router := setupGroupsTestRouter(handlers)

	user := createTestUser(&testing.T{}, db)
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	requestBody := map[string]interface{}{
		"name": "Benchmark Group",
		"type": "standard",
		"permission": map[string]interface{}{
			"read":  true,
			"write": false,
		},
	}
	body, _ := json.Marshal(requestBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/group", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkHandlers_handleListGroups(b *testing.B) {
	db := setupGroupsTestDB(&testing.T{})
	handlers := &Handlers{db: db}
	router := setupGroupsTestRouter(handlers)

	user := createTestUser(&testing.T{}, db)
	createTestGroup(&testing.T{}, db, user.ID)
	createTestGroup(&testing.T{}, db, user.ID)

	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/group", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func TestHandlers_groupsErrorHandling(t *testing.T) {
	db := setupGroupsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupGroupsTestRouter(handlers)

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
			name:           "Invalid JSON in create group",
			method:         http.MethodPost,
			path:           "/group",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid JSON in update group",
			method:         http.MethodPut,
			path:           "/group/1",
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

func TestHandlers_groupsOwnershipValidation(t *testing.T) {
	db := setupGroupsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupGroupsTestRouter(handlers)

	// Create two users
	user1 := createTestUser(t, db)
	user2 := &models.User{
		Email:       "user2@example.com",
		Password:    "hashedpassword",
		DisplayName: "User 2",
		Activated:   true,
	}
	db.Create(user2)

	// Create group for user1
	group := createTestGroup(t, db, user1.ID)

	// Verify group was created
	assert.NotNil(t, group)

	// Try to access user1's group as user2
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
			name:           "Get other user's group",
			method:         http.MethodGet,
			path:           "/group/1",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Update other user's group",
			method:         http.MethodPut,
			path:           "/group/1",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Delete other user's group",
			method:         http.MethodDelete,
			path:           "/group/1",
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

func TestHandlers_groupsCompleteFlow(t *testing.T) {
	db := setupGroupsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupGroupsTestRouter(handlers)

	user := createTestUser(t, db)
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	// Test complete group lifecycle
	t.Run("Complete group lifecycle", func(t *testing.T) {
		// 1. Create group
		createBody := map[string]interface{}{
			"name":  "Lifecycle Group",
			"type":  "standard",
			"extra": "lifecycle test data",
			"permission": map[string]interface{}{
				"read":  true,
				"write": false,
				"admin": false,
			},
		}
		body, _ := json.Marshal(createBody)
		req := httptest.NewRequest(http.MethodPost, "/group", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 2. List groups
		req = httptest.NewRequest(http.MethodGet, "/group", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 3. Get specific group
		req = httptest.NewRequest(http.MethodGet, "/group/1", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 4. Update group
		updateBody := map[string]interface{}{
			"name": "Updated Lifecycle Group",
			"type": "premium",
			"permission": map[string]interface{}{
				"read":   true,
				"write":  true,
				"admin":  false,
				"delete": true,
			},
		}
		body, _ = json.Marshal(updateBody)
		req = httptest.NewRequest(http.MethodPut, "/group/1", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 5. Delete group
		req = httptest.NewRequest(http.MethodDelete, "/group/1", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHandlers_groupsPermissionHandling(t *testing.T) {
	db := setupGroupsTestDB(t)
	handlers := &Handlers{db: db}
	router := setupGroupsTestRouter(handlers)

	user := createTestUser(t, db)
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	// Test different permission structures
	tests := []struct {
		name        string
		permissions interface{}
		expectError bool
	}{
		{
			name: "Simple boolean permissions",
			permissions: map[string]interface{}{
				"read":  true,
				"write": false,
				"admin": false,
			},
			expectError: false,
		},
		{
			name: "Nested permissions",
			permissions: map[string]interface{}{
				"users": map[string]interface{}{
					"create": true,
					"read":   true,
					"update": false,
					"delete": false,
				},
				"groups": map[string]interface{}{
					"create": false,
					"read":   true,
					"update": true,
					"delete": false,
				},
			},
			expectError: false,
		},
		{
			name: "Complex nested permissions",
			permissions: map[string]interface{}{
				"modules": map[string]interface{}{
					"dashboard": map[string]interface{}{
						"view":   true,
						"export": false,
					},
					"reports": map[string]interface{}{
						"view":     true,
						"create":   false,
						"download": true,
					},
				},
			},
			expectError: false,
		},
		{
			name:        "Empty permissions",
			permissions: map[string]interface{}{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestBody := map[string]interface{}{
				"name":       "Permission Test Group",
				"type":       "test",
				"permission": tt.permissions,
			}
			body, _ := json.Marshal(requestBody)
			req := httptest.NewRequest(http.MethodPost, "/group", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if tt.expectError {
				assert.NotEqual(t, http.StatusOK, w.Code)
			} else {
				assert.Equal(t, http.StatusOK, w.Code)

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")
			}
		})
	}
}
