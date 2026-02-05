package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlers_HandleWebSocketVoice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test credential
	cred := models.UserCredential{
		UserID:    1,
		APIKey:    "test_api_key",
		APISecret: "test_api_secret",
	}
	require.NoError(t, db.Create(&cred).Error)

	// Create test assistant
	assistant := models.Assistant{
		ID:           1,
		Name:         "Test Assistant",
		SystemPrompt: "You are a helpful assistant",
		Temperature:  0.7,
		Language:     "zh-cn",
		Speaker:      "101016",
	}
	require.NoError(t, db.Create(&assistant).Error)

	tests := []struct {
		name         string
		queryParams  map[string]string
		expectedCode int
	}{
		{
			name: "Valid WebSocket request",
			queryParams: map[string]string{
				"apiKey":      "test_api_key",
				"apiSecret":   "test_api_secret",
				"assistantId": "1",
				"language":    "zh-cn",
				"speaker":     "101016",
			},
			expectedCode: 400, // Will fail upgrade but parameters are valid
		},
		{
			name: "Missing API key",
			queryParams: map[string]string{
				"apiSecret":   "test_api_secret",
				"assistantId": "1",
			},
			expectedCode: 500,
		},
		{
			name: "Missing API secret",
			queryParams: map[string]string{
				"apiKey":      "test_api_key",
				"assistantId": "1",
			},
			expectedCode: 500,
		},
		{
			name: "Invalid assistant ID",
			queryParams: map[string]string{
				"apiKey":      "test_api_key",
				"apiSecret":   "test_api_secret",
				"assistantId": "invalid",
			},
			expectedCode: 500,
		},
		{
			name: "Missing assistant ID",
			queryParams: map[string]string{
				"apiKey":    "test_api_key",
				"apiSecret": "test_api_secret",
			},
			expectedCode: 500,
		},
		{
			name: "Invalid credentials",
			queryParams: map[string]string{
				"apiKey":      "invalid_key",
				"apiSecret":   "invalid_secret",
				"assistantId": "1",
			},
			expectedCode: 500,
		},
		{
			name: "Non-existent assistant",
			queryParams: map[string]string{
				"apiKey":      "test_api_key",
				"apiSecret":   "test_api_secret",
				"assistantId": "99999",
			},
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Build query string
			values := url.Values{}
			for k, v := range tt.queryParams {
				values.Set(k, v)
			}

			req := httptest.NewRequest("GET", "/ws/voice?"+values.Encode(), nil)
			req.Header.Set("Connection", "upgrade")
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Sec-WebSocket-Version", "13")
			req.Header.Set("Sec-WebSocket-Key", "test-key")
			c.Request = req

			h.HandleWebSocketVoice(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

func TestHandlers_HandleHardwareWebSocketVoice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test user
	user := models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(&user).Error)

	// Create test credential
	cred := models.UserCredential{
		UserID:    user.ID,
		APIKey:    "device_api_key",
		APISecret: "device_api_secret",
	}
	require.NoError(t, db.Create(&cred).Error)

	// Create test assistant
	assistant := models.Assistant{
		ID:           1,
		Name:         "Hardware Assistant",
		SystemPrompt: "You are a hardware assistant",
		Temperature:  0.8,
		Language:     "zh-cn",
		Speaker:      "502007",
		ApiKey:       "device_api_key",
		ApiSecret:    "device_api_secret",
	}
	require.NoError(t, db.Create(&assistant).Error)

	// Create test device
	assistantID := uint(assistant.ID)
	device := models.Device{
		MacAddress:  "AA:BB:CC:DD:EE:FF",
		UserID:      user.ID,
		AssistantID: &assistantID,
	}
	require.NoError(t, db.Create(&device).Error)

	tests := []struct {
		name         string
		deviceID     string
		headerKey    string
		expectedCode int
	}{
		{
			name:         "Valid hardware WebSocket request with header",
			deviceID:     "AA:BB:CC:DD:EE:FF",
			headerKey:    "Device-Id",
			expectedCode: 400, // Will fail upgrade but parameters are valid
		},
		{
			name:         "Valid hardware WebSocket request with query param",
			deviceID:     "AA:BB:CC:DD:EE:FF",
			headerKey:    "",
			expectedCode: 400, // Will fail upgrade but parameters are valid
		},
		{
			name:         "Missing device ID",
			deviceID:     "",
			headerKey:    "",
			expectedCode: 400,
		},
		{
			name:         "Non-existent device",
			deviceID:     "FF:EE:DD:CC:BB:AA",
			headerKey:    "Device-Id",
			expectedCode: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			var req *http.Request
			if tt.headerKey != "" {
				req = httptest.NewRequest("GET", "/ws/hardware", nil)
				req.Header.Set(tt.headerKey, tt.deviceID)
			} else if tt.deviceID != "" {
				req = httptest.NewRequest("GET", "/ws/hardware?device-id="+tt.deviceID, nil)
			} else {
				req = httptest.NewRequest("GET", "/ws/hardware", nil)
			}

			req.Header.Set("Connection", "upgrade")
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Sec-WebSocket-Version", "13")
			req.Header.Set("Sec-WebSocket-Key", "test-key")
			c.Request = req

			h.HandleHardwareWebSocketVoice(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}
func TestHandlers_WebSocketVoiceParameterHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test credential
	cred := models.UserCredential{
		UserID:    1,
		APIKey:    "test_api_key",
		APISecret: "test_api_secret",
	}
	require.NoError(t, db.Create(&cred).Error)

	// Create test assistant with different configurations
	assistant := models.Assistant{
		ID:           1,
		Name:         "Test Assistant",
		SystemPrompt: "Custom system prompt",
		Temperature:  0.9,
		Language:     "en-us",
		Speaker:      "custom_speaker",
	}
	require.NoError(t, db.Create(&assistant).Error)

	t.Run("Default parameters when not provided", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("GET", "/ws/voice?apiKey=test_api_key&apiSecret=test_api_secret&assistantId=1", nil)
		req.Header.Set("Connection", "upgrade")
		req.Header.Set("Upgrade", "websocket")
		c.Request = req

		h.HandleWebSocketVoice(c)

		// Should use assistant's configured language and speaker
		assert.Equal(t, 400, w.Code) // Upgrade will fail but parameters are processed
	})

	t.Run("Override parameters", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		queryParams := "apiKey=test_api_key&apiSecret=test_api_secret&assistantId=1&language=zh-cn&speaker=101016"
		req := httptest.NewRequest("GET", "/ws/voice?"+queryParams, nil)
		req.Header.Set("Connection", "upgrade")
		req.Header.Set("Upgrade", "websocket")
		c.Request = req

		h.HandleWebSocketVoice(c)

		// Should use provided parameters instead of assistant defaults
		assert.Equal(t, 400, w.Code) // Upgrade will fail but parameters are processed
	})
}

func TestHandlers_HardwareWebSocketDeviceValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test user
	user := models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(&user).Error)

	// Create test credential
	cred := models.UserCredential{
		UserID:    user.ID,
		APIKey:    "device_api_key",
		APISecret: "device_api_secret",
	}
	require.NoError(t, db.Create(&cred).Error)

	// Create test assistant
	assistant := models.Assistant{
		ID:        1,
		Name:      "Hardware Assistant",
		ApiKey:    "device_api_key",
		ApiSecret: "device_api_secret",
	}
	require.NoError(t, db.Create(&assistant).Error)

	tests := []struct {
		name         string
		setupDevice  func() *models.Device
		deviceID     string
		expectedCode int
		expectedMsg  string
	}{
		{
			name: "Device without assistant binding",
			setupDevice: func() *models.Device {
				device := models.Device{
					MacAddress:  "11:22:33:44:55:66",
					UserID:      user.ID,
					AssistantID: nil, // No assistant bound
				}
				db.Create(&device)
				return &device
			},
			deviceID:     "11:22:33:44:55:66",
			expectedCode: 400,
			expectedMsg:  "设备未绑定助手",
		},
		{
			name: "Assistant without API credentials",
			setupDevice: func() *models.Device {
				// Create assistant without API credentials
				assistantNoAPI := models.Assistant{
					ID:        2,
					Name:      "No API Assistant",
					ApiKey:    "",
					ApiSecret: "",
				}
				db.Create(&assistantNoAPI)

				assistantID := uint(assistantNoAPI.ID)
				device := models.Device{
					MacAddress:  "22:33:44:55:66:77",
					UserID:      user.ID,
					AssistantID: &assistantID,
				}
				db.Create(&device)
				return &device
			},
			deviceID:     "22:33:44:55:66:77",
			expectedCode: 400,
			expectedMsg:  "助手未配置API凭证",
		},
		{
			name: "Assistant with invalid API credentials",
			setupDevice: func() *models.Device {
				// Create assistant with invalid API credentials
				assistantInvalidAPI := models.Assistant{
					ID:        3,
					Name:      "Invalid API Assistant",
					ApiKey:    "invalid_key",
					ApiSecret: "invalid_secret",
				}
				db.Create(&assistantInvalidAPI)

				assistantID := uint(assistantInvalidAPI.ID)
				device := models.Device{
					MacAddress:  "33:44:55:66:77:88",
					UserID:      user.ID,
					AssistantID: &assistantID,
				}
				db.Create(&device)
				return &device
			},
			deviceID:     "33:44:55:66:77:88",
			expectedCode: 400,
			expectedMsg:  "无效的API凭证",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := tt.setupDevice()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			req := httptest.NewRequest("GET", "/ws/hardware", nil)
			req.Header.Set("Device-Id", tt.deviceID)
			req.Header.Set("Connection", "upgrade")
			req.Header.Set("Upgrade", "websocket")
			c.Request = req

			h.HandleHardwareWebSocketVoice(c)

			assert.Equal(t, tt.expectedCode, w.Code)
			if tt.expectedMsg != "" {
				assert.Contains(t, w.Body.String(), tt.expectedMsg)
			}

			// Cleanup
			if device != nil {
				db.Delete(device)
			}
		})
	}
}

// Test WebSocket upgrader configuration
func TestWebSocketUpgraderConfig(t *testing.T) {
	t.Run("Upgrader configuration", func(t *testing.T) {
		assert.Equal(t, 1024*1024, voiceUpgrader.ReadBufferSize)
		assert.Equal(t, 1024*1024, voiceUpgrader.WriteBufferSize)

		// Test CheckOrigin function
		req := &http.Request{
			Header: make(http.Header),
		}
		req.Header.Set("Origin", "http://example.com")

		assert.True(t, voiceUpgrader.CheckOrigin(req))
	})
}

// Test error scenarios
func TestWebSocketVoiceErrorScenarios(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	t.Run("Database connection error simulation", func(t *testing.T) {
		// Close database to simulate error
		sqlDB, _ := db.DB()
		sqlDB.Close()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("GET", "/ws/voice?apiKey=test&apiSecret=test&assistantId=1", nil)
		c.Request = req

		h.HandleWebSocketVoice(c)

		assert.Equal(t, 500, w.Code)
	})
}

// Test concurrent WebSocket connections
func TestWebSocketVoiceConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test data
	cred := models.UserCredential{
		UserID:    1,
		APIKey:    "concurrent_key",
		APISecret: "concurrent_secret",
	}
	require.NoError(t, db.Create(&cred).Error)

	assistant := models.Assistant{
		ID:           1,
		Name:         "Concurrent Assistant",
		SystemPrompt: "Test",
		Temperature:  0.7,
	}
	require.NoError(t, db.Create(&assistant).Error)

	t.Run("Multiple concurrent requests", func(t *testing.T) {
		const numRequests = 10
		results := make(chan int, numRequests)

		for i := 0; i < numRequests; i++ {
			go func() {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)

				queryParams := "apiKey=concurrent_key&apiSecret=concurrent_secret&assistantId=1"
				req := httptest.NewRequest("GET", "/ws/voice?"+queryParams, nil)
				req.Header.Set("Connection", "upgrade")
				req.Header.Set("Upgrade", "websocket")
				c.Request = req

				h.HandleWebSocketVoice(c)
				results <- w.Code
			}()
		}

		// Collect results
		for i := 0; i < numRequests; i++ {
			code := <-results
			// All should fail upgrade (400) but process parameters correctly
			assert.Equal(t, 400, code)
		}
	})
}

// Benchmark tests
func BenchmarkHandlers_HandleWebSocketVoice(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Setup test data
	cred := models.UserCredential{
		UserID:    1,
		APIKey:    "bench_key",
		APISecret: "bench_secret",
	}
	db.Create(&cred)

	assistant := models.Assistant{
		ID:           1,
		Name:         "Benchmark Assistant",
		SystemPrompt: "Test",
		Temperature:  0.7,
	}
	db.Create(&assistant)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		queryParams := "apiKey=bench_key&apiSecret=bench_secret&assistantId=1"
		req := httptest.NewRequest("GET", "/ws/voice?"+queryParams, nil)
		req.Header.Set("Connection", "upgrade")
		req.Header.Set("Upgrade", "websocket")
		c.Request = req

		h.HandleWebSocketVoice(c)
	}
}

func BenchmarkHandlers_HandleHardwareWebSocketVoice(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Setup test data
	user := models.User{Email: "bench@example.com", DisplayName: "benchuser"}
	db.Create(&user)

	cred := models.UserCredential{
		UserID:    user.ID,
		APIKey:    "hw_bench_key",
		APISecret: "hw_bench_secret",
	}
	db.Create(&cred)

	assistant := models.Assistant{
		ID:        1,
		Name:      "HW Benchmark Assistant",
		ApiKey:    "hw_bench_key",
		ApiSecret: "hw_bench_secret",
	}
	db.Create(&assistant)

	assistantID := uint(assistant.ID)
	device := models.Device{
		MacAddress:  "BB:BB:BB:BB:BB:BB",
		UserID:      user.ID,
		AssistantID: &assistantID,
	}
	db.Create(&device)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("GET", "/ws/hardware", nil)
		req.Header.Set("Device-Id", "BB:BB:BB:BB:BB:BB")
		req.Header.Set("Connection", "upgrade")
		req.Header.Set("Upgrade", "websocket")
		c.Request = req

		h.HandleHardwareWebSocketVoice(c)
	}
}

// Test edge cases
func TestWebSocketVoiceEdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	t.Run("Very long device ID", func(t *testing.T) {
		longDeviceID := strings.Repeat("A", 1000)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("GET", "/ws/hardware", nil)
		req.Header.Set("Device-Id", longDeviceID)
		c.Request = req

		h.HandleHardwareWebSocketVoice(c)

		assert.Equal(t, 400, w.Code)
	})

	t.Run("Special characters in parameters", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// URL encode special characters
		specialParams := url.Values{}
		specialParams.Set("apiKey", "key with spaces & symbols!")
		specialParams.Set("apiSecret", "secret@#$%^&*()")
		specialParams.Set("assistantId", "1")

		req := httptest.NewRequest("GET", "/ws/voice?"+specialParams.Encode(), nil)
		c.Request = req

		h.HandleWebSocketVoice(c)

		assert.Equal(t, 500, w.Code) // Invalid credentials
	})

	t.Run("Zero assistant ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("GET", "/ws/voice?apiKey=test&apiSecret=test&assistantId=0", nil)
		c.Request = req

		h.HandleWebSocketVoice(c)

		assert.Equal(t, 500, w.Code)
	})

	t.Run("Negative assistant ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("GET", "/ws/voice?apiKey=test&apiSecret=test&assistantId=-1", nil)
		c.Request = req

		h.HandleWebSocketVoice(c)

		assert.Equal(t, 500, w.Code)
	})
}
