package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/cache"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func setupOTATestDB(t *testing.T) (*gorm.DB, func()) {
	db := setupTestDB(t)

	// Auto migrate tables
	err := db.AutoMigrate(
		&models.User{},
		&models.Assistant{},
		&models.Device{},
		&models.OTA{},
	)
	assert.NoError(t, err)

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS users")
		db.Exec("DROP TABLE IF EXISTS assistants")
		db.Exec("DROP TABLE IF EXISTS devices")
		db.Exec("DROP TABLE IF EXISTS otas")
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}

	return db, cleanup
}

func TestHandlers_HandleOTACheck(t *testing.T) {
	db, cleanup := setupOTATestDB(t)
	defer cleanup()

	// Create test user and assistant
	user := &models.User{
		Email:       "ota@test.com",
		Password:    "password123",
		DisplayName: "otauser",
	}
	db.Create(user)

	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	// Create test device
	device := &models.Device{
		ID:          "00:11:22:33:44:55",
		MacAddress:  "00:11:22:33:44:55",
		UserID:      user.ID,
		AssistantID: func() *uint { id := uint(assistant.ID); return &id }(),
		Board:       "test_board",
		AppVersion:  "1.0.0",
	}
	db.Create(device)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		deviceID   string
		clientID   string
		body       interface{}
		wantStatus int
	}{
		{
			name:     "Valid device check - existing device",
			deviceID: "00:11:22:33:44:55",
			clientID: "client-001",
			body: models.DeviceReportReq{
				Application: &models.Application{
					Version: "1.0.0",
				},
				Board: &models.BoardInfo{
					Type: "test_board",
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "Valid device check - new device",
			deviceID: "00:11:22:33:44:66",
			clientID: "client-002",
			body: models.DeviceReportReq{
				Application: &models.Application{
					Version: "1.0.0",
				},
				Board: &models.BoardInfo{
					Type: "test_board",
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "Missing device ID",
			deviceID:   "",
			clientID:   "client-003",
			body:       models.DeviceReportReq{},
			wantStatus: http.StatusOK,
		},
		{
			name:       "Invalid MAC address format",
			deviceID:   "invalid-mac",
			clientID:   "client-004",
			body:       models.DeviceReportReq{},
			wantStatus: http.StatusOK,
		},
		{
			name:       "Valid device with empty body",
			deviceID:   "00:11:22:33:44:77",
			clientID:   "client-005",
			body:       nil,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			var body *bytes.Buffer
			if tt.body != nil {
				bodyBytes, _ := json.Marshal(tt.body)
				body = bytes.NewBuffer(bodyBytes)
			} else {
				body = bytes.NewBuffer([]byte{})
			}

			c.Request = httptest.NewRequest("POST", "/ota/", body)
			c.Request.Header.Set("Content-Type", "application/json")
			if tt.deviceID != "" {
				c.Request.Header.Set("Device-Id", tt.deviceID)
			}
			if tt.clientID != "" {
				c.Request.Header.Set("Client-Id", tt.clientID)
			}

			h.HandleOTACheck(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp models.DeviceReportResp
			json.Unmarshal(w.Body.Bytes(), &resp)

			// Verify response structure
			assert.NotNil(t, resp.ServerTime)
			assert.NotNil(t, resp.Firmware)

			if tt.deviceID == "" || tt.deviceID == "invalid-mac" {
				// Should return error response
				return
			}

			// Check if device exists to determine expected response
			var existingDevice models.Device
			err := db.Where("mac_address = ?", tt.deviceID).First(&existingDevice).Error

			if err == gorm.ErrRecordNotFound {
				// New device - should have activation code
				assert.NotNil(t, resp.Activation)
				assert.NotEmpty(t, resp.Activation.Code)
				assert.NotEmpty(t, resp.Activation.Challenge)
				assert.NotEmpty(t, resp.Activation.Message)
			} else {
				// Existing device - should have websocket config
				assert.NotNil(t, resp.Websocket)
				assert.NotEmpty(t, resp.Websocket.URL)
			}
		})
	}
}

func TestHandlers_HandleOTAActivate(t *testing.T) {
	db, cleanup := setupOTATestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "ota@test.com",
		Password:    "password123",
		DisplayName: "otauser",
	}
	db.Create(user)

	device := &models.Device{
		ID:         "00:11:22:33:44:55",
		MacAddress: "00:11:22:33:44:55",
		UserID:     user.ID,
		Board:      "test_board",
		AppVersion: "1.0.0",
	}
	db.Create(device)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		deviceID   string
		wantStatus int
	}{
		{
			name:       "Existing device",
			deviceID:   "00:11:22:33:44:55",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Non-existent device",
			deviceID:   "00:11:22:33:44:99",
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "Missing device ID",
			deviceID:   "",
			wantStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("POST", "/ota/activate", nil)
			if tt.deviceID != "" {
				c.Request.Header.Set("Device-Id", tt.deviceID)
			}

			h.HandleOTAActivate(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantStatus == http.StatusOK {
				assert.Equal(t, "success", w.Body.String())
			}
		})
	}
}

func TestHandlers_HandleOTAGet(t *testing.T) {
	db, cleanup := setupOTATestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/ota/", nil)

	h.HandleOTAGet(c)

	assert.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, "OTA interface")
}

func TestHandlers_buildOTAResponse(t *testing.T) {
	db, cleanup := setupOTATestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "ota@test.com",
		Password:    "password123",
		DisplayName: "otauser",
	}
	db.Create(user)

	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	device := &models.Device{
		ID:          "00:11:22:33:44:55",
		MacAddress:  "00:11:22:33:44:55",
		UserID:      user.ID,
		AssistantID: func() *uint { id := uint(assistant.ID); return &id }(),
		Board:       "test_board",
		AppVersion:  "1.0.0",
		AutoUpdate:  1,
	}
	db.Create(device)

	h := &Handlers{db: db}

	tests := []struct {
		name     string
		deviceID string
		clientID string
		req      *models.DeviceReportReq
	}{
		{
			name:     "Existing device",
			deviceID: "00:11:22:33:44:55",
			clientID: "client-001",
			req: &models.DeviceReportReq{
				Application: &models.Application{
					Version: "1.0.0",
				},
			},
		},
		{
			name:     "New device",
			deviceID: "00:11:22:33:44:99",
			clientID: "client-002",
			req: &models.DeviceReportReq{
				Application: &models.Application{
					Version: "1.0.0",
				},
				Board: &models.BoardInfo{
					Type: "test_board",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := h.buildOTAResponse(tt.deviceID, tt.clientID, tt.req)

			assert.NotNil(t, resp)
			assert.NotNil(t, resp.ServerTime)
			assert.NotNil(t, resp.Firmware)

			// Check if device exists
			var existingDevice models.Device
			err := db.Where("mac_address = ?", tt.deviceID).First(&existingDevice).Error

			if err == gorm.ErrRecordNotFound {
				// New device - should have activation
				assert.NotNil(t, resp.Activation)
				assert.NotEmpty(t, resp.Activation.Code)
			} else {
				// Existing device - should have websocket config
				assert.NotNil(t, resp.Websocket)
			}
		})
	}
}

func TestHandlers_buildActivation(t *testing.T) {
	db, cleanup := setupOTATestDB(t)
	defer cleanup()

	h := &Handlers{db: db}

	deviceID := "00:11:22:33:44:88"
	req := &models.DeviceReportReq{
		Application: &models.Application{
			Version: "1.0.0",
		},
		Board: &models.BoardInfo{
			Type: "test_board",
		},
	}

	activation := h.buildActivation(deviceID, req)

	assert.NotNil(t, activation)
	assert.Equal(t, deviceID, activation.Challenge)
	assert.NotEmpty(t, activation.Code)
	assert.NotEmpty(t, activation.Message)
	assert.Len(t, activation.Code, 6) // 6-digit activation code

	// Verify activation code is stored in cache
	ctx := context.Background()
	cacheClient := cache.GetGlobalCache()

	codeKey := fmt.Sprintf("ota:activation:code:%s", activation.Code)
	cachedDeviceID, ok := cacheClient.Get(ctx, codeKey)
	assert.True(t, ok)
	assert.Equal(t, deviceID, cachedDeviceID)
}

func TestHandlers_buildMQTTConfig(t *testing.T) {
	db, cleanup := setupOTATestDB(t)
	defer cleanup()

	h := &Handlers{db: db}

	deviceID := "00:11:22:33:44:55"
	groupID := "GID_test_board"

	mqttConfig := h.buildMQTTConfig(deviceID, groupID)

	assert.NotNil(t, mqttConfig)
	assert.NotEmpty(t, mqttConfig.ClientID)
	assert.NotEmpty(t, mqttConfig.Username)
	assert.NotEmpty(t, mqttConfig.PublishTopic)
	assert.NotEmpty(t, mqttConfig.SubscribeTopic)

	// Verify client ID format
	expectedClientID := fmt.Sprintf("%s@@@%s@@@%s", groupID, "00_11_22_33_44_55", "00_11_22_33_44_55")
	assert.Equal(t, expectedClientID, mqttConfig.ClientID)

	// Verify subscribe topic format
	expectedSubscribeTopic := "devices/p2p/00_11_22_33_44_55"
	assert.Equal(t, expectedSubscribeTopic, mqttConfig.SubscribeTopic)
}

func TestHandlers_getLatestFirmware(t *testing.T) {
	db, cleanup := setupOTATestDB(t)
	defer cleanup()

	// Create test OTA firmware
	ota := &models.OTA{
		Type:         "test_board",
		Version:      "1.1.0",
		FirmwarePath: "https://example.com/firmware/v1.1.0.bin",
		Remark:       "Test firmware update",
	}
	db.Create(ota)

	h := &Handlers{db: db}

	tests := []struct {
		name            string
		boardType       string
		currentVersion  string
		expectedURL     string
		expectedVersion string
	}{
		{
			name:            "Update available",
			boardType:       "test_board",
			currentVersion:  "1.0.0",
			expectedURL:     "https://example.com/firmware/v1.1.0.bin",
			expectedVersion: "1.1.0",
		},
		{
			name:            "Same version",
			boardType:       "test_board",
			currentVersion:  "1.1.0",
			expectedURL:     "",
			expectedVersion: "1.1.0",
		},
		{
			name:            "No firmware available",
			boardType:       "unknown_board",
			currentVersion:  "1.0.0",
			expectedURL:     "",
			expectedVersion: "1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firmware := h.getLatestFirmware(tt.boardType, tt.currentVersion)

			assert.NotNil(t, firmware)
			assert.Equal(t, tt.expectedVersion, firmware.Version)
			assert.Equal(t, tt.expectedURL, firmware.URL)
		})
	}
}

func TestGenerateActivationCode(t *testing.T) {
	// Test multiple generations to ensure randomness
	codes := make(map[string]bool)

	for i := 0; i < 100; i++ {
		code := generateActivationCode()

		assert.Len(t, code, 6)
		assert.Regexp(t, `^\d{6}$`, code) // Should be 6 digits

		// Check for reasonable randomness (no duplicates in 100 generations is very likely)
		assert.False(t, codes[code], "Duplicate activation code generated: %s", code)
		codes[code] = true
	}
}

func TestIsMacAddressValid_OTA(t *testing.T) {
	tests := []struct {
		name       string
		macAddress string
		expected   bool
	}{
		{
			name:       "Valid MAC with colons",
			macAddress: "00:11:22:33:44:55",
			expected:   true,
		},
		{
			name:       "Valid MAC with dashes",
			macAddress: "00-11-22-33-44-55",
			expected:   true,
		},
		{
			name:       "Invalid MAC format",
			macAddress: "invalid-mac",
			expected:   false,
		},
		{
			name:       "Empty MAC",
			macAddress: "",
			expected:   false,
		},
		{
			name:       "Uppercase MAC",
			macAddress: "AA:BB:CC:DD:EE:FF",
			expected:   true,
		},
		{
			name:       "Mixed case MAC",
			macAddress: "aA:bB:cC:dD:eE:fF",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMacAddressValid(tt.macAddress)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGeneratePasswordSignature(t *testing.T) {
	content := "test-content"
	secretKey := "test-secret-key"

	signature := generatePasswordSignature(content, secretKey)

	assert.NotEmpty(t, signature)
	assert.Greater(t, len(signature), 20) // Base64 encoded HMAC-SHA256 should be longer

	// Test consistency
	signature2 := generatePasswordSignature(content, secretKey)
	assert.Equal(t, signature, signature2)

	// Test different content produces different signature
	signature3 := generatePasswordSignature("different-content", secretKey)
	assert.NotEqual(t, signature, signature3)
}

func TestHandlers_OTACheck_WithCache(t *testing.T) {
	db, cleanup := setupOTATestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	deviceID := "00:11:22:33:44:99"

	// Setup cache with existing activation data
	ctx := context.Background()
	cacheClient := cache.GetGlobalCache()

	safeDeviceId := "00_11_22_33_44_99"
	dataKey := fmt.Sprintf("ota:activation:data:%s", safeDeviceId)
	existingCode := "123456"

	dataMap := map[string]interface{}{
		"id":              deviceID,
		"mac_address":     deviceID,
		"board":           "test_board",
		"app_version":     "1.0.0",
		"deviceId":        deviceID,
		"activation_code": existingCode,
	}

	cacheClient.Set(ctx, dataKey, dataMap, time.Hour)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("POST", "/ota/", bytes.NewBuffer([]byte("{}")))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Device-Id", deviceID)

	h.HandleOTACheck(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.DeviceReportResp
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Should reuse existing activation code
	assert.NotNil(t, resp.Activation)
	assert.Equal(t, existingCode, resp.Activation.Code)
}

// Benchmark tests
func BenchmarkHandlers_HandleOTACheck(b *testing.B) {
	db, cleanup := setupOTATestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "ota@test.com",
		Password:    "password123",
		DisplayName: "otauser",
	}
	db.Create(user)

	device := &models.Device{
		ID:         "00:11:22:33:44:55",
		MacAddress: "00:11:22:33:44:55",
		UserID:     user.ID,
		Board:      "test_board",
		AppVersion: "1.0.0",
	}
	db.Create(device)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	req := models.DeviceReportReq{
		Application: &models.Application{
			Version: "1.0.0",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body, _ := json.Marshal(req)
		c.Request = httptest.NewRequest("POST", "/ota/", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.Header.Set("Device-Id", "00:11:22:33:44:55")

		h.HandleOTACheck(c)
	}
}

func BenchmarkGenerateActivationCode(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generateActivationCode()
	}
}

func BenchmarkGeneratePasswordSignature(b *testing.B) {
	content := "test-content-for-benchmark"
	secretKey := "test-secret-key-for-benchmark"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generatePasswordSignature(content, secretKey)
	}
}
