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

func TestPhoneNumberHandler_ListPhoneNumbers(t *testing.T) {
	db := setupTestDB(t)

	handler := NewPhoneNumberHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test phone numbers
	phoneNumber1 := &models.PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138000",
		CountryCode: "+86",
		Carrier:     "移动",
		Alias:       "主号码",
		Status:      models.PhoneNumberStatusActive,
		IsVerified:  true,
		IsPrimary:   true,
	}
	require.NoError(t, db.Create(phoneNumber1).Error)

	phoneNumber2 := &models.PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13900139000",
		CountryCode: "+86",
		Carrier:     "联通",
		Alias:       "备用号码",
		Status:      models.PhoneNumberStatusActive,
		IsVerified:  false,
		IsPrimary:   false,
	}
	require.NoError(t, db.Create(phoneNumber2).Error)

	tests := []struct {
		name           string
		setupAuth      bool
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "Success - List phone numbers",
			setupAuth:      true,
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:           "Unauthorized - No auth",
			setupAuth:      false,
			expectedStatus: http.StatusUnauthorized,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/phone-numbers", nil)

			if tt.setupAuth {
				c.Set("user", user)
			}

			handler.ListPhoneNumbers(c)

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

func TestPhoneNumberHandler_GetPhoneNumber(t *testing.T) {
	db := setupTestDB(t)

	handler := NewPhoneNumberHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create another user
	otherUser := &models.User{
		Email:       "other@example.com",
		DisplayName: "otheruser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(otherUser).Error)

	// Create test phone number
	phoneNumber := &models.PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138000",
		CountryCode: "+86",
		Carrier:     "移动",
		Alias:       "主号码",
		Status:      models.PhoneNumberStatusActive,
		IsVerified:  true,
		IsPrimary:   true,
	}
	require.NoError(t, db.Create(phoneNumber).Error)

	tests := []struct {
		name           string
		phoneNumberID  string
		setupAuth      bool
		authUser       *models.User
		expectedStatus int
	}{
		{
			name:           "Success - Get phone number",
			phoneNumberID:  fmt.Sprintf("%d", phoneNumber.ID),
			setupAuth:      true,
			authUser:       user,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Unauthorized - No auth",
			phoneNumberID:  fmt.Sprintf("%d", phoneNumber.ID),
			setupAuth:      false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Forbidden - Different user",
			phoneNumberID:  fmt.Sprintf("%d", phoneNumber.ID),
			setupAuth:      true,
			authUser:       otherUser,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Bad Request - Invalid ID",
			phoneNumberID:  "invalid",
			setupAuth:      true,
			authUser:       user,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Not Found - Non-existent ID",
			phoneNumberID:  "99999",
			setupAuth:      true,
			authUser:       user,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/phone-numbers/"+tt.phoneNumberID, nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.phoneNumberID}}

			if tt.setupAuth {
				c.Set("user", tt.authUser)
			}

			handler.GetPhoneNumber(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data, ok := response["data"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, phoneNumber.PhoneNumber, data["phoneNumber"])
			}
		})
	}
}

func TestPhoneNumberHandler_CreatePhoneNumber(t *testing.T) {
	db := setupTestDB(t)

	handler := NewPhoneNumberHandler(db)
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
		requestBody    map[string]interface{}
		setupAuth      bool
		expectedStatus int
	}{
		{
			name: "Success - Create phone number",
			requestBody: map[string]interface{}{
				"phoneNumber": "13800138000",
				"countryCode": "+86",
				"carrier":     "移动",
				"alias":       "主号码",
				"description": "我的主要号码",
			},
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name: "Success - Create with default country code",
			requestBody: map[string]interface{}{
				"phoneNumber": "13900139000",
				"carrier":     "联通",
				"alias":       "备用号码",
			},
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Unauthorized - No auth",
			requestBody:    map[string]interface{}{"phoneNumber": "13800138000"},
			setupAuth:      false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Bad Request - Missing phone number",
			requestBody:    map[string]interface{}{"carrier": "移动"},
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Bad Request - Duplicate phone number",
			requestBody: map[string]interface{}{
				"phoneNumber": "13800138000", // Same as first test
				"carrier":     "移动",
			},
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/phone-numbers", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			if tt.setupAuth {
				c.Set("user", user)
			}

			handler.CreatePhoneNumber(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data, ok := response["data"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, tt.requestBody["phoneNumber"], data["phoneNumber"])

				// Check if it's set as primary for first phone number
				if tt.name == "Success - Create phone number" {
					assert.True(t, data["isPrimary"].(bool))
				}
			}
		})
	}
}

func TestPhoneNumberHandler_UpdatePhoneNumber(t *testing.T) {
	db := setupTestDB(t)

	handler := NewPhoneNumberHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test phone number
	phoneNumber := &models.PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138000",
		CountryCode: "+86",
		Carrier:     "移动",
		Alias:       "主号码",
		Status:      models.PhoneNumberStatusActive,
		IsVerified:  true,
		IsPrimary:   true,
	}
	require.NoError(t, db.Create(phoneNumber).Error)

	tests := []struct {
		name           string
		phoneNumberID  string
		requestBody    map[string]interface{}
		setupAuth      bool
		expectedStatus int
	}{
		{
			name:          "Success - Update phone number",
			phoneNumberID: fmt.Sprintf("%d", phoneNumber.ID),
			requestBody: map[string]interface{}{
				"carrier":     "联通",
				"alias":       "更新的别名",
				"description": "更新的描述",
				"status":      "inactive",
			},
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Unauthorized - No auth",
			phoneNumberID:  fmt.Sprintf("%d", phoneNumber.ID),
			requestBody:    map[string]interface{}{"carrier": "联通"},
			setupAuth:      false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Bad Request - Invalid ID",
			phoneNumberID:  "invalid",
			requestBody:    map[string]interface{}{"carrier": "联通"},
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Not Found - Non-existent ID",
			phoneNumberID:  "99999",
			requestBody:    map[string]interface{}{"carrier": "联通"},
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("PUT", "/api/phone-numbers/"+tt.phoneNumberID, bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = []gin.Param{{Key: "id", Value: tt.phoneNumberID}}

			if tt.setupAuth {
				c.Set("user", user)
			}

			handler.UpdatePhoneNumber(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data, ok := response["data"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, tt.requestBody["carrier"], data["carrier"])
			}
		})
	}
}

func TestPhoneNumberHandler_DeletePhoneNumber(t *testing.T) {
	db := setupTestDB(t)

	handler := NewPhoneNumberHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test phone numbers
	primaryPhone := &models.PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138000",
		CountryCode: "+86",
		Carrier:     "移动",
		Status:      models.PhoneNumberStatusActive,
		IsPrimary:   true,
	}
	require.NoError(t, db.Create(primaryPhone).Error)

	secondaryPhone := &models.PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13900139000",
		CountryCode: "+86",
		Carrier:     "联通",
		Status:      models.PhoneNumberStatusActive,
		IsPrimary:   false,
	}
	require.NoError(t, db.Create(secondaryPhone).Error)

	tests := []struct {
		name           string
		phoneNumberID  string
		setupAuth      bool
		expectedStatus int
	}{
		{
			name:           "Success - Delete secondary phone number",
			phoneNumberID:  fmt.Sprintf("%d", secondaryPhone.ID),
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Bad Request - Cannot delete primary phone",
			phoneNumberID:  fmt.Sprintf("%d", primaryPhone.ID),
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Unauthorized - No auth",
			phoneNumberID:  fmt.Sprintf("%d", secondaryPhone.ID),
			setupAuth:      false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Bad Request - Invalid ID",
			phoneNumberID:  "invalid",
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("DELETE", "/api/phone-numbers/"+tt.phoneNumberID, nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.phoneNumberID}}

			if tt.setupAuth {
				c.Set("user", user)
			}

			handler.DeletePhoneNumber(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestPhoneNumberHandler_SetPrimaryPhoneNumber(t *testing.T) {
	db := setupTestDB(t)

	handler := NewPhoneNumberHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test phone numbers
	phoneNumber1 := &models.PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138000",
		CountryCode: "+86",
		Status:      models.PhoneNumberStatusActive,
		IsPrimary:   true,
	}
	require.NoError(t, db.Create(phoneNumber1).Error)

	phoneNumber2 := &models.PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13900139000",
		CountryCode: "+86",
		Status:      models.PhoneNumberStatusActive,
		IsPrimary:   false,
	}
	require.NoError(t, db.Create(phoneNumber2).Error)

	tests := []struct {
		name           string
		phoneNumberID  string
		setupAuth      bool
		expectedStatus int
	}{
		{
			name:           "Success - Set primary phone number",
			phoneNumberID:  fmt.Sprintf("%d", phoneNumber2.ID),
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Unauthorized - No auth",
			phoneNumberID:  fmt.Sprintf("%d", phoneNumber2.ID),
			setupAuth:      false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Bad Request - Invalid ID",
			phoneNumberID:  "invalid",
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/phone-numbers/"+tt.phoneNumberID+"/set-primary", nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.phoneNumberID}}

			if tt.setupAuth {
				c.Set("user", user)
			}

			handler.SetPrimaryPhoneNumber(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestPhoneNumberHandler_GetCallForwardGuide(t *testing.T) {
	db := setupTestDB(t)

	handler := NewPhoneNumberHandler(db)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		carrier        string
		expectedStatus int
	}{
		{
			name:           "Success - 移动 carrier",
			carrier:        "移动",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Success - 联通 carrier",
			carrier:        "联通",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Success - 电信 carrier",
			carrier:        "电信",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Success - Default carrier (no param)",
			carrier:        "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Success - Unknown carrier (fallback to 移动)",
			carrier:        "unknown",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/phone-numbers/call-forward-guide"
			if tt.carrier != "" {
				url += "?carrier=" + tt.carrier
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", url, nil)

			handler.GetCallForwardGuide(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data, ok := response["data"].(map[string]interface{})
				require.True(t, ok)
				assert.Contains(t, data, "carrier")
				assert.Contains(t, data, "codes")
			}
		})
	}
}

func TestPhoneNumberHandler_UpdateCallForwardStatus(t *testing.T) {
	db := setupTestDB(t)

	handler := NewPhoneNumberHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test phone number
	phoneNumber := &models.PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138000",
		CountryCode: "+86",
		Status:      models.PhoneNumberStatusActive,
	}
	require.NoError(t, db.Create(phoneNumber).Error)

	tests := []struct {
		name           string
		phoneNumberID  string
		requestBody    map[string]interface{}
		setupAuth      bool
		expectedStatus int
	}{
		{
			name:          "Success - Update call forward status",
			phoneNumberID: fmt.Sprintf("%d", phoneNumber.ID),
			requestBody: map[string]interface{}{
				"enabled": true,
				"status":  "active",
			},
			setupAuth:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Unauthorized - No auth",
			phoneNumberID:  fmt.Sprintf("%d", phoneNumber.ID),
			requestBody:    map[string]interface{}{"enabled": true},
			setupAuth:      false,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Bad Request - Invalid ID",
			phoneNumberID:  "invalid",
			requestBody:    map[string]interface{}{"enabled": true},
			setupAuth:      true,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/phone-numbers/"+tt.phoneNumberID+"/call-forward-status", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = []gin.Param{{Key: "id", Value: tt.phoneNumberID}}

			if tt.setupAuth {
				c.Set("user", user)
			}

			handler.UpdateCallForwardStatus(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// Benchmark tests
func BenchmarkPhoneNumberHandler_ListPhoneNumbers(b *testing.B) {
	db := setupTestDB(&testing.T{})

	handler := NewPhoneNumberHandler(db)
	gin.SetMode(gin.TestMode)

	// Create test user and phone numbers
	user := &models.User{Email: "test@example.com", DisplayName: "testuser"}
	db.Create(user)

	for i := 0; i < 100; i++ {
		phoneNumber := &models.PhoneNumber{
			UserID:      user.ID,
			PhoneNumber: fmt.Sprintf("1380013%04d", i),
			CountryCode: "+86",
			Status:      models.PhoneNumberStatusActive,
		}
		db.Create(phoneNumber)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/phone-numbers", nil)
		c.Set("user", user)

		handler.ListPhoneNumbers(c)
	}
}

func BenchmarkPhoneNumberHandler_CreatePhoneNumber(b *testing.B) {
	db := setupTestDB(&testing.T{})

	handler := NewPhoneNumberHandler(db)
	gin.SetMode(gin.TestMode)

	user := &models.User{Email: "test@example.com", DisplayName: "testuser"}
	db.Create(user)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requestBody := map[string]interface{}{
			"phoneNumber": fmt.Sprintf("1380013%04d", i),
			"countryCode": "+86",
			"carrier":     "移动",
		}
		body, _ := json.Marshal(requestBody)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/phone-numbers", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)

		handler.CreatePhoneNumber(c)
	}
}
