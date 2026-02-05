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
	"gorm.io/gorm"
)

func setupNodePluginTestDB(t *testing.T) (*gorm.DB, func()) {
	db := setupTestDB(t)

	// Auto migrate tables
	err := db.AutoMigrate(
		&models.User{},
		&models.NodePlugin{},
		&models.NodePluginVersion{},
		&models.NodePluginReview{},
		&models.NodePluginInstallation{},
	)
	assert.NoError(t, err)

	cleanup := func() {
		db.Exec("DROP TABLE IF EXISTS users")
		db.Exec("DROP TABLE IF EXISTS node_plugins")
		db.Exec("DROP TABLE IF EXISTS node_plugin_versions")
		db.Exec("DROP TABLE IF EXISTS node_plugin_reviews")
		db.Exec("DROP TABLE IF EXISTS node_plugin_installations")
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}

	return db, cleanup
}

func TestNodePluginHandler_CreatePlugin(t *testing.T) {
	db, cleanup := setupNodePluginTestDB(t)
	defer cleanup()

	// Create test user
	user := &models.User{
		Email:       "plugin@test.com",
		Password:    "password123",
		DisplayName: "pluginuser",
	}
	db.Create(user)

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		request    map[string]interface{}
		wantStatus int
		wantError  bool
	}{
		{
			name: "Valid plugin creation",
			request: map[string]interface{}{
				"name":        "test-plugin",
				"displayName": "Test Plugin",
				"description": "A test plugin",
				"category":    "utility",
				"icon":        "test-icon",
				"color":       "#FF0000",
				"tags":        []string{"test", "utility"},
				"definition": models.NodePluginDefinition{
					Type: "function",
					Inputs: []models.NodePluginPort{
						{Name: "param1", Type: "string", Required: true},
					},
					Outputs: []models.NodePluginPort{
						{Name: "result", Type: "string"},
					},
				},
				"schema": models.NodePluginSchema{
					Properties: map[string]models.SchemaProperty{
						"param1": {Type: "string", Description: "Input parameter"},
					},
					Required: []string{"param1"},
				},
				"author":     "Test Author",
				"homepage":   "https://example.com",
				"repository": "https://github.com/test/plugin",
				"license":    "MIT",
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name: "Missing required fields",
			request: map[string]interface{}{
				"name": "test-plugin",
			},
			wantStatus: http.StatusOK,
			wantError:  true,
		},
		{
			name: "Empty plugin name",
			request: map[string]interface{}{
				"name":        "",
				"displayName": "Test Plugin",
				"category":    "utility",
				"definition": models.NodePluginDefinition{
					Type: "function",
					Inputs: []models.NodePluginPort{
						{Name: "input1", Type: "string", Required: true},
					},
					Outputs: []models.NodePluginPort{
						{Name: "output1", Type: "string"},
					},
				},
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
			c.Request = httptest.NewRequest("POST", "/api/node-plugins", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("userID", user.ID)

			handler.CreatePlugin(c)

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

func TestNodePluginHandler_ListPlugins(t *testing.T) {
	db, cleanup := setupNodePluginTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "plugin@test.com",
		Password:    "password123",
		DisplayName: "pluginuser",
	}
	db.Create(user)

	// Create test plugins
	plugins := []models.NodePlugin{
		{
			UserID:      user.ID,
			Name:        "plugin-1",
			Slug:        "plugin-1",
			DisplayName: "Plugin 1",
			Description: "First plugin",
			Category:    models.NodePluginCategoryUtility,
			Version:     "1.0.0",
			Status:      models.NodePluginStatusPublished,
			Definition: models.NodePluginDefinition{
				Type: "function",
				Inputs: []models.NodePluginPort{
					{Name: "input1", Type: "string", Required: true},
				},
				Outputs: []models.NodePluginPort{
					{Name: "output1", Type: "string"},
				},
			},
		},
		{
			UserID:      user.ID,
			Name:        "plugin-2",
			Slug:        "plugin-2",
			DisplayName: "Plugin 2",
			Description: "Second plugin",
			Category:    models.NodePluginCategoryData,
			Version:     "1.0.0",
			Status:      models.NodePluginStatusDraft,
			Definition: models.NodePluginDefinition{
				Type: "function",
				Inputs: []models.NodePluginPort{
					{Name: "input2", Type: "string", Required: true},
				},
				Outputs: []models.NodePluginPort{
					{Name: "output2", Type: "string"},
				},
			},
		},
	}

	for _, plugin := range plugins {
		db.Create(&plugin)
	}

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		query      string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "List all published plugins",
			query:      "",
			wantCount:  1, // Only published plugins by default
			wantStatus: http.StatusOK,
		},
		{
			name:       "List plugins by category",
			query:      "?category=utility",
			wantCount:  1,
			wantStatus: http.StatusOK,
		},
		{
			name:       "List plugins by status",
			query:      "?status=draft",
			wantCount:  1,
			wantStatus: http.StatusOK,
		},
		{
			name:       "List plugins by user",
			query:      fmt.Sprintf("?userId=%d", user.ID),
			wantCount:  1, // Only published plugins
			wantStatus: http.StatusOK,
		},
		{
			name:       "Search plugins by keyword",
			query:      "?keyword=First",
			wantCount:  1,
			wantStatus: http.StatusOK,
		},
		{
			name:       "List with pagination",
			query:      "?page=1&pageSize=1",
			wantCount:  1,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", "/api/node-plugins"+tt.query, nil)

			handler.ListPlugins(c)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)

			assert.Equal(t, "success", resp["status"])
			data := resp["data"].(map[string]interface{})
			plugins := data["plugins"].([]interface{})
			assert.Equal(t, tt.wantCount, len(plugins))
		})
	}
}

func TestNodePluginHandler_GetPlugin(t *testing.T) {
	db, cleanup := setupNodePluginTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "plugin@test.com",
		Password:    "password123",
		DisplayName: "pluginuser",
	}
	db.Create(user)

	plugin := &models.NodePlugin{
		UserID:      user.ID,
		Name:        "test-plugin",
		Slug:        "test-plugin",
		DisplayName: "Test Plugin",
		Description: "A test plugin",
		Category:    models.NodePluginCategoryUtility,
		Version:     "1.0.0",
		Status:      models.NodePluginStatusPublished,
		Definition: models.NodePluginDefinition{
			Type: "function",
			Inputs: []models.NodePluginPort{
				{Name: "input1", Type: "string", Required: true},
			},
			Outputs: []models.NodePluginPort{
				{Name: "output1", Type: "string"},
			},
		},
	}
	db.Create(plugin)

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		pluginID   string
		wantStatus int
		wantError  bool
	}{
		{
			name:       "Valid plugin ID",
			pluginID:   fmt.Sprintf("%d", plugin.ID),
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:       "Invalid plugin ID",
			pluginID:   "invalid",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
		{
			name:       "Non-existent plugin ID",
			pluginID:   "99999",
			wantStatus: http.StatusOK,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", fmt.Sprintf("/api/node-plugins/%s", tt.pluginID), nil)
			c.Params = gin.Params{{Key: "id", Value: tt.pluginID}}

			handler.GetPlugin(c)

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

func TestNodePluginHandler_UpdatePlugin(t *testing.T) {
	db, cleanup := setupNodePluginTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "plugin@test.com",
		Password:    "password123",
		DisplayName: "pluginuser",
	}
	db.Create(user)

	plugin := &models.NodePlugin{
		UserID:      user.ID,
		Name:        "test-plugin",
		Slug:        "test-plugin",
		DisplayName: "Test Plugin",
		Description: "A test plugin",
		Category:    models.NodePluginCategoryUtility,
		Version:     "1.0.0",
		Status:      models.NodePluginStatusDraft,
		Definition: models.NodePluginDefinition{
			Type: "function",
			Inputs: []models.NodePluginPort{
				{Name: "input1", Type: "string", Required: true},
			},
			Outputs: []models.NodePluginPort{
				{Name: "output1", Type: "string"},
			},
		},
	}
	db.Create(plugin)

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	updateReq := map[string]interface{}{
		"displayName": "Updated Plugin",
		"description": "Updated description",
		"version":     "1.1.0",
		"changeLog":   "Updated functionality",
		"definition": models.NodePluginDefinition{
			Type: "function",
			Inputs: []models.NodePluginPort{
				{Name: "updatedInput", Type: "string", Required: true},
			},
			Outputs: []models.NodePluginPort{
				{Name: "updatedOutput", Type: "string"},
			},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(updateReq)
	c.Request = httptest.NewRequest("PUT", fmt.Sprintf("/api/node-plugins/%d", plugin.ID), bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", plugin.ID)}}
	c.Set("userID", user.ID)

	handler.UpdatePlugin(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])

	// Verify update
	var updated models.NodePlugin
	db.First(&updated, plugin.ID)
	assert.Equal(t, "Updated Plugin", updated.DisplayName)
	assert.Equal(t, "Updated description", updated.Description)
}

func TestNodePluginHandler_PublishPlugin(t *testing.T) {
	db, cleanup := setupNodePluginTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "plugin@test.com",
		Password:    "password123",
		DisplayName: "pluginuser",
	}
	db.Create(user)

	plugin := &models.NodePlugin{
		UserID:      user.ID,
		Name:        "test-plugin",
		Slug:        "test-plugin",
		DisplayName: "Test Plugin",
		Description: "A test plugin",
		Category:    models.NodePluginCategoryUtility,
		Version:     "1.0.0",
		Status:      models.NodePluginStatusDraft,
		Definition: models.NodePluginDefinition{
			Type: "function",
			Inputs: []models.NodePluginPort{
				{Name: "input1", Type: "string", Required: true},
			},
			Outputs: []models.NodePluginPort{
				{Name: "output1", Type: "string"},
			},
		},
	}
	db.Create(plugin)

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("POST", fmt.Sprintf("/api/node-plugins/%d/publish", plugin.ID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", plugin.ID)}}
	c.Set("userID", user.ID)

	handler.PublishPlugin(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])

	// Verify publication
	var published models.NodePlugin
	db.First(&published, plugin.ID)
	assert.Equal(t, models.NodePluginStatusPublished, published.Status)
}

func TestNodePluginHandler_InstallPlugin(t *testing.T) {
	db, cleanup := setupNodePluginTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "plugin@test.com",
		Password:    "password123",
		DisplayName: "pluginuser",
	}
	db.Create(user)

	plugin := &models.NodePlugin{
		UserID:      user.ID,
		Name:        "test-plugin",
		Slug:        "test-plugin",
		DisplayName: "Test Plugin",
		Description: "A test plugin",
		Category:    models.NodePluginCategoryUtility,
		Version:     "1.0.0",
		Status:      models.NodePluginStatusPublished,
		Definition: models.NodePluginDefinition{
			Type: "function",
			Inputs: []models.NodePluginPort{
				{Name: "input1", Type: "string", Required: true},
			},
			Outputs: []models.NodePluginPort{
				{Name: "output1", Type: "string"},
			},
		},
	}
	db.Create(plugin)

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	installReq := map[string]interface{}{
		"version": "1.0.0",
		"config": map[string]interface{}{
			"setting1": "value1",
			"setting2": "value2",
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, _ := json.Marshal(installReq)
	c.Request = httptest.NewRequest("POST", fmt.Sprintf("/api/node-plugins/%d/install", plugin.ID), bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", plugin.ID)}}
	c.Set("userID", user.ID)

	handler.InstallPlugin(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])

	// Verify installation
	var installation models.NodePluginInstallation
	err := db.Where("user_id = ? AND plugin_id = ?", user.ID, plugin.ID).First(&installation).Error
	assert.NoError(t, err)
	assert.Equal(t, "active", installation.Status)
}

func TestNodePluginHandler_DeletePlugin(t *testing.T) {
	db, cleanup := setupNodePluginTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "plugin@test.com",
		Password:    "password123",
		DisplayName: "pluginuser",
	}
	db.Create(user)

	plugin := &models.NodePlugin{
		UserID:      user.ID,
		Name:        "test-plugin",
		Slug:        "test-plugin",
		DisplayName: "Test Plugin",
		Description: "A test plugin",
		Category:    models.NodePluginCategoryUtility,
		Version:     "1.0.0",
		Status:      models.NodePluginStatusDraft,
		Definition: models.NodePluginDefinition{
			Type: "function",
			Inputs: []models.NodePluginPort{
				{Name: "input1", Type: "string", Required: true},
			},
			Outputs: []models.NodePluginPort{
				{Name: "output1", Type: "string"},
			},
		},
	}
	db.Create(plugin)

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("DELETE", fmt.Sprintf("/api/node-plugins/%d", plugin.ID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", plugin.ID)}}
	c.Set("userID", user.ID)

	handler.DeletePlugin(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])

	// Verify deletion
	var deleted models.NodePlugin
	err := db.First(&deleted, plugin.ID).Error
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestNodePluginHandler_ListInstalledPlugins(t *testing.T) {
	db, cleanup := setupNodePluginTestDB(t)
	defer cleanup()

	user := &models.User{
		Email:       "plugin@test.com",
		Password:    "password123",
		DisplayName: "pluginuser",
	}
	db.Create(user)

	plugin := &models.NodePlugin{
		UserID:      user.ID,
		Name:        "test-plugin",
		Slug:        "test-plugin",
		DisplayName: "Test Plugin",
		Description: "A test plugin",
		Category:    models.NodePluginCategoryUtility,
		Version:     "1.0.0",
		Status:      models.NodePluginStatusPublished,
		Definition: models.NodePluginDefinition{
			Type: "function",
			Inputs: []models.NodePluginPort{
				{Name: "input1", Type: "string", Required: true},
			},
			Outputs: []models.NodePluginPort{
				{Name: "output1", Type: "string"},
			},
		},
	}
	db.Create(plugin)

	// Create installation
	installation := &models.NodePluginInstallation{
		UserID:   user.ID,
		PluginID: plugin.ID,
		Version:  "1.0.0",
		Status:   "active",
		Config:   models.JSONMap{"setting": "value"},
	}
	db.Create(installation)

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/api/node-plugins/installed", nil)
	c.Set("userID", user.ID)

	handler.ListInstalledPlugins(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "success", resp["status"])
	data := resp["data"].([]interface{})
	assert.Equal(t, 1, len(data))
}

func TestNodePluginHandler_UnauthorizedAccess(t *testing.T) {
	db, cleanup := setupNodePluginTestDB(t)
	defer cleanup()

	user1 := &models.User{
		Email:       "user1@test.com",
		Password:    "password123",
		DisplayName: "user1",
	}
	db.Create(user1)

	user2 := &models.User{
		Email:       "user2@test.com",
		Password:    "password123",
		DisplayName: "user2",
	}
	db.Create(user2)

	// Plugin belongs to user1
	plugin := &models.NodePlugin{
		UserID:      user1.ID,
		Name:        "test-plugin",
		Slug:        "test-plugin",
		DisplayName: "Test Plugin",
		Description: "A test plugin",
		Category:    models.NodePluginCategoryUtility,
		Version:     "1.0.0",
		Status:      models.NodePluginStatusDraft,
		Definition: models.NodePluginDefinition{
			Type: "function",
			Inputs: []models.NodePluginPort{
				{Name: "input1", Type: "string", Required: true},
			},
			Outputs: []models.NodePluginPort{
				{Name: "output1", Type: "string"},
			},
		},
	}
	db.Create(plugin)

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	// Try to update with user2 (unauthorized)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	updateReq := map[string]interface{}{
		"displayName": "Unauthorized Update",
	}

	body, _ := json.Marshal(updateReq)
	c.Request = httptest.NewRequest("PUT", fmt.Sprintf("/api/node-plugins/%d", plugin.ID), bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", plugin.ID)}}
	c.Set("userID", user2.ID) // Wrong user

	handler.UpdatePlugin(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "fail", resp["status"])
	assert.Equal(t, "无权限操作", resp["message"])
}

func TestNodePluginHandler_NoUserID(t *testing.T) {
	db, cleanup := setupNodePluginTestDB(t)
	defer cleanup()

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	request := map[string]interface{}{
		"name":        "test-plugin",
		"displayName": "Test Plugin",
		"category":    "utility",
		"definition": models.NodePluginDefinition{
			Type: "function",
			Inputs: []models.NodePluginPort{
				{Name: "input1", Type: "string", Required: true},
			},
			Outputs: []models.NodePluginPort{
				{Name: "output1", Type: "string"},
			},
		},
	}

	body, _ := json.Marshal(request)
	c.Request = httptest.NewRequest("POST", "/api/node-plugins", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")
	// No userID set

	handler.CreatePlugin(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	assert.Equal(t, "fail", resp["status"])
	assert.Equal(t, "未授权", resp["message"])
}

// Benchmark tests
func BenchmarkNodePluginHandler_CreatePlugin(b *testing.B) {
	db, cleanup := setupNodePluginTestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "plugin@test.com",
		Password:    "password123",
		DisplayName: "pluginuser",
	}
	db.Create(user)

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	request := map[string]interface{}{
		"name":        "benchmark-plugin",
		"displayName": "Benchmark Plugin",
		"description": "A benchmark plugin",
		"category":    "utility",
		"definition": models.NodePluginDefinition{
			Type: "function",
			Inputs: []models.NodePluginPort{
				{Name: "input1", Type: "string", Required: true},
			},
			Outputs: []models.NodePluginPort{
				{Name: "output1", Type: "string"},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body, _ := json.Marshal(request)
		c.Request = httptest.NewRequest("POST", "/api/node-plugins", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("userID", user.ID)

		handler.CreatePlugin(c)
	}
}

func BenchmarkNodePluginHandler_ListPlugins(b *testing.B) {
	db, cleanup := setupNodePluginTestDB(&testing.T{})
	defer cleanup()

	user := &models.User{
		Email:       "plugin@test.com",
		Password:    "password123",
		DisplayName: "pluginuser",
	}
	db.Create(user)

	// Create test data
	for i := 0; i < 50; i++ {
		plugin := &models.NodePlugin{
			UserID:      user.ID,
			Name:        fmt.Sprintf("plugin-%d", i),
			Slug:        fmt.Sprintf("plugin-%d", i),
			DisplayName: fmt.Sprintf("Plugin %d", i),
			Description: fmt.Sprintf("Plugin %d description", i),
			Category:    models.NodePluginCategoryUtility,
			Version:     "1.0.0",
			Status:      models.NodePluginStatusPublished,
			Definition: models.NodePluginDefinition{
				Type: "function",
				Inputs: []models.NodePluginPort{
					{Name: fmt.Sprintf("input%d", i), Type: "string", Required: true},
				},
				Outputs: []models.NodePluginPort{
					{Name: fmt.Sprintf("output%d", i), Type: "string"},
				},
			},
		}
		db.Create(plugin)
	}

	handler := NewNodePluginHandler(db)
	gin.SetMode(gin.TestMode)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", "/api/node-plugins?page=1&pageSize=20", nil)

		handler.ListPlugins(c)
	}
}
