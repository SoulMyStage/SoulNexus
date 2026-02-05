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

func setupDeviceTestDB(t *testing.T) (*gorm.DB, func()) {
	db := setupTestDB(t)

	// Auto migrate tables
	err := db.AutoMigrate(
		&models.User{},
		&models.Assistant{},
		&models.Device{},
		&models.Group{},
		&models.GroupMember{},
		&models.CallRecording{},
		&models.DeviceErrorLog{},
		&models.OTA{},
	)
	assert.NoError(t, err)

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS users")
		db.Exec("DROP TABLE IF EXISTS assistants")
		db.Exec("DROP TABLE IF EXISTS devices")
		db.Exec("DROP TABLE IF EXISTS groups")
		db.Exec("DROP TABLE IF EXISTS group_members")
		db.Exec("DROP TABLE IF EXISTS call_recordings")
		db.Exec("DROP TABLE IF EXISTS device_error_logs")
		db.Exec("DROP TABLE IF EXISTS otas")
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}

	return db, cleanup
}

func TestHandlers_BindDevice(t *testing.T) {
	db, cleanup := setupDeviceTestDB(t)
	defer cleanup()

	// Create test user
	user := &models.User{
		Email:       "device@test.com",
		Password:    "password123",
		DisplayName: "deviceuser",
	}
	db.Create(user)

	// Create test assistant
	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
	}
	db.Create(assistant)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Setup cache with activation data
	ctx := context.Background()
	cacheClient := cache.GetGlobalCache()

	deviceID := "00:11:22:33:44:55"
	activationCode := "123456"

	// Store activation data in cache
	safeDeviceId := "00_11_22_33_44_55"
	dataKey := fmt.Sprintf("ota:activation:data:%s", safeDeviceId)
	codeKey := fmt.Sprintf("ota:activation:code:%s", activationCode)

	dataMap := map[string]interface{}{
		"id":              deviceID,
		"mac_address":     deviceID,
		"board":           "test_board",
		"app_version":     "1.0.0",
		"deviceId":        deviceID,
		"activation_code": activationCode,
	}

	cacheClient.Set(ctx, dataKey, dataMap, time.Hour)
	cacheClient.Set(ctx, codeKey, deviceID, time.Hour)

	tests := []struct {
		name       string
		agentID    string
		deviceCode string
		wantStatus int
		wantError  bool
	}{
		{
			name:       "Valid device binding",
			agentID:    fmt.Sprintf("%d", assistant.ID),
			deviceCode: activationCode,
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:       "Invalid activation code",
			agentID:    fmt.Sprintf("%d", assistant.ID),
			deviceCode: "invalid",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
		{
			name:       "Empty activation code",
			agentID:    fmt.Sprintf("%d", assistant.ID),
			deviceCode: "",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
		{
			name:       "Invalid agent ID",
			agentID:    "invalid",
			deviceCode: activationCode,
			wantStatus: http.StatusOK,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("POST", fmt.Sprintf("/device/bind/%s/%s", tt.agentID, tt.deviceCode), nil)
			c.Params = gin.Params{
				{Key: "agentId", Value: tt.agentID},
				{Key: "deviceCode", Value: tt.deviceCode},
			}
			c.Set("user", user)

			h.BindDevice(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError {
				assert.Equal(t, "fail", resp["status"])
			} else {
				assert.Equal(t, "success", resp["status"])
			}
		})
	}
}

func TestHandlers_GetUserDevices(t *testing.T) {
	db, cleanup := setupDeviceTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "device@test.com",
		Password:    "password123",
		DisplayName: "deviceuser",
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

	// Create test devices
	assistantIDUint1 := uint(assistant.ID)
	assistantIDUint2 := uint(assistant.ID)
	devices := []models.Device{
		{
			ID:          "device-001",
			MacAddress:  "00:11:22:33:44:55",
			UserID:      user.ID,
			AssistantID: &assistantIDUint1,
			Board:       "test_board",
			AppVersion:  "1.0.0",
		},
		{
			ID:          "device-002",
			MacAddress:  "00:11:22:33:44:66",
			UserID:      user.ID,
			AssistantID: &assistantIDUint2,
			Board:       "test_board",
			AppVersion:  "1.0.1",
		},
	}

	for _, device := range devices {
		db.Create(&device)
	}

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", fmt.Sprintf("/device/bind/%d", assistant.ID), nil)
	c.Params = gin.Params{{Key: "agentId", Value: fmt.Sprintf("%d", assistant.ID)}}
	c.Set("user", user)

	h.GetUserDevices(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])
	data := resp["data"].([]interface{})
	assert.Equal(t, 2, len(data))
}

func TestHandlers_UnbindDevice(t *testing.T) {
	db, cleanup := setupDeviceTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "device@test.com",
		Password:    "password123",
		DisplayName: "deviceuser",
	}
	db.Create(user)

	device := &models.Device{
		ID:         "device-001",
		MacAddress: "00:11:22:33:44:55",
		UserID:     user.ID,
		Board:      "test_board",
		AppVersion: "1.0.0",
	}
	db.Create(device)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	request := struct {
		DeviceID string `json:"deviceId"`
	}{
		DeviceID: device.ID,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(request)
	c.Request = httptest.NewRequest("POST", "/device/unbind", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", user)

	h.UnbindDevice(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])

	// Verify deletion
	var deleted models.Device
	err := db.First(&deleted, "id = ?", device.ID).Error
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestHandlers_UpdateDeviceInfo(t *testing.T) {
	db, cleanup := setupDeviceTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "device@test.com",
		Password:    "password123",
		DisplayName: "deviceuser",
	}
	db.Create(user)

	device := &models.Device{
		ID:         "device-001",
		MacAddress: "00:11:22:33:44:55",
		UserID:     user.ID,
		Board:      "test_board",
		AppVersion: "1.0.0",
		Alias:      "Old Alias",
		AutoUpdate: 1,
	}
	db.Create(device)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	newAutoUpdate := 0
	request := struct {
		Alias      string `json:"alias"`
		AutoUpdate *int   `json:"autoUpdate"`
	}{
		Alias:      "New Alias",
		AutoUpdate: &newAutoUpdate,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(request)
	c.Request = httptest.NewRequest("PUT", fmt.Sprintf("/device/update/%s", device.ID), bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: device.ID}}
	c.Set("user", user)

	h.UpdateDeviceInfo(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])

	// Verify update
	var updated models.Device
	db.First(&updated, "id = ?", device.ID)
	assert.Equal(t, "New Alias", updated.Alias)
	assert.Equal(t, 0, updated.AutoUpdate)
}

func TestHandlers_ManualAddDevice(t *testing.T) {
	db, cleanup := setupDeviceTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "device@test.com",
		Password:    "password123",
		DisplayName: "deviceuser",
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

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		request    map[string]interface{}
		wantStatus int
		wantError  bool
	}{
		{
			name: "Valid manual device addition",
			request: map[string]interface{}{
				"agentId":    fmt.Sprintf("%d", assistant.ID),
				"board":      "test_board",
				"appVersion": "1.0.0",
				"macAddress": "00:11:22:33:44:77",
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name: "Invalid MAC address",
			request: map[string]interface{}{
				"agentId":    fmt.Sprintf("%d", assistant.ID),
				"board":      "test_board",
				"appVersion": "1.0.0",
				"macAddress": "invalid-mac",
			},
			wantStatus: http.StatusOK,
			wantError:  true,
		},
		{
			name: "Missing required fields",
			request: map[string]interface{}{
				"agentId": fmt.Sprintf("%d", assistant.ID),
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
			c.Request = httptest.NewRequest("POST", "/device/manual-add", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", user)

			h.ManualAddDevice(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError {
				assert.Equal(t, "fail", resp["status"])
			} else {
				assert.Equal(t, "success", resp["status"])
			}
		})
	}
}

func TestHandlers_GetDeviceConfig(t *testing.T) {
	db, cleanup := setupDeviceTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "device@test.com",
		Password:    "password123",
		DisplayName: "deviceuser",
	}
	db.Create(user)

	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
		ApiKey:       "test-api-key",
		ApiSecret:    "test-api-secret",
		LLMModel:     "gpt-3.5-turbo",
		Temperature:  0.7,
		MaxTokens:    1000,
	}
	db.Create(assistant)

	device := &models.Device{
		ID:          "device-001",
		MacAddress:  "00:11:22:33:44:55",
		UserID:      user.ID,
		AssistantID: &[]uint{uint(assistant.ID)}[0],
		Board:       "test_board",
		AppVersion:  "1.0.0",
	}
	db.Create(device)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		deviceID   string
		wantStatus int
		wantError  bool
	}{
		{
			name:       "Valid device config request",
			deviceID:   device.MacAddress,
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:       "Non-existent device",
			deviceID:   "00:11:22:33:44:99",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
		{
			name:       "Empty device ID",
			deviceID:   "",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", fmt.Sprintf("/device/config/%s", tt.deviceID), nil)
			c.Params = gin.Params{{Key: "deviceId", Value: tt.deviceID}}

			h.GetDeviceConfig(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError {
				assert.Equal(t, "fail", resp["status"])
			} else {
				assert.Equal(t, "success", resp["status"])

				// Verify config data
				data := resp["data"].(map[string]interface{})
				assert.Equal(t, tt.deviceID, data["deviceId"])
				assert.Equal(t, float64(assistant.ID), data["assistantId"])
				assert.Equal(t, assistant.ApiKey, data["apiKey"])
				assert.Equal(t, assistant.ApiSecret, data["apiSecret"])
			}
		})
	}
}

func TestHandlers_UpdateDeviceStatus(t *testing.T) {
	db, cleanup := setupDeviceTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "device@test.com",
		Password:    "password123",
		DisplayName: "deviceuser",
	}
	db.Create(user)

	device := &models.Device{
		ID:         "device-001",
		MacAddress: "00:11:22:33:44:55",
		UserID:     user.ID,
		Board:      "test_board",
		AppVersion: "1.0.0",
	}
	db.Create(device)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	isOnline := true
	cpuUsage := 45.5
	memoryUsage := 60.2
	temperature := 35.8

	request := struct {
		MacAddress    string                 `json:"macAddress"`
		IsOnline      *bool                  `json:"isOnline"`
		CPUUsage      *float64               `json:"cpuUsage"`
		MemoryUsage   *float64               `json:"memoryUsage"`
		Temperature   *float64               `json:"temperature"`
		SystemInfo    map[string]interface{} `json:"systemInfo"`
		HardwareInfo  map[string]interface{} `json:"hardwareInfo"`
		NetworkInfo   map[string]interface{} `json:"networkInfo"`
		AudioStatus   map[string]interface{} `json:"audioStatus"`
		ServiceStatus map[string]interface{} `json:"serviceStatus"`
	}{
		MacAddress:  device.MacAddress,
		IsOnline:    &isOnline,
		CPUUsage:    &cpuUsage,
		MemoryUsage: &memoryUsage,
		Temperature: &temperature,
		SystemInfo: map[string]interface{}{
			"os":      "Linux",
			"version": "5.4.0",
		},
		HardwareInfo: map[string]interface{}{
			"cpu": "ARM Cortex-A72",
			"ram": "4GB",
		},
		NetworkInfo: map[string]interface{}{
			"wifi": "connected",
			"ip":   "192.168.1.100",
		},
		AudioStatus: map[string]interface{}{
			"microphone": "active",
			"speaker":    "active",
		},
		ServiceStatus: map[string]interface{}{
			"asr": "running",
			"tts": "running",
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(request)
	c.Request = httptest.NewRequest("POST", "/device/status", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateDeviceStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])
}

func TestHandlers_LogDeviceError(t *testing.T) {
	db, cleanup := setupDeviceTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "device@test.com",
		Password:    "password123",
		DisplayName: "deviceuser",
	}
	db.Create(user)

	device := &models.Device{
		ID:         "device-001",
		MacAddress: "00:11:22:33:44:55",
		UserID:     user.ID,
		Board:      "test_board",
		AppVersion: "1.0.0",
	}
	db.Create(device)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	request := struct {
		MacAddress string `json:"macAddress"`
		ErrorType  string `json:"errorType"`
		ErrorLevel string `json:"errorLevel"`
		ErrorCode  string `json:"errorCode"`
		ErrorMsg   string `json:"errorMsg"`
		StackTrace string `json:"stackTrace"`
		Context    string `json:"context"`
	}{
		MacAddress: device.MacAddress,
		ErrorType:  "system",
		ErrorLevel: "error",
		ErrorCode:  "E001",
		ErrorMsg:   "Test error message",
		StackTrace: "Stack trace here",
		Context:    "Error context",
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(request)
	c.Request = httptest.NewRequest("POST", "/device/error", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.LogDeviceError(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])
}

func TestHandlers_GetDeviceDetail(t *testing.T) {
	db, cleanup := setupDeviceTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "device@test.com",
		Password:    "password123",
		DisplayName: "deviceuser",
	}
	db.Create(user)

	device := &models.Device{
		ID:         "device-001",
		MacAddress: "00:11:22:33:44:55",
		UserID:     user.ID,
		Board:      "test_board",
		AppVersion: "1.0.0",
		Alias:      "Test Device",
	}
	db.Create(device)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", fmt.Sprintf("/device/%s", device.MacAddress), nil)
	c.Params = gin.Params{{Key: "deviceId", Value: device.MacAddress}}
	c.Set("user", user)

	h.GetDeviceDetail(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, device.MacAddress, data["mac_address"])
	assert.Equal(t, device.Alias, data["alias"])
}

func TestHandlers_GetCallRecordingDetail(t *testing.T) {
	db, cleanup := setupDeviceTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "device@test.com",
		Password:    "password123",
		DisplayName: "deviceuser",
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

	recording := &models.CallRecording{
		UserID:      user.ID,
		AssistantID: uint(assistant.ID),
		MacAddress:  "00:11:22:33:44:55",
		SessionID:   "session-123",
		Duration:    120,
		UserInput:   "Hello",
		AIResponse:  "Hi there",
		StartTime:   time.Now().Add(-2 * time.Minute),
		EndTime:     time.Now(),
	}
	db.Create(recording)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/device/call-recordings/%d", recording.ID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", recording.ID)}}
	c.Set("user", user)

	h.GetCallRecordingDetail(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, float64(recording.ID), data["id"])
	assert.Equal(t, recording.SessionID, data["sessionId"])
	assert.NotNil(t, data["conversationDetailsData"])
	assert.NotNil(t, data["timingMetricsData"])
}

// Test helper functions
func TestIsMacAddressValid(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMacAddressValid(tt.macAddress)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateBasicConversationDetails(t *testing.T) {
	recording := models.CallRecording{
		SessionID:  "test-session",
		UserInput:  "Hello",
		AIResponse: "Hi there",
		StartTime:  time.Now().Add(-2 * time.Minute),
		EndTime:    time.Now(),
		Duration:   120,
	}

	details := generateBasicConversationDetails(recording)

	assert.Equal(t, recording.SessionID, details["sessionId"])
	assert.Equal(t, 2, details["totalTurns"])
	assert.Equal(t, 1, details["userTurns"])
	assert.Equal(t, 1, details["aiTurns"])
	assert.Equal(t, 0, details["interruptions"])

	turns := details["turns"].([]map[string]interface{})
	assert.Equal(t, 2, len(turns))
	assert.Equal(t, "user", turns[0]["type"])
	assert.Equal(t, "ai", turns[1]["type"])
}

func TestGenerateBasicTimingMetrics(t *testing.T) {
	recording := models.CallRecording{
		UserInput:  "Hello",
		AIResponse: "Hi there",
		Duration:   120,
	}

	metrics := generateBasicTimingMetrics(recording)

	assert.Equal(t, float64(120000), metrics["sessionDuration"]) // 120 seconds * 1000
	assert.Equal(t, 1, metrics["asrCalls"])
	assert.Equal(t, 1, metrics["llmCalls"])
	assert.Equal(t, 1, metrics["ttsCalls"])
	assert.Equal(t, 1000, metrics["asrTotalTime"])
	assert.Equal(t, 1500, metrics["llmTotalTime"])
	assert.Equal(t, 800, metrics["ttsTotalTime"])
}

// Benchmark tests
func BenchmarkHandlers_GetDeviceConfig(b *testing.B) {
	db, cleanup := setupDeviceTestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "device@test.com",
		Password:    "password123",
		DisplayName: "deviceuser",
	}
	db.Create(user)

	assistant := &models.Assistant{
		UserID:       user.ID,
		Name:         "Test Assistant",
		SystemPrompt: "Test prompt",
		Language:     "zh",
		Speaker:      "test_speaker",
		ApiKey:       "test-api-key",
		ApiSecret:    "test-api-secret",
	}
	db.Create(assistant)

	device := &models.Device{
		ID:          "device-001",
		MacAddress:  "00:11:22:33:44:55",
		UserID:      user.ID,
		AssistantID: &[]uint{uint(assistant.ID)}[0],
		Board:       "test_board",
		AppVersion:  "1.0.0",
	}
	db.Create(device)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", fmt.Sprintf("/device/config/%s", device.MacAddress), nil)
		c.Params = gin.Params{{Key: "deviceId", Value: device.MacAddress}}

		h.GetDeviceConfig(c)
	}
}

func BenchmarkHandlers_UpdateDeviceStatus(b *testing.B) {
	db, cleanup := setupDeviceTestDB(&testing.T{})
	defer cleanup()

	device := &models.Device{
		ID:         "device-001",
		MacAddress: "00:11:22:33:44:55",
		Board:      "test_board",
		AppVersion: "1.0.0",
	}
	db.Create(device)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	isOnline := true
	request := struct {
		MacAddress string `json:"macAddress"`
		IsOnline   *bool  `json:"isOnline"`
	}{
		MacAddress: device.MacAddress,
		IsOnline:   &isOnline,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body, _ := json.Marshal(request)
		c.Request = httptest.NewRequest("POST", "/device/status", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.UpdateDeviceStatus(c)
	}
}
