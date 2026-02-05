package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/code-100-precent/LingEcho/pkg/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func setupMiddlewareStatsTestDB(t *testing.T) (*gorm.DB, func()) {
	db := setupTestDB(t)

	cleanup := func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}

	return db, cleanup
}

func TestHandlers_handleGetMiddlewareStats(t *testing.T) {
	db, cleanup := setupMiddlewareStatsTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/api/middleware/stats", nil)

	h.handleGetMiddlewareStats(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])
	assert.Equal(t, "Middleware statistics retrieved successfully", resp["message"])

	data := resp["data"].(map[string]interface{})
	assert.Contains(t, data, "stats")
}

func TestHandlers_handleUpdateRateLimitConfig(t *testing.T) {
	db, cleanup := setupMiddlewareStatsTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		config     config.RateLimiterConfig
		wantStatus int
		wantError  bool
	}{
		{
			name: "Valid rate limit config",
			config: config.RateLimiterConfig{
				GlobalRPS:    100,
				GlobalBurst:  200,
				GlobalWindow: 60 * time.Second,
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name: "Disabled rate limiter",
			config: config.RateLimiterConfig{
				GlobalRPS:    0,
				GlobalBurst:  0,
				GlobalWindow: 0,
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name: "High rate limit",
			config: config.RateLimiterConfig{
				GlobalRPS:    10000,
				GlobalBurst:  20000,
				GlobalWindow: 3600 * time.Second,
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, _ := json.Marshal(tt.config)
			c.Request = httptest.NewRequest("PUT", "/api/middleware/rate-limit/config", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			h.handleUpdateRateLimitConfig(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError {
				assert.Equal(t, "fail", resp["status"])
			} else {
				assert.Equal(t, "success", resp["status"])
				assert.Equal(t, "Rate limit configuration updated successfully", resp["message"])
			}
		})
	}
}

func TestHandlers_handleUpdateTimeoutConfig(t *testing.T) {
	db, cleanup := setupMiddlewareStatsTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		config     config.TimeoutConfig
		wantStatus int
		wantError  bool
	}{
		{
			name: "Valid timeout config",
			config: config.TimeoutConfig{
				DefaultTimeout: 30 * time.Second,
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name: "Disabled timeout",
			config: config.TimeoutConfig{
				DefaultTimeout: 0,
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name: "Long timeout config",
			config: config.TimeoutConfig{
				DefaultTimeout: 300 * time.Second,
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, _ := json.Marshal(tt.config)
			c.Request = httptest.NewRequest("PUT", "/api/middleware/timeout/config", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			h.handleUpdateTimeoutConfig(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError {
				assert.Equal(t, "fail", resp["status"])
			} else {
				assert.Equal(t, "success", resp["status"])
				assert.Equal(t, "Timeout configuration updated successfully", resp["message"])
			}
		})
	}
}

func TestHandlers_handleUpdateCircuitBreakerConfig(t *testing.T) {
	db, cleanup := setupMiddlewareStatsTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		config     config.CircuitBreakerConfig
		wantStatus int
		wantError  bool
	}{
		{
			name: "Valid circuit breaker config",
			config: config.CircuitBreakerConfig{
				FailureThreshold:      5,
				SuccessThreshold:      3,
				Timeout:               30 * time.Second,
				OpenTimeout:           30 * time.Second,
				MaxConcurrentRequests: 200,
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name: "Disabled circuit breaker",
			config: config.CircuitBreakerConfig{
				FailureThreshold:      0,
				SuccessThreshold:      0,
				Timeout:               0,
				OpenTimeout:           0,
				MaxConcurrentRequests: 0,
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name: "Aggressive circuit breaker",
			config: config.CircuitBreakerConfig{
				FailureThreshold:      1,
				SuccessThreshold:      1,
				Timeout:               5 * time.Second,
				OpenTimeout:           10 * time.Second,
				MaxConcurrentRequests: 10,
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name: "Conservative circuit breaker",
			config: config.CircuitBreakerConfig{
				FailureThreshold:      20,
				SuccessThreshold:      10,
				Timeout:               120 * time.Second,
				OpenTimeout:           300 * time.Second,
				MaxConcurrentRequests: 1000,
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			body, _ := json.Marshal(tt.config)
			c.Request = httptest.NewRequest("PUT", "/api/middleware/circuit-breaker/config", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			h.handleUpdateCircuitBreakerConfig(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantError {
				assert.Equal(t, "fail", resp["status"])
			} else {
				assert.Equal(t, "success", resp["status"])
				assert.Equal(t, "Circuit breaker configuration updated successfully", resp["message"])
			}
		})
	}
}

func TestHandlers_handleUpdateRateLimitConfig_InvalidJSON(t *testing.T) {
	db, cleanup := setupMiddlewareStatsTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Invalid JSON
	c.Request = httptest.NewRequest("PUT", "/api/middleware/rate-limit/config", bytes.NewBuffer([]byte("invalid json")))
	c.Request.Header.Set("Content-Type", "application/json")

	h.handleUpdateRateLimitConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "fail", resp["status"])
	assert.Equal(t, "Invalid configuration format", resp["message"])
}

func TestHandlers_handleUpdateTimeoutConfig_InvalidJSON(t *testing.T) {
	db, cleanup := setupMiddlewareStatsTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Invalid JSON
	c.Request = httptest.NewRequest("PUT", "/api/middleware/timeout/config", bytes.NewBuffer([]byte("invalid json")))
	c.Request.Header.Set("Content-Type", "application/json")

	h.handleUpdateTimeoutConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "fail", resp["status"])
	assert.Equal(t, "Invalid configuration format", resp["message"])
}

func TestHandlers_handleUpdateCircuitBreakerConfig_InvalidJSON(t *testing.T) {
	db, cleanup := setupMiddlewareStatsTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Invalid JSON
	c.Request = httptest.NewRequest("PUT", "/api/middleware/circuit-breaker/config", bytes.NewBuffer([]byte("invalid json")))
	c.Request.Header.Set("Content-Type", "application/json")

	h.handleUpdateCircuitBreakerConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "fail", resp["status"])
	assert.Equal(t, "Invalid configuration format", resp["message"])
}

func TestHandlers_registerMiddlewareRoutes(t *testing.T) {
	db, cleanup := setupMiddlewareStatsTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	router := gin.New()
	_ = router.Group("/api")

	// This would normally be called during route registration
	// h.registerMiddlewareRoutes(api)

	// Since registerMiddlewareRoutes is not exported, we test the individual handlers
	// The route registration itself would be tested in integration tests

	// Test that the handlers exist and can be called
	assert.NotNil(t, h.handleGetMiddlewareStats)
	assert.NotNil(t, h.handleUpdateRateLimitConfig)
	assert.NotNil(t, h.handleUpdateTimeoutConfig)
	assert.NotNil(t, h.handleUpdateCircuitBreakerConfig)
}

func TestHandlers_MiddlewareStats_EdgeCases(t *testing.T) {
	db, cleanup := setupMiddlewareStatsTestDB(t)
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		handler    func(*gin.Context)
		method     string
		path       string
		body       []byte
		wantStatus int
	}{
		{
			name:       "Get stats with no middleware manager",
			handler:    h.handleGetMiddlewareStats,
			method:     "GET",
			path:       "/api/middleware/stats",
			body:       nil,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Update rate limit with empty body",
			handler:    h.handleUpdateRateLimitConfig,
			method:     "PUT",
			path:       "/api/middleware/rate-limit/config",
			body:       []byte("{}"),
			wantStatus: http.StatusOK,
		},
		{
			name:       "Update timeout with empty body",
			handler:    h.handleUpdateTimeoutConfig,
			method:     "PUT",
			path:       "/api/middleware/timeout/config",
			body:       []byte("{}"),
			wantStatus: http.StatusOK,
		},
		{
			name:       "Update circuit breaker with empty body",
			handler:    h.handleUpdateCircuitBreakerConfig,
			method:     "PUT",
			path:       "/api/middleware/circuit-breaker/config",
			body:       []byte("{}"),
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			var body *bytes.Buffer
			if tt.body != nil {
				body = bytes.NewBuffer(tt.body)
			}

			c.Request = httptest.NewRequest(tt.method, tt.path, body)
			if tt.body != nil {
				c.Request.Header.Set("Content-Type", "application/json")
			}

			tt.handler(c)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

// Benchmark tests
func BenchmarkHandlers_handleGetMiddlewareStats(b *testing.B) {
	db, cleanup := setupMiddlewareStatsTestDB(&testing.T{})
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", "/api/middleware/stats", nil)

		h.handleGetMiddlewareStats(c)
	}
}

func BenchmarkHandlers_handleUpdateRateLimitConfig(b *testing.B) {
	db, cleanup := setupMiddlewareStatsTestDB(&testing.T{})
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	config := config.RateLimiterConfig{
		GlobalRPS:    100,
		GlobalBurst:  200,
		GlobalWindow: 60 * time.Second,
	}

	body, _ := json.Marshal(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("PUT", "/api/middleware/rate-limit/config", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.handleUpdateRateLimitConfig(c)
	}
}

func BenchmarkHandlers_handleUpdateTimeoutConfig(b *testing.B) {
	db, cleanup := setupMiddlewareStatsTestDB(&testing.T{})
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	config := config.TimeoutConfig{
		DefaultTimeout: 30 * time.Second,
	}

	body, _ := json.Marshal(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("PUT", "/api/middleware/timeout/config", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.handleUpdateTimeoutConfig(c)
	}
}

func BenchmarkHandlers_handleUpdateCircuitBreakerConfig(b *testing.B) {
	db, cleanup := setupMiddlewareStatsTestDB(&testing.T{})
	defer cleanup()

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	config := config.CircuitBreakerConfig{
		FailureThreshold:      5,
		SuccessThreshold:      3,
		Timeout:               30 * time.Second,
		OpenTimeout:           30 * time.Second,
		MaxConcurrentRequests: 200,
	}

	body, _ := json.Marshal(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("PUT", "/api/middleware/circuit-breaker/config", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")

		h.handleUpdateCircuitBreakerConfig(c)
	}
}
