package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowPluginHandler_PublishWorkflowAsPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewWorkflowPluginHandler(db)

	// Create test user
	user := models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(&user).Error)

	// Create test workflow
	workflow := models.WorkflowDefinition{
		UserID:      user.ID,
		Name:        "Test Workflow",
		Slug:        "test-workflow",
		Description: "Test workflow description",
		Status:      "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
				{ID: "end", Name: "End", Type: "end"},
			},
			Edges: []models.WorkflowEdgeSchema{
				{ID: "edge1", Source: "start", Target: "end"},
			},
		},
	}
	require.NoError(t, db.Create(&workflow).Error)

	tests := []struct {
		name         string
		workflowID   string
		userID       uint
		request      map[string]interface{}
		expectedCode int
	}{
		{
			name:       "Valid plugin publication",
			workflowID: strconv.Itoa(int(workflow.ID)),
			userID:     user.ID,
			request: map[string]interface{}{
				"name":        "Test Plugin",
				"displayName": "Test Plugin Display",
				"description": "Test plugin description",
				"category":    "automation",
				"icon":        "icon-test",
				"color":       "#FF0000",
				"tags":        []string{"test", "automation"},
				"inputSchema": models.WorkflowPluginIOSchema{
					Parameters: []models.WorkflowPluginParameter{
						{
							Name:        "input",
							Type:        "string",
							Required:    true,
							Description: "Input parameter",
						},
					},
				},
				"outputSchema": models.WorkflowPluginIOSchema{
					Parameters: []models.WorkflowPluginParameter{
						{
							Name:        "output",
							Type:        "string",
							Required:    true,
							Description: "Output parameter",
						},
					},
				},
				"author":     "Test Author",
				"homepage":   "https://example.com",
				"repository": "https://github.com/test/repo",
				"license":    "MIT",
			},
			expectedCode: 200,
		},
		{
			name:       "Missing required fields",
			workflowID: strconv.Itoa(int(workflow.ID)),
			userID:     user.ID,
			request: map[string]interface{}{
				"name": "Test Plugin",
				// Missing displayName and category
			},
			expectedCode: 500,
		},
		{
			name:         "Invalid workflow ID",
			workflowID:   "invalid",
			userID:       user.ID,
			request:      map[string]interface{}{},
			expectedCode: 500,
		},
		{
			name:       "Non-existent workflow",
			workflowID: "99999",
			userID:     user.ID,
			request: map[string]interface{}{
				"name":        "Test Plugin",
				"displayName": "Test Plugin Display",
				"category":    "automation",
			},
			expectedCode: 500,
		},
		{
			name:       "Unauthorized user",
			workflowID: strconv.Itoa(int(workflow.ID)),
			userID:     999, // Different user
			request: map[string]interface{}{
				"name":        "Test Plugin",
				"displayName": "Test Plugin Display",
				"category":    "automation",
			},
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "workflowId", Value: tt.workflowID}}

			// Mock getUserID function
			c.Set("userID", tt.userID)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/workflows/"+tt.workflowID+"/publish", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			handler.PublishWorkflowAsPlugin(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                   `json:"code"`
					Data models.WorkflowPlugin `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.request["name"], response.Data.Name)
				assert.Equal(t, tt.request["displayName"], response.Data.DisplayName)
			}
		})
	}
}
func TestWorkflowPluginHandler_ListWorkflowPlugins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewWorkflowPluginHandler(db)

	// Create test plugins
	plugin1 := models.WorkflowPlugin{
		UserID:      1,
		Name:        "Plugin One",
		DisplayName: "Plugin One Display",
		Category:    models.WorkflowPluginCategoryUtility,
		Status:      models.WorkflowPluginStatusPublished,
		Description: "First test plugin",
	}
	plugin2 := models.WorkflowPlugin{
		UserID:      2,
		Name:        "Plugin Two",
		DisplayName: "Plugin Two Display",
		Category:    models.WorkflowPluginCategoryAPIIntegration,
		Status:      models.WorkflowPluginStatusDraft,
		Description: "Second test plugin",
	}
	require.NoError(t, db.Create(&plugin1).Error)
	require.NoError(t, db.Create(&plugin2).Error)

	tests := []struct {
		name          string
		queryParams   map[string]string
		expectedCode  int
		expectedCount int
	}{
		{
			name:          "List all plugins",
			queryParams:   map[string]string{},
			expectedCode:  200,
			expectedCount: 2,
		},
		{
			name: "Filter by category",
			queryParams: map[string]string{
				"category": string(models.WorkflowPluginCategoryUtility),
			},
			expectedCode:  200,
			expectedCount: 1,
		},
		{
			name: "Filter by status",
			queryParams: map[string]string{
				"status": string(models.WorkflowPluginStatusPublished),
			},
			expectedCode:  200,
			expectedCount: 1,
		},
		{
			name: "Filter by user",
			queryParams: map[string]string{
				"userId": "1",
			},
			expectedCode:  200,
			expectedCount: 1,
		},
		{
			name: "Search by keyword",
			queryParams: map[string]string{
				"keyword": "One",
			},
			expectedCode:  200,
			expectedCount: 1,
		},
		{
			name: "Pagination",
			queryParams: map[string]string{
				"page":     "1",
				"pageSize": "1",
			},
			expectedCode:  200,
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Set query parameters
			for k, v := range tt.queryParams {
				c.Request = httptest.NewRequest("GET", "/plugins?"+k+"="+v, nil)
			}
			if len(tt.queryParams) == 0 {
				c.Request = httptest.NewRequest("GET", "/plugins", nil)
			}

			handler.ListWorkflowPlugins(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int `json:"code"`
					Data struct {
						Plugins  []models.WorkflowPlugin `json:"plugins"`
						Total    int64                   `json:"total"`
						Page     int                     `json:"page"`
						PageSize int                     `json:"pageSize"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.expectedCount, len(response.Data.Plugins))
			}
		})
	}
}

func TestWorkflowPluginHandler_GetWorkflowPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewWorkflowPluginHandler(db)

	// Create test plugin
	plugin := models.WorkflowPlugin{
		UserID:      1,
		Name:        "Test Plugin",
		DisplayName: "Test Plugin Display",
		Category:    models.WorkflowPluginCategoryUtility,
		Status:      models.WorkflowPluginStatusPublished,
	}
	require.NoError(t, db.Create(&plugin).Error)

	tests := []struct {
		name         string
		pluginID     string
		expectedCode int
	}{
		{
			name:         "Valid plugin ID",
			pluginID:     strconv.Itoa(int(plugin.ID)),
			expectedCode: 200,
		},
		{
			name:         "Invalid plugin ID",
			pluginID:     "invalid",
			expectedCode: 500,
		},
		{
			name:         "Non-existent plugin",
			pluginID:     "99999",
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "id", Value: tt.pluginID}}

			req := httptest.NewRequest("GET", "/plugins/"+tt.pluginID, nil)
			c.Request = req

			handler.GetWorkflowPlugin(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                   `json:"code"`
					Data models.WorkflowPlugin `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, plugin.Name, response.Data.Name)
			}
		})
	}
}

func TestWorkflowPluginHandler_UpdateWorkflowPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewWorkflowPluginHandler(db)

	// Create test plugin
	plugin := models.WorkflowPlugin{
		UserID:      1,
		Name:        "Original Plugin",
		DisplayName: "Original Display Name",
		Category:    models.WorkflowPluginCategoryUtility,
		Status:      models.WorkflowPluginStatusDraft,
		Version:     "1.0.0",
	}
	require.NoError(t, db.Create(&plugin).Error)

	tests := []struct {
		name         string
		pluginID     string
		userID       uint
		request      map[string]interface{}
		expectedCode int
	}{
		{
			name:     "Valid update",
			pluginID: strconv.Itoa(int(plugin.ID)),
			userID:   1,
			request: map[string]interface{}{
				"displayName": "Updated Display Name",
				"description": "Updated description",
				"version":     "1.1.0",
				"changeLog":   "Updated features",
			},
			expectedCode: 200,
		},
		{
			name:     "Unauthorized user",
			pluginID: strconv.Itoa(int(plugin.ID)),
			userID:   999,
			request: map[string]interface{}{
				"displayName": "Updated Display Name",
			},
			expectedCode: 500,
		},
		{
			name:         "Invalid plugin ID",
			pluginID:     "invalid",
			userID:       1,
			request:      map[string]interface{}{},
			expectedCode: 500,
		},
		{
			name:     "Non-existent plugin",
			pluginID: "99999",
			userID:   1,
			request: map[string]interface{}{
				"displayName": "Updated Display Name",
			},
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "id", Value: tt.pluginID}}
			c.Set("userID", tt.userID)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("PUT", "/plugins/"+tt.pluginID, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			handler.UpdateWorkflowPlugin(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

func TestWorkflowPluginHandler_PublishWorkflowPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewWorkflowPluginHandler(db)

	// Create test plugin
	plugin := models.WorkflowPlugin{
		UserID:      1,
		Name:        "Draft Plugin",
		DisplayName: "Draft Plugin Display",
		Status:      models.WorkflowPluginStatusDraft,
	}
	require.NoError(t, db.Create(&plugin).Error)

	tests := []struct {
		name         string
		pluginID     string
		userID       uint
		expectedCode int
	}{
		{
			name:         "Valid publish",
			pluginID:     strconv.Itoa(int(plugin.ID)),
			userID:       1,
			expectedCode: 200,
		},
		{
			name:         "Unauthorized user",
			pluginID:     strconv.Itoa(int(plugin.ID)),
			userID:       999,
			expectedCode: 500,
		},
		{
			name:         "Invalid plugin ID",
			pluginID:     "invalid",
			userID:       1,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "id", Value: tt.pluginID}}
			c.Set("userID", tt.userID)

			req := httptest.NewRequest("POST", "/plugins/"+tt.pluginID+"/publish", nil)
			c.Request = req

			handler.PublishWorkflowPlugin(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				// Verify status was updated
				var updated models.WorkflowPlugin
				db.First(&updated, plugin.ID)
				assert.Equal(t, models.WorkflowPluginStatusPublished, updated.Status)
			}
		})
	}
}
func TestWorkflowPluginHandler_InstallWorkflowPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewWorkflowPluginHandler(db)

	// Create test plugin
	plugin := models.WorkflowPlugin{
		UserID:      1,
		Name:        "Installable Plugin",
		DisplayName: "Installable Plugin Display",
		Status:      models.WorkflowPluginStatusPublished,
		Version:     "1.0.0",
	}
	require.NoError(t, db.Create(&plugin).Error)

	tests := []struct {
		name         string
		pluginID     string
		userID       uint
		request      map[string]interface{}
		expectedCode int
		setupFunc    func()
	}{
		{
			name:     "Valid installation",
			pluginID: strconv.Itoa(int(plugin.ID)),
			userID:   2, // Different user installing
			request: map[string]interface{}{
				"version": "1.0.0",
				"config": map[string]interface{}{
					"setting1": "value1",
					"setting2": "value2",
				},
			},
			expectedCode: 200,
		},
		{
			name:     "Installation without version (use latest)",
			pluginID: strconv.Itoa(int(plugin.ID)),
			userID:   3,
			request: map[string]interface{}{
				"config": map[string]interface{}{
					"setting1": "value1",
				},
			},
			expectedCode: 200,
		},
		{
			name:     "Duplicate installation",
			pluginID: strconv.Itoa(int(plugin.ID)),
			userID:   4,
			request: map[string]interface{}{
				"version": "1.0.0",
			},
			expectedCode: 500,
			setupFunc: func() {
				// Create existing installation
				existing := models.WorkflowPluginInstallation{
					UserID:   4,
					PluginID: plugin.ID,
					Version:  "1.0.0",
					Status:   "active",
				}
				db.Create(&existing)
			},
		},
		{
			name:         "Non-existent plugin",
			pluginID:     "99999",
			userID:       5,
			request:      map[string]interface{}{},
			expectedCode: 500,
		},
		{
			name:         "Invalid plugin ID",
			pluginID:     "invalid",
			userID:       5,
			request:      map[string]interface{}{},
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFunc != nil {
				tt.setupFunc()
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "id", Value: tt.pluginID}}
			c.Set("userID", tt.userID)

			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/plugins/"+tt.pluginID+"/install", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			handler.InstallWorkflowPlugin(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				// Verify installation was created
				var installation models.WorkflowPluginInstallation
				err := db.Where("user_id = ? AND plugin_id = ?", tt.userID, plugin.ID).First(&installation).Error
				assert.NoError(t, err)
				assert.Equal(t, "active", installation.Status)
			}
		})
	}
}

func TestWorkflowPluginHandler_ListInstalledWorkflowPlugins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewWorkflowPluginHandler(db)

	// Create test plugin
	plugin := models.WorkflowPlugin{
		UserID:      1,
		Name:        "Installed Plugin",
		DisplayName: "Installed Plugin Display",
		Status:      models.WorkflowPluginStatusPublished,
	}
	require.NoError(t, db.Create(&plugin).Error)

	// Create installation
	installation := models.WorkflowPluginInstallation{
		UserID:   2,
		PluginID: plugin.ID,
		Version:  "1.0.0",
		Status:   "active",
	}
	require.NoError(t, db.Create(&installation).Error)

	tests := []struct {
		name          string
		userID        uint
		expectedCode  int
		expectedCount int
	}{
		{
			name:          "User with installations",
			userID:        2,
			expectedCode:  200,
			expectedCount: 1,
		},
		{
			name:          "User without installations",
			userID:        999,
			expectedCode:  200,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("userID", tt.userID)

			req := httptest.NewRequest("GET", "/plugins/installed", nil)
			c.Request = req

			handler.ListInstalledWorkflowPlugins(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                                 `json:"code"`
					Data []models.WorkflowPluginInstallation `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Len(t, response.Data, tt.expectedCount)
			}
		})
	}
}

func TestWorkflowPluginHandler_DeleteWorkflowPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewWorkflowPluginHandler(db)

	// Create test plugin with related records
	plugin := models.WorkflowPlugin{
		UserID:      1,
		Name:        "Deletable Plugin",
		DisplayName: "Deletable Plugin Display",
		Status:      models.WorkflowPluginStatusDraft,
	}
	require.NoError(t, db.Create(&plugin).Error)

	// Create related records
	version := models.WorkflowPluginVersion{
		PluginID: plugin.ID,
		Version:  "1.0.0",
	}
	require.NoError(t, db.Create(&version).Error)

	installation := models.WorkflowPluginInstallation{
		UserID:   2,
		PluginID: plugin.ID,
		Version:  "1.0.0",
		Status:   "active",
	}
	require.NoError(t, db.Create(&installation).Error)

	tests := []struct {
		name         string
		pluginID     string
		userID       uint
		expectedCode int
	}{
		{
			name:         "Valid deletion",
			pluginID:     strconv.Itoa(int(plugin.ID)),
			userID:       1,
			expectedCode: 200,
		},
		{
			name:         "Unauthorized user",
			pluginID:     strconv.Itoa(int(plugin.ID)),
			userID:       999,
			expectedCode: 500,
		},
		{
			name:         "Invalid plugin ID",
			pluginID:     "invalid",
			userID:       1,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "id", Value: tt.pluginID}}
			c.Set("userID", tt.userID)

			req := httptest.NewRequest("DELETE", "/plugins/"+tt.pluginID, nil)
			c.Request = req

			handler.DeleteWorkflowPlugin(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				// Verify plugin and related records were deleted
				var count int64
				db.Model(&models.WorkflowPlugin{}).Where("id = ?", plugin.ID).Count(&count)
				assert.Equal(t, int64(0), count)

				db.Model(&models.WorkflowPluginVersion{}).Where("plugin_id = ?", plugin.ID).Count(&count)
				assert.Equal(t, int64(0), count)

				db.Model(&models.WorkflowPluginInstallation{}).Where("plugin_id = ?", plugin.ID).Count(&count)
				assert.Equal(t, int64(0), count)
			}
		})
	}
}

func TestWorkflowPluginHandler_GetUserWorkflowPlugins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewWorkflowPluginHandler(db)

	// Create test plugins for different users
	plugin1 := models.WorkflowPlugin{
		UserID:      1,
		Name:        "User 1 Plugin 1",
		DisplayName: "User 1 Plugin 1 Display",
	}
	plugin2 := models.WorkflowPlugin{
		UserID:      1,
		Name:        "User 1 Plugin 2",
		DisplayName: "User 1 Plugin 2 Display",
	}
	plugin3 := models.WorkflowPlugin{
		UserID:      2,
		Name:        "User 2 Plugin",
		DisplayName: "User 2 Plugin Display",
	}
	require.NoError(t, db.Create(&plugin1).Error)
	require.NoError(t, db.Create(&plugin2).Error)
	require.NoError(t, db.Create(&plugin3).Error)

	tests := []struct {
		name          string
		userID        uint
		expectedCode  int
		expectedCount int
	}{
		{
			name:          "User with multiple plugins",
			userID:        1,
			expectedCode:  200,
			expectedCount: 2,
		},
		{
			name:          "User with single plugin",
			userID:        2,
			expectedCode:  200,
			expectedCount: 1,
		},
		{
			name:          "User with no plugins",
			userID:        999,
			expectedCode:  200,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("userID", tt.userID)

			req := httptest.NewRequest("GET", "/plugins/user", nil)
			c.Request = req

			handler.GetUserWorkflowPlugins(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                     `json:"code"`
					Data []models.WorkflowPlugin `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Len(t, response.Data, tt.expectedCount)
			}
		})
	}
}

// Test helper function for getUserID - using existing function from plugin_utils.go

// Test generateSlug function - using existing function from plugin_utils.go

// Benchmark tests
func BenchmarkWorkflowPluginHandler_ListWorkflowPlugins(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	handler := NewWorkflowPluginHandler(db)

	// Create test data
	for i := 0; i < 100; i++ {
		plugin := models.WorkflowPlugin{
			UserID:      uint(i%10 + 1),
			Name:        fmt.Sprintf("Plugin %d", i),
			DisplayName: fmt.Sprintf("Plugin %d Display", i),
			Category:    models.WorkflowPluginCategoryUtility,
			Status:      models.WorkflowPluginStatusPublished,
		}
		db.Create(&plugin)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		req := httptest.NewRequest("GET", "/plugins", nil)
		c.Request = req

		handler.ListWorkflowPlugins(c)
	}
}

// Edge case tests
func TestWorkflowPluginHandler_EdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	handler := NewWorkflowPluginHandler(db)

	t.Run("Publish plugin with duplicate slug", func(t *testing.T) {
		// Create workflow
		workflow := models.WorkflowDefinition{
			UserID: 1,
			Name:   "Test Workflow",
			Definition: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
				},
			},
		}
		db.Create(&workflow)

		// Create existing plugin with same name
		existing := models.WorkflowPlugin{
			UserID: 1,
			Name:   "Duplicate Plugin",
			Slug:   "duplicate-plugin",
		}
		db.Create(&existing)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "workflowId", Value: strconv.Itoa(int(workflow.ID))}}
		c.Set("userID", uint(1))

		request := map[string]interface{}{
			"name":        "Duplicate Plugin",
			"displayName": "Duplicate Plugin Display",
			"category":    "automation",
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/workflows/1/publish", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		handler.PublishWorkflowAsPlugin(c)

		assert.Equal(t, 500, w.Code)
	})

	t.Run("Update plugin with new version creates version record", func(t *testing.T) {
		plugin := models.WorkflowPlugin{
			UserID:  1,
			Name:    "Versioned Plugin",
			Version: "1.0.0",
		}
		db.Create(&plugin)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "id", Value: strconv.Itoa(int(plugin.ID))}}
		c.Set("userID", uint(1))

		request := map[string]interface{}{
			"version":   "2.0.0",
			"changeLog": "Major update",
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("PUT", "/plugins/1", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		handler.UpdateWorkflowPlugin(c)

		assert.Equal(t, 200, w.Code)

		// Verify version record was created
		var version models.WorkflowPluginVersion
		err := db.Where("plugin_id = ? AND version = ?", plugin.ID, "2.0.0").First(&version).Error
		assert.NoError(t, err)
		assert.Equal(t, "Major update", version.ChangeLog)
	})
}
