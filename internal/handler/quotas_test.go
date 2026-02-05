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

func TestHandlers_ListUserQuotas(t *testing.T) {
	db := setupTestDB(t)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test quotas
	quota1 := &models.UserQuota{
		UserID:     user.ID,
		QuotaType:  models.QuotaTypeAPICalls,
		TotalQuota: 1000,
		UsedQuota:  100,
		Period:     models.QuotaPeriodMonthly,
	}
	require.NoError(t, db.Create(quota1).Error)

	quota2 := &models.UserQuota{
		UserID:     user.ID,
		QuotaType:  models.QuotaTypeLLMTokens,
		TotalQuota: 5000,
		UsedQuota:  500,
		Period:     models.QuotaPeriodMonthly,
	}
	require.NoError(t, db.Create(quota2).Error)

	tests := []struct {
		name           string
		setupAuth      bool
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "Success - List user quotas",
			setupAuth:      true,
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:           "Unauthorized - No auth",
			setupAuth:      false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/quota/user", nil)

			if tt.setupAuth {
				c.Set("user", user)
			}

			handlers.ListUserQuotas(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data, ok := response["data"].([]interface{})
				require.True(t, ok)
				assert.Len(t, data, tt.expectedCount)
			}
		})
	}
}

func TestHandlers_GetUserQuota(t *testing.T) {
	db := setupTestDB(t)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test quota
	quota := &models.UserQuota{
		UserID:     user.ID,
		QuotaType:  models.QuotaTypeAPICalls,
		TotalQuota: 1000,
		UsedQuota:  100,
		Period:     models.QuotaPeriodMonthly,
	}
	require.NoError(t, db.Create(quota).Error)

	tests := []struct {
		name           string
		quotaType      string
		setupAuth      bool
		expectedStatus int
	}{
		{
			name:           "Success - Get user quota",
			quotaType:      string(models.QuotaTypeAPICalls),
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Not Found - Non-existent quota type",
			quotaType:      "non_existent",
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Unauthorized - No auth",
			quotaType:      string(models.QuotaTypeAPICalls),
			setupAuth:      false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/quota/user/"+tt.quotaType, nil)
			c.Params = []gin.Param{{Key: "type", Value: tt.quotaType}}

			if tt.setupAuth {
				c.Set("user", user)
			}

			handlers.GetUserQuota(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data, ok := response["data"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, string(models.QuotaTypeAPICalls), data["quotaType"])
			}
		})
	}
}

func TestHandlers_CreateUserQuota(t *testing.T) {
	db := setupTestDB(t)

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
		requestBody    CreateUserQuotaRequest
		setupAuth      bool
		expectedStatus int
	}{
		{
			name: "Success - Create user quota",
			requestBody: CreateUserQuotaRequest{
				QuotaType:  models.QuotaTypeAPICalls,
				TotalQuota: 1000,
				Period:     models.QuotaPeriodMonthly,
			},
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name: "Success - Create quota with default period",
			requestBody: CreateUserQuotaRequest{
				QuotaType:  models.QuotaTypeLLMTokens,
				TotalQuota: 5000,
			},
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name: "Bad Request - Duplicate quota type",
			requestBody: CreateUserQuotaRequest{
				QuotaType:  models.QuotaTypeAPICalls, // Same as first test
				TotalQuota: 2000,
				Period:     models.QuotaPeriodMonthly,
			},
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Bad Request - Invalid total quota",
			requestBody: CreateUserQuotaRequest{
				QuotaType:  models.QuotaTypeLLMCalls,
				TotalQuota: -100, // Negative value should fail validation
				Period:     models.QuotaPeriodMonthly,
			},
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Unauthorized - No auth",
			requestBody:    CreateUserQuotaRequest{QuotaType: models.QuotaTypeAPICalls, TotalQuota: 1000},
			setupAuth:      false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/quota/user", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			if tt.setupAuth {
				c.Set("user", user)
			}

			handlers.CreateUserQuota(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data, ok := response["data"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, string(tt.requestBody.QuotaType), data["quotaType"])
			}
		})
	}
}

func TestHandlers_UpdateUserQuota(t *testing.T) {
	db := setupTestDB(t)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test quota
	quota := &models.UserQuota{
		UserID:     user.ID,
		QuotaType:  models.QuotaTypeAPICalls,
		TotalQuota: 1000,
		UsedQuota:  100,
		Period:     models.QuotaPeriodMonthly,
	}
	require.NoError(t, db.Create(quota).Error)

	tests := []struct {
		name           string
		quotaType      string
		requestBody    UpdateUserQuotaRequest
		setupAuth      bool
		expectedStatus int
	}{
		{
			name:      "Success - Update user quota",
			quotaType: string(models.QuotaTypeAPICalls),
			requestBody: UpdateUserQuotaRequest{
				TotalQuota: func(i int64) *int64 { return &i }(2000),
				Period:     func(p models.QuotaPeriod) *models.QuotaPeriod { return &p }(models.QuotaPeriodMonthly),
			},
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Not Found - Non-existent quota type",
			quotaType:      "non_existent",
			requestBody:    UpdateUserQuotaRequest{},
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Unauthorized - No auth",
			quotaType:      string(models.QuotaTypeAPICalls),
			requestBody:    UpdateUserQuotaRequest{},
			setupAuth:      false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("PUT", "/api/quota/user/"+tt.quotaType, bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = []gin.Param{{Key: "type", Value: tt.quotaType}}

			if tt.setupAuth {
				c.Set("user", user)
			}

			handlers.UpdateUserQuota(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data, ok := response["data"].(map[string]interface{})
				require.True(t, ok)
				if tt.requestBody.TotalQuota != nil {
					assert.Equal(t, float64(*tt.requestBody.TotalQuota), data["totalQuota"])
				}
			}
		})
	}
}

func TestHandlers_DeleteUserQuota(t *testing.T) {
	db := setupTestDB(t)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test quota
	quota := &models.UserQuota{
		UserID:     user.ID,
		QuotaType:  models.QuotaTypeAPICalls,
		TotalQuota: 1000,
		UsedQuota:  100,
		Period:     models.QuotaPeriodMonthly,
	}
	require.NoError(t, db.Create(quota).Error)

	tests := []struct {
		name           string
		quotaType      string
		setupAuth      bool
		expectedStatus int
	}{
		{
			name:           "Success - Delete user quota",
			quotaType:      string(models.QuotaTypeAPICalls),
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Success - Delete non-existent quota (no error)",
			quotaType:      "non_existent",
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Unauthorized - No auth",
			quotaType:      string(models.QuotaTypeLLMTokens),
			setupAuth:      false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("DELETE", "/api/quota/user/"+tt.quotaType, nil)
			c.Params = []gin.Param{{Key: "type", Value: tt.quotaType}}

			if tt.setupAuth {
				c.Set("user", user)
			}

			handlers.DeleteUserQuota(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestHandlers_ListGroupQuotas(t *testing.T) {
	db := setupTestDB(t)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test group
	group := &models.Group{
		Name:      "Test Group",
		CreatorID: user.ID,
	}
	require.NoError(t, db.Create(group).Error)

	// Create test group quota
	quota := &models.GroupQuota{
		GroupID:    group.ID,
		QuotaType:  models.QuotaTypeAPICalls,
		TotalQuota: 5000,
		UsedQuota:  500,
		Period:     models.QuotaPeriodMonthly,
	}
	require.NoError(t, db.Create(quota).Error)

	tests := []struct {
		name           string
		groupID        string
		setupAuth      bool
		expectedStatus int
	}{
		{
			name:           "Success - List group quotas",
			groupID:        fmt.Sprintf("%d", group.ID),
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Bad Request - Invalid group ID",
			groupID:        "invalid",
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Not Found - Non-existent group",
			groupID:        "99999",
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Unauthorized - No auth",
			groupID:        fmt.Sprintf("%d", group.ID),
			setupAuth:      false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/quota/group/"+tt.groupID, nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.groupID}}

			if tt.setupAuth {
				c.Set("user", user)
			}

			handlers.ListGroupQuotas(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data, ok := response["data"].([]interface{})
				require.True(t, ok)
				assert.Len(t, data, 1)
			}
		})
	}
}

func TestHandlers_CreateGroupQuota(t *testing.T) {
	db := setupTestDB(t)

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test group
	group := &models.Group{
		Name:      "Test Group",
		CreatorID: user.ID,
	}
	require.NoError(t, db.Create(group).Error)

	tests := []struct {
		name           string
		groupID        string
		requestBody    CreateGroupQuotaRequest
		setupAuth      bool
		expectedStatus int
	}{
		{
			name:    "Success - Create group quota",
			groupID: fmt.Sprintf("%d", group.ID),
			requestBody: CreateGroupQuotaRequest{
				QuotaType:  models.QuotaTypeAPICalls,
				TotalQuota: 5000,
				Period:     models.QuotaPeriodMonthly,
			},
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Bad Request - Invalid group ID",
			groupID:        "invalid",
			requestBody:    CreateGroupQuotaRequest{QuotaType: models.QuotaTypeAPICalls, TotalQuota: 1000},
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Unauthorized - No auth",
			groupID:        fmt.Sprintf("%d", group.ID),
			requestBody:    CreateGroupQuotaRequest{QuotaType: models.QuotaTypeAPICalls, TotalQuota: 1000},
			setupAuth:      false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/quota/group/"+tt.groupID, bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = []gin.Param{{Key: "id", Value: tt.groupID}}

			if tt.setupAuth {
				c.Set("user", user)
			}

			handlers.CreateGroupQuota(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data, ok := response["data"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, string(tt.requestBody.QuotaType), data["quotaType"])
			}
		})
	}
}

// Benchmark tests
func BenchmarkHandlers_ListUserQuotas(b *testing.B) {
	db := setupTestDB(&testing.T{})

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user and quotas
	user := &models.User{Email: "test@example.com", DisplayName: "testuser"}
	db.Create(user)

	for i := 0; i < 10; i++ {
		quota := &models.UserQuota{
			UserID:     user.ID,
			QuotaType:  models.QuotaType(fmt.Sprintf("quota_type_%d", i)),
			TotalQuota: int64(1000 * (i + 1)),
			UsedQuota:  int64(100 * (i + 1)),
			Period:     models.QuotaPeriodMonthly,
		}
		db.Create(quota)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/quota/user", nil)
		c.Set("user", user)

		handlers.ListUserQuotas(c)
	}
}

func BenchmarkHandlers_CreateUserQuota(b *testing.B) {
	db := setupTestDB(&testing.T{})

	handlers := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	user := &models.User{Email: "test@example.com", DisplayName: "testuser"}
	db.Create(user)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requestBody := CreateUserQuotaRequest{
			QuotaType:  models.QuotaType(fmt.Sprintf("quota_type_%d", i)),
			TotalQuota: 1000,
			Period:     models.QuotaPeriodMonthly,
		}
		body, _ := json.Marshal(requestBody)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/quota/user", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)

		handlers.CreateUserQuota(c)
	}
}
