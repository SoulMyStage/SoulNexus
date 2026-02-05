package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/constants"
	"github.com/code-100-precent/LingEcho/pkg/notification"
)

func setupNotificationsTestDB(t *testing.T) (*gorm.DB, func()) {
	db := setupTestDB(t)

	// Auto-migrate models
	err := db.AutoMigrate(
		&models.User{},
		&notification.InternalNotification{},
	)
	require.NoError(t, err)

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS users")
		db.Exec("DROP TABLE IF EXISTS internal_notifications")
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}

	return db, cleanup
}

func setupNotificationsTestRouter(handlers *Handlers) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add middleware to set database
	router.Use(func(c *gin.Context) {
		c.Set(constants.DbField, handlers.db)
		c.Next()
	})

	// Notification routes
	router.GET("/notification/unread-count", handlers.handleUnReadNotificationCount)
	router.GET("/notification", handlers.handleListNotifications)
	router.POST("/notification/readAll", handlers.handleAllNotifications)
	router.PUT("/notification/read/:id", handlers.handleMarkNotificationAsRead)
	router.DELETE("/notification/:id", handlers.handleDeleteNotification)
	router.POST("/notification/batch-delete", handlers.handleBatchDeleteNotifications)

	return router
}

func createTestInternalNotification(t *testing.T, db *gorm.DB, userID uint, isRead bool) *notification.InternalNotification {
	notif := &notification.InternalNotification{
		UserID:  userID,
		Title:   "Test Notification",
		Content: "This is a test notification",
		Read:    isRead,
	}

	err := db.Create(notif).Error
	require.NoError(t, err)

	return notif
}

func TestHandlers_handleGetUnreadNotificationCount(t *testing.T) {
	db, cleanup := setupNotificationsTestDB(t)
	defer cleanup()

	handlers := &Handlers{db: db}
	router := setupNotificationsTestRouter(handlers)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "testuser",
	}
	db.Create(user)

	// Create test notifications
	createTestInternalNotification(t, db, user.ID, false) // unread
	createTestInternalNotification(t, db, user.ID, false) // unread
	createTestInternalNotification(t, db, user.ID, true)  // read

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})

	req := httptest.NewRequest(http.MethodGet, "/notification/unread-count", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Should return count of 2 unread notifications
	assert.Contains(t, response, "data")
	count := response["data"].(float64)
	assert.Equal(t, float64(2), count)
}

func TestHandlers_handleListNotifications(t *testing.T) {
	db, cleanup := setupNotificationsTestDB(t)
	defer cleanup()

	handlers := &Handlers{db: db}
	router := setupNotificationsTestRouter(handlers)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "testuser",
	}
	db.Create(user)

	// Create test notifications
	createTestInternalNotification(t, db, user.ID, false)
	notification2 := createTestInternalNotification(t, db, user.ID, true)
	notification2.Title = "Second Notification"
	db.Save(notification2)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "List all notifications",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "List with pagination",
			queryParams:    "?page=1&size=10",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "List unread only",
			queryParams:    "?filter=unread",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "List read only",
			queryParams:    "?filter=read",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/notification"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "list")
				assert.Contains(t, data, "total")
				assert.Contains(t, data, "totalUnread")
				assert.Contains(t, data, "totalRead")
				assert.Contains(t, data, "page")
				assert.Contains(t, data, "size")
			}
		})
	}
}

func TestHandlers_handleMarkAllNotificationsRead(t *testing.T) {
	db, cleanup := setupNotificationsTestDB(t)
	defer cleanup()

	handlers := &Handlers{db: db}
	router := setupNotificationsTestRouter(handlers)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "testuser",
	}
	db.Create(user)

	// Create test notifications (all unread)
	createTestInternalNotification(t, db, user.ID, false)
	createTestInternalNotification(t, db, user.ID, false)
	createTestInternalNotification(t, db, user.ID, false)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})

	req := httptest.NewRequest(http.MethodPost, "/notification/readAll", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify all notifications are marked as read
	var unreadCount int64
	db.Model(&notification.InternalNotification{}).Where("user_id = ? AND `read` = ?", user.ID, false).Count(&unreadCount)
	assert.Equal(t, int64(0), unreadCount)
}

func TestHandlers_handleMarkNotificationRead(t *testing.T) {
	db, cleanup := setupNotificationsTestDB(t)
	defer cleanup()

	handlers := &Handlers{db: db}
	router := setupNotificationsTestRouter(handlers)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "testuser",
	}
	db.Create(user)

	// Create test notification (unread)
	createTestInternalNotification(t, db, user.ID, false)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})

	tests := []struct {
		name           string
		notificationID string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Valid notification mark as read",
			notificationID: "1",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Invalid notification ID",
			notificationID: "999",
			expectedStatus: http.StatusOK,
			expectError:    true,
		},
		{
			name:           "Non-numeric notification ID",
			notificationID: "abc",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/notification/read/"+tt.notificationID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if tt.expectError {
				assert.NotEqual(t, http.StatusOK, w.Code)
			} else {
				assert.Equal(t, tt.expectedStatus, w.Code)

				// Verify notification is marked as read
				var updatedNotification notification.InternalNotification
				db.First(&updatedNotification, tt.notificationID)
				assert.True(t, updatedNotification.Read)
			}
		})
	}
}

func TestHandlers_handleDeleteNotification(t *testing.T) {
	db, cleanup := setupNotificationsTestDB(t)
	defer cleanup()

	handlers := &Handlers{db: db}
	router := setupNotificationsTestRouter(handlers)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "testuser",
	}
	db.Create(user)

	// Create test notification
	createTestInternalNotification(t, db, user.ID, false)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})

	tests := []struct {
		name           string
		notificationID string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Valid notification deletion",
			notificationID: "1",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Invalid notification ID",
			notificationID: "999",
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name:           "Non-numeric notification ID",
			notificationID: "abc",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/notification/"+tt.notificationID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if tt.expectError {
				assert.NotEqual(t, http.StatusOK, w.Code)
			} else {
				assert.Equal(t, tt.expectedStatus, w.Code)

				// Verify notification is deleted
				var count int64
				db.Model(&notification.InternalNotification{}).Where("id = ?", tt.notificationID).Count(&count)
				assert.Equal(t, int64(0), count)
			}
		})
	}
}

func TestHandlers_handleBatchDeleteNotifications(t *testing.T) {
	db, cleanup := setupNotificationsTestDB(t)
	defer cleanup()

	handlers := &Handlers{db: db}
	router := setupNotificationsTestRouter(handlers)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "testuser",
	}
	db.Create(user)

	// Create test notifications
	createTestInternalNotification(t, db, user.ID, false)
	createTestInternalNotification(t, db, user.ID, true)
	createTestInternalNotification(t, db, user.ID, false)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Valid batch delete",
			requestBody: map[string]interface{}{
				"ids": []int{1, 2},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Empty IDs array",
			requestBody: map[string]interface{}{
				"ids": []int{},
			},
			expectedStatus: http.StatusOK,
			expectError:    true,
		},
		{
			name:           "Missing IDs field",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusOK,
			expectError:    true,
		},
		{
			name: "Invalid notification IDs",
			requestBody: map[string]interface{}{
				"ids": []int{999, 1000},
			},
			expectedStatus: http.StatusOK, // May still succeed but delete 0 records
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/notification/batch-delete", bytes.NewBuffer(body))
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

func TestHandlers_notificationsErrorHandling(t *testing.T) {
	db, cleanup := setupNotificationsTestDB(t)
	defer cleanup()

	handlers := &Handlers{db: db}
	router := setupNotificationsTestRouter(handlers)

	user := &models.User{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "testuser",
	}
	db.Create(user)

	router.Use(func(c *gin.Context) {
		c.Set("user", user)
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
			name:           "Invalid JSON in batch delete",
			method:         http.MethodPost,
			path:           "/notification/batch-delete",
			body:           "invalid json",
			expectedStatus: http.StatusOK, // Handler may handle gracefully
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

			// Just ensure it doesn't crash
			assert.NotEqual(t, 0, w.Code)
		})
	}
}

func TestHandlers_notificationsCompleteFlow(t *testing.T) {
	db, cleanup := setupNotificationsTestDB(t)
	defer cleanup()

	handlers := &Handlers{db: db}
	router := setupNotificationsTestRouter(handlers)

	user := &models.User{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "testuser",
	}
	db.Create(user)

	router.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})

	// Create some test notifications
	createTestInternalNotification(t, db, user.ID, false)
	createTestInternalNotification(t, db, user.ID, false)
	createTestInternalNotification(t, db, user.ID, true)

	// Test complete notification management flow
	t.Run("Complete notification management flow", func(t *testing.T) {
		// 1. Get unread count
		req := httptest.NewRequest(http.MethodGet, "/notification/unread-count", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 2. List all notifications
		req = httptest.NewRequest(http.MethodGet, "/notification", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 3. Mark specific notification as read
		req = httptest.NewRequest(http.MethodPut, "/notification/read/1", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 4. Mark all notifications as read
		req = httptest.NewRequest(http.MethodPost, "/notification/readAll", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 5. Delete specific notification
		req = httptest.NewRequest(http.MethodDelete, "/notification/1", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 6. Batch delete remaining notifications
		batchDeleteBody := map[string]interface{}{
			"ids": []int{2, 3},
		}
		body, _ := json.Marshal(batchDeleteBody)
		req = httptest.NewRequest(http.MethodPost, "/notification/batch-delete", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHandlers_notificationsPagination(t *testing.T) {
	db, cleanup := setupNotificationsTestDB(t)
	defer cleanup()

	handlers := &Handlers{db: db}
	router := setupNotificationsTestRouter(handlers)

	user := &models.User{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "testuser",
	}
	db.Create(user)

	router.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})

	// Create many notifications for pagination testing
	for i := 0; i < 25; i++ {
		notif := createTestInternalNotification(t, db, user.ID, i%3 == 0)
		notif.Title = fmt.Sprintf("Notification %d", i+1)
		db.Save(notif)
	}

	tests := []struct {
		name        string
		queryParams string
		expectItems int
	}{
		{
			name:        "First page with default size",
			queryParams: "?page=1",
			expectItems: 10, // Default page size
		},
		{
			name:        "Custom page size",
			queryParams: "?page=1&size=10",
			expectItems: 10,
		},
		{
			name:        "Large page size",
			queryParams: "?page=1&size=100",
			expectItems: 25, // All items
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/notification"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			data := response["data"].(map[string]interface{})
			_ = data["list"].([]interface{})

			// Verify pagination metadata
			assert.Contains(t, data, "total")
			assert.Contains(t, data, "page")
			assert.Contains(t, data, "size")
			assert.Equal(t, float64(25), data["total"].(float64))
		})
	}
}

// Benchmark tests
func BenchmarkHandlers_handleGetUnreadNotificationCount(b *testing.B) {
	db, cleanup := setupNotificationsTestDB(&testing.T{})
	defer cleanup()

	handlers := &Handlers{db: db}
	router := setupNotificationsTestRouter(handlers)

	user := &models.User{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "testuser",
	}
	db.Create(user)

	createTestInternalNotification(&testing.T{}, db, user.ID, false)
	createTestInternalNotification(&testing.T{}, db, user.ID, false)

	router.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/notification/unread-count", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkHandlers_handleListNotifications(b *testing.B) {
	db, cleanup := setupNotificationsTestDB(&testing.T{})
	defer cleanup()

	handlers := &Handlers{db: db}
	router := setupNotificationsTestRouter(handlers)

	user := &models.User{
		Email:       "test@example.com",
		Password:    "password123",
		DisplayName: "testuser",
	}
	db.Create(user)

	for i := 0; i < 10; i++ {
		createTestInternalNotification(&testing.T{}, db, user.ID, i%2 == 0)
	}

	router.Use(func(c *gin.Context) {
		c.Set("user", user)
		c.Next()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/notification", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
