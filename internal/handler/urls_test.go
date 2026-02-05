package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/code-100-precent/LingEcho/pkg/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestNewHandlers(t *testing.T) {
	db := setupTestDB(t)

	handlers := NewHandlers(db)

	assert.NotNil(t, handlers)
	assert.NotNil(t, handlers.db)
	assert.NotNil(t, handlers.wsHub)
	assert.NotNil(t, handlers.searchHandler)
	assert.NotNil(t, handlers.ipLocationService)
	assert.NotNil(t, handlers.sipHandler)
}

func TestHandlers_SetSipServer(t *testing.T) {
	db := setupTestDB(t)

	handlers := NewHandlers(db)

	// Mock SIP server
	mockSipServer := &MockSipServer{}

	handlers.SetSipServer(mockSipServer)

	assert.Equal(t, mockSipServer, handlers.sipHandler.sipServer)
}

func TestHandlers_GetSearchHandler(t *testing.T) {
	db := setupTestDB(t)

	handlers := NewHandlers(db)

	searchHandler := handlers.GetSearchHandler()
	assert.NotNil(t, searchHandler)
	assert.Equal(t, handlers.searchHandler, searchHandler)
}

func TestHandlers_Register(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()

	handlers := NewHandlers(db)

	// This should not panic
	assert.NotPanics(t, func() {
		handlers.Register(engine)
	})

	// Verify some routes are registered
	routes := engine.Routes()
	assert.Greater(t, len(routes), 0)

	// Check for some expected routes
	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Path] = true
	}

	// These routes should be registered
	expectedRoutes := []string{
		"/api/system/health",
		"/api/system/status",
		"/api/notification",
		"/api/group",
		"/api/assistant",
	}

	for _, expectedRoute := range expectedRoutes {
		assert.True(t, routePaths[expectedRoute], "Route %s should be registered", expectedRoute)
	}
}

func TestRegisterNotificationRoutes(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := engine.Group("/api")

	handlers := NewHandlers(db)
	handlers.registerNotificationRoutes(r)

	routes := engine.Routes()

	// Check notification routes
	notificationRoutes := []string{
		"/api/notification/unread-count",
		"/api/notification",
		"/api/notification/readAll",
		"/api/notification/read/:id",
		"/api/notification/:id",
		"/api/notification/batch-delete",
		"/api/notification/all-ids",
	}

	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Path] = true
	}

	for _, expectedRoute := range notificationRoutes {
		assert.True(t, routePaths[expectedRoute], "Notification route %s should be registered", expectedRoute)
	}
}

func TestRegisterSystemRoutes(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := engine.Group("/api")

	handlers := NewHandlers(db)
	handlers.registerSystemRoutes(r)

	routes := engine.Routes()

	// Check system routes
	systemRoutes := []string{
		"/api/system/rate-limiter/config",
		"/api/system/health",
		"/api/system/status",
		"/api/system/dashboard/metrics",
		"/api/system/init",
		"/api/system/voice-clone/config",
		"/api/system/voiceprint/config",
		"/api/voiceprint",
		"/api/system/search/status",
		"/api/system/search/config",
		"/api/system/search/enable",
		"/api/system/search/disable",
	}

	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Path] = true
	}

	for _, expectedRoute := range systemRoutes {
		assert.True(t, routePaths[expectedRoute], "System route %s should be registered", expectedRoute)
	}
}

func TestRegisterOTARoutes(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := engine.Group("/api")

	handlers := NewHandlers(db)
	handlers.registerOTARoutes(r)

	routes := engine.Routes()

	// Check OTA routes
	otaRoutes := []string{
		"/api/ota/",
		"/api/ota/activate",
	}

	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Path] = true
	}

	for _, expectedRoute := range otaRoutes {
		assert.True(t, routePaths[expectedRoute], "OTA route %s should be registered", expectedRoute)
	}
}

func TestRegisterDeviceRoutes(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := engine.Group("/api")

	handlers := NewHandlers(db)
	handlers.registerDeviceRoutes(r)

	routes := engine.Routes()

	// Check device routes
	deviceRoutes := []string{
		"/api/device/config/:deviceId",
		"/api/device/bind/:agentId/:deviceCode",
		"/api/device/bind/:agentId",
		"/api/device/unbind",
		"/api/device/update/:id",
		"/api/device/manual-add",
		"/api/device/:deviceId",
		"/api/device/:deviceId/error-logs",
		"/api/device/:deviceId/performance-history",
		"/api/device/call-recordings",
		"/api/device/call-recordings/:id",
	}

	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Path] = true
	}

	for _, expectedRoute := range deviceRoutes {
		assert.True(t, routePaths[expectedRoute], "Device route %s should be registered", expectedRoute)
	}
}

func TestRegisterGroupRoutes(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := engine.Group("/api")

	handlers := NewHandlers(db)
	handlers.registerGroupRoutes(r)

	routes := engine.Routes()

	// Check group routes
	groupRoutes := []string{
		"/api/group",
		"/api/group/search-users",
		"/api/group/invitations",
		"/api/group/invitations/:id/accept",
		"/api/group/invitations/:id/reject",
		"/api/group/:id/overview/config",
		"/api/group/:id/statistics",
		"/api/group/:id/leave",
		"/api/group/:id/members/:memberId",
		"/api/group/:id/members/:memberId/role",
		"/api/group/:id/invite",
		"/api/group/:id/resources",
		"/api/group/:id/avatar",
		"/api/group/:id",
	}

	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Path] = true
	}

	for _, expectedRoute := range groupRoutes {
		assert.True(t, routePaths[expectedRoute], "Group route %s should be registered", expectedRoute)
	}
}

func TestRegisterAssistantRoutes(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := engine.Group("/api")

	handlers := NewHandlers(db)
	handlers.registerAssistantRoutes(r)

	routes := engine.Routes()

	// Check assistant routes
	assistantRoutes := []string{
		"/api/assistant/add",
		"/api/assistant",
		"/api/assistant/:id",
		"/api/assistant/:id/graph",
		"/api/assistant/:id/js",
		"/api/assistant/lingecho/client/:id/loader.js",
		"/api/assistant/:id/tools",
	}

	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Path] = true
	}

	for _, expectedRoute := range assistantRoutes {
		assert.True(t, routePaths[expectedRoute], "Assistant route %s should be registered", expectedRoute)
	}
}

func TestRegisterJSTemplateRoutes(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := engine.Group("/api")

	handlers := NewHandlers(db)
	handlers.registerJSTemplateRoutes(r)

	routes := engine.Routes()

	// Check JS template routes
	jsTemplateRoutes := []string{
		"/api/js-templates",
		"/api/js-templates/:id",
		"/api/js-templates/name/:name",
		"/api/js-templates/default",
		"/api/js-templates/custom",
		"/api/js-templates/search",
		"/api/js-templates/:id/versions",
		"/api/js-templates/:id/versions/:versionId",
		"/api/js-templates/:id/versions/:versionId/rollback",
		"/api/js-templates/:id/versions/:versionId/publish",
		"/api/js-templates/webhook/:jsSourceId",
	}

	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Path] = true
	}

	for _, expectedRoute := range jsTemplateRoutes {
		assert.True(t, routePaths[expectedRoute], "JS template route %s should be registered", expectedRoute)
	}
}

func TestRegisterWebSocketRoutes(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := engine.Group("/api")

	handlers := NewHandlers(db)
	handlers.registerWebSocketRoutes(r)

	routes := engine.Routes()

	// Check WebSocket routes
	wsRoutes := []string{
		"/api/ws",
		"/api/voice/websocket",
		"/api/ws/stats",
		"/api/ws/health",
		"/api/ws/user/:user_id",
		"/api/ws/group/:group",
		"/api/ws/message",
		"/api/ws/broadcast",
	}

	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Path] = true
	}

	for _, expectedRoute := range wsRoutes {
		assert.True(t, routePaths[expectedRoute], "WebSocket route %s should be registered", expectedRoute)
	}
}

func TestRegisterWorkflowRoutes(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := engine.Group("/api")

	handlers := NewHandlers(db)
	handlers.registerWorkflowRoutes(r)

	routes := engine.Routes()

	// Check workflow routes
	workflowRoutes := []string{
		"/api/workflows/definitions",
		"/api/workflows/definitions/:id",
		"/api/workflows/definitions/:id/run",
		"/api/workflows/definitions/:id/nodes/:nodeId/test",
		"/api/workflows/events/types",
		"/api/workflows/definitions/:id/versions",
		"/api/workflows/definitions/:id/versions/:versionId",
		"/api/workflows/definitions/:id/versions/:versionId/rollback",
		"/api/workflows/definitions/:id/versions/compare",
	}

	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Path] = true
	}

	for _, expectedRoute := range workflowRoutes {
		assert.True(t, routePaths[expectedRoute], "Workflow route %s should be registered", expectedRoute)
	}
}

// Mock SIP server for testing - using existing MockSipServer from sip_test.go

// Test route registration with middleware
func TestRouteRegistrationWithMiddleware(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()

	handlers := NewHandlers(db)
	handlers.Register(engine)

	// Test that routes are accessible
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/system/health", nil)
	engine.ServeHTTP(w, req)

	// Health endpoint should be accessible without auth
	assert.Equal(t, http.StatusOK, w.Code)
}

// Test search handler initialization
func TestSearchHandlerInitialization(t *testing.T) {
	db := setupTestDB(t)

	// Test with search disabled
	handlers := NewHandlers(db)
	assert.NotNil(t, handlers.searchHandler)

	// Test search handler database connection
	searchHandler := handlers.GetSearchHandler()
	assert.NotNil(t, searchHandler)
}

// Test WebSocket hub initialization
func TestWebSocketHubInitialization(t *testing.T) {
	db := setupTestDB(t)

	handlers := NewHandlers(db)
	assert.NotNil(t, handlers.wsHub)
	assert.IsType(t, &websocket.Hub{}, handlers.wsHub)
}

// Test IP location service initialization
func TestIPLocationServiceInitialization(t *testing.T) {
	db := setupTestDB(t)

	handlers := NewHandlers(db)
	assert.NotNil(t, handlers.ipLocationService)
}

// Test SIP handler initialization
func TestSipHandlerInitialization(t *testing.T) {
	db := setupTestDB(t)

	handlers := NewHandlers(db)
	assert.NotNil(t, handlers.sipHandler)
	assert.Equal(t, db, handlers.sipHandler.db)
}

// Benchmark route registration
func BenchmarkHandlersRegister(b *testing.B) {
	db := setupTestDB(&testing.T{})

	gin.SetMode(gin.TestMode)
	handlers := NewHandlers(db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine := gin.New()
		handlers.Register(engine)
	}
}

// Test concurrent handler creation
func TestConcurrentHandlerCreation(t *testing.T) {
	db := setupTestDB(t)

	const numGoroutines = 10
	results := make(chan *Handlers, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			handlers := NewHandlers(db)
			results <- handlers
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		handlers := <-results
		assert.NotNil(t, handlers)
		assert.NotNil(t, handlers.db)
		assert.NotNil(t, handlers.wsHub)
		assert.NotNil(t, handlers.searchHandler)
		assert.NotNil(t, handlers.ipLocationService)
		assert.NotNil(t, handlers.sipHandler)
	}
}

// Test edge cases
func TestHandlersEdgeCases(t *testing.T) {
	t.Run("Nil database", func(t *testing.T) {
		// Should not panic with nil database
		assert.NotPanics(t, func() {
			handlers := NewHandlers(nil)
			assert.NotNil(t, handlers)
		})
	})

	t.Run("Multiple register calls", func(t *testing.T) {
		db := setupTestDB(t)

		gin.SetMode(gin.TestMode)
		engine := gin.New()
		handlers := NewHandlers(db)

		// Should not panic when registering multiple times
		assert.NotPanics(t, func() {
			handlers.Register(engine)
			handlers.Register(engine)
		})
	})
}

// Test route parameter validation
func TestRouteParameterValidation(t *testing.T) {
	db := setupTestDB(t)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handlers := NewHandlers(db)
	handlers.Register(engine)

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "Valid health check",
			method:         "GET",
			path:           "/api/system/health",
			expectedStatus: 200,
		},
		{
			name:           "Invalid route",
			method:         "GET",
			path:           "/api/invalid/route",
			expectedStatus: 404,
		},
		{
			name:           "Valid system status",
			method:         "GET",
			path:           "/api/system/status",
			expectedStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			engine.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
