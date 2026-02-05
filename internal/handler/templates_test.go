package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/constants"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCreateJSTemplate(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
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
		template       models.JSTemplate
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Valid custom template",
			template: models.JSTemplate{
				Name:    "Test Template",
				Type:    "custom",
				Content: "console.log('Hello World');",
				Status:  "active",
			},
			expectedStatus: 200,
		},
		{
			name: "Template with invalid content",
			template: models.JSTemplate{
				Name:    "Invalid Template",
				Type:    "custom",
				Content: "eval('malicious code');", // Should fail AST validation
			},
			expectedStatus: 400,
			expectedError:  "代码不符合安全规范",
		},
		{
			name: "Template with empty name",
			template: models.JSTemplate{
				Type:    "custom",
				Content: "console.log('test');",
			},
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Set up request
			jsonData, _ := json.Marshal(tt.template)
			c.Request = httptest.NewRequest("POST", "/js-templates", bytes.NewBuffer(jsonData))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", user)
			c.Set(constants.DbField, db)

			h.CreateJSTemplate(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Equal(t, "Template created successfully", response["msg"])

				// Verify template was created in database
				var created models.JSTemplate
				err = db.Where("name = ?", tt.template.Name).First(&created).Error
				require.NoError(t, err)
				assert.Equal(t, user.ID, created.UserID)
				assert.Equal(t, tt.template.Type, created.Type)
			}
		})
	}
}

func TestGetJSTemplate(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test template
	template := &models.JSTemplate{
		Name:    "Test Template",
		Type:    "custom",
		Content: "console.log('Hello World');",
		Status:  "active",
		UserID:  user.ID,
	}
	require.NoError(t, models.CreateJSTemplate(db, template))

	tests := []struct {
		name           string
		templateID     string
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid template ID",
			templateID:     template.ID,
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Invalid template ID",
			templateID:     "invalid-id",
			user:           user,
			expectedStatus: 400,
			expectedError:  "Template not found",
		},
		{
			name:           "No user",
			templateID:     template.ID,
			user:           nil,
			expectedStatus: 400,
			expectedError:  "User not logged in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", fmt.Sprintf("/js-templates/%s", tt.templateID), nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.templateID}}
			if tt.user != nil {
				c.Set("user", tt.user)
			}
			c.Set(constants.DbField, db)

			h.GetJSTemplate(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Equal(t, "Template retrieved successfully", response["msg"])
			}
		})
	}
}

func TestListJSTemplates(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test templates
	for i := 0; i < 5; i++ {
		template := &models.JSTemplate{
			Name:    fmt.Sprintf("Test Template %d", i),
			Type:    "custom",
			Content: "console.log('test');",
			Status:  "active",
			UserID:  user.ID,
		}
		require.NoError(t, models.CreateJSTemplate(db, template))
	}

	tests := []struct {
		name           string
		queryParams    string
		user           *models.User
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "List all templates",
			queryParams:    "",
			user:           user,
			expectedStatus: 200,
			expectedCount:  5,
		},
		{
			name:           "List with pagination",
			queryParams:    "?page=1&limit=3",
			user:           user,
			expectedStatus: 200,
			expectedCount:  3,
		},
		{
			name:           "No user",
			queryParams:    "",
			user:           nil,
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", "/js-templates"+tt.queryParams, nil)
			if tt.user != nil {
				c.Set("user", tt.user)
			}
			c.Set(constants.DbField, db)

			h.ListJSTemplates(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data := response["data"].(map[string]interface{})
				templates := data["data"].([]interface{})
				assert.Equal(t, tt.expectedCount, len(templates))
			}
		})
	}
}

func TestUpdateJSTemplate(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test template
	template := &models.JSTemplate{
		Name:    "Test Template",
		Type:    "custom",
		Content: "console.log('Hello World');",
		Status:  "active",
		UserID:  user.ID,
	}
	require.NoError(t, models.CreateJSTemplate(db, template))

	tests := []struct {
		name           string
		templateID     string
		updates        map[string]interface{}
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name:       "Valid update",
			templateID: template.ID,
			updates: map[string]interface{}{
				"name":        "Updated Template",
				"description": "Updated description",
			},
			user:           user,
			expectedStatus: 200,
		},
		{
			name:       "Update with invalid content",
			templateID: template.ID,
			updates: map[string]interface{}{
				"content": "eval('malicious');",
			},
			user:           user,
			expectedStatus: 400,
			expectedError:  "代码不符合安全规范",
		},
		{
			name:           "Invalid template ID",
			templateID:     "invalid-id",
			updates:        map[string]interface{}{"name": "Updated"},
			user:           user,
			expectedStatus: 400,
			expectedError:  "Template not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			jsonData, _ := json.Marshal(tt.updates)
			c.Request = httptest.NewRequest("PUT", fmt.Sprintf("/js-templates/%s", tt.templateID), bytes.NewBuffer(jsonData))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = []gin.Param{{Key: "id", Value: tt.templateID}}
			if tt.user != nil {
				c.Set("user", tt.user)
			}
			c.Set(constants.DbField, db)

			h.UpdateJSTemplate(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}
		})
	}
}

func TestDeleteJSTemplate(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test template
	template := &models.JSTemplate{
		Name:    "Test Template",
		Type:    "custom",
		Content: "console.log('Hello World');",
		Status:  "active",
		UserID:  user.ID,
	}
	require.NoError(t, models.CreateJSTemplate(db, template))

	tests := []struct {
		name           string
		templateID     string
		user           *models.User
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid deletion",
			templateID:     template.ID,
			user:           user,
			expectedStatus: 200,
		},
		{
			name:           "Invalid template ID",
			templateID:     "invalid-id",
			user:           user,
			expectedStatus: 400,
			expectedError:  "Template not found",
		},
		{
			name:           "No user",
			templateID:     template.ID,
			user:           nil,
			expectedStatus: 400,
			expectedError:  "User not logged in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("DELETE", fmt.Sprintf("/js-templates/%s", tt.templateID), nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.templateID}}
			if tt.user != nil {
				c.Set("user", tt.user)
			}
			c.Set(constants.DbField, db)

			h.DeleteJSTemplate(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}

			if w.Code == 200 {
				// Verify template was deleted
				var deletedTemplate models.JSTemplate
				err := db.Where("id = ?", tt.templateID).First(&deletedTemplate).Error
				assert.Error(t, err)
				assert.Equal(t, gorm.ErrRecordNotFound, err)
			}
		})
	}
}

func TestTriggerJSTemplateWebhook(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test template with webhook enabled
	template := &models.JSTemplate{
		Name:           "Webhook Template",
		Type:           "custom",
		Content:        "console.log('webhook triggered');",
		JsSourceID:     "test-source-id",
		WebhookEnabled: true,
		WebhookSecret:  "test-secret",
	}
	require.NoError(t, models.CreateJSTemplate(db, template))

	tests := []struct {
		name           string
		jsSourceID     string
		payload        map[string]interface{}
		headers        map[string]string
		expectedStatus int
		expectedError  string
	}{
		{
			name:       "Valid webhook trigger",
			jsSourceID: template.JsSourceID,
			payload: map[string]interface{}{
				"test": "data",
			},
			expectedStatus: 200,
		},
		{
			name:           "Invalid jsSourceID",
			jsSourceID:     "invalid-id",
			payload:        map[string]interface{}{},
			expectedStatus: 400,
			expectedError:  "Template not found",
		},
		{
			name:           "Webhook disabled template",
			jsSourceID:     "disabled-webhook",
			payload:        map[string]interface{}{},
			expectedStatus: 400,
			expectedError:  "Webhook is not enabled",
		},
	}

	// Create template with webhook disabled for test
	disabledTemplate := &models.JSTemplate{
		Name:           "Disabled Webhook Template",
		Type:           "custom",
		Content:        "console.log('test');",
		JsSourceID:     "disabled-webhook",
		WebhookEnabled: false,
	}
	require.NoError(t, models.CreateJSTemplate(db, disabledTemplate))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			jsonData, _ := json.Marshal(tt.payload)
			c.Request = httptest.NewRequest("POST", fmt.Sprintf("/js-templates/webhook/%s", tt.jsSourceID), bytes.NewBuffer(jsonData))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = []gin.Param{{Key: "jsSourceId", Value: tt.jsSourceID}}

			// Set custom headers
			for key, value := range tt.headers {
				c.Request.Header.Set(key, value)
			}

			c.Set(constants.DbField, db)

			h.TriggerJSTemplateWebhook(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Contains(t, response["msg"].(string), tt.expectedError)
			}
		})
	}
}

func TestListJSTemplateVersions(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test template
	template := &models.JSTemplate{
		Name:    "Test Template",
		Type:    "custom",
		Content: "console.log('Hello World');",
		Status:  "active",
		UserID:  user.ID,
		Version: 1,
	}
	require.NoError(t, models.CreateJSTemplate(db, template))

	// Create test versions
	for i := 1; i <= 3; i++ {
		version := &models.JSTemplateVersion{
			ID:         fmt.Sprintf("version-%d", i),
			TemplateID: template.ID,
			Version:    uint(i),
			Name:       fmt.Sprintf("Version %d", i),
			Content:    fmt.Sprintf("console.log('version %d');", i),
			Status:     "active",
			CreatedBy:  user.ID,
		}
		require.NoError(t, models.CreateJSTemplateVersion(db, version))
	}

	tests := []struct {
		name           string
		templateID     string
		user           *models.User
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "Valid template versions",
			templateID:     template.ID,
			user:           user,
			expectedStatus: 200,
			expectedCount:  3,
		},
		{
			name:           "Invalid template ID",
			templateID:     "invalid-id",
			user:           user,
			expectedStatus: 400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", fmt.Sprintf("/js-templates/%s/versions", tt.templateID), nil)
			c.Params = []gin.Param{{Key: "id", Value: tt.templateID}}
			if tt.user != nil {
				c.Set("user", tt.user)
			}
			c.Set(constants.DbField, db)

			h.ListJSTemplateVersions(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data := response["data"].(map[string]interface{})
				versions := data["data"].([]interface{})
				assert.Equal(t, tt.expectedCount, len(versions))
			}
		})
	}
}

func TestSearchJSTemplates(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test templates with different names
	templates := []string{"Search Template", "Another Template", "Test Script"}
	for _, name := range templates {
		template := &models.JSTemplate{
			Name:    name,
			Type:    "custom",
			Content: "console.log('test');",
			Status:  "active",
			UserID:  user.ID,
		}
		require.NoError(t, models.CreateJSTemplate(db, template))
	}

	tests := []struct {
		name           string
		keyword        string
		user           *models.User
		expectedStatus int
		minResults     int
	}{
		{
			name:           "Search with keyword 'Template'",
			keyword:        "Template",
			user:           user,
			expectedStatus: 200,
			minResults:     2, // Should find "Search Template" and "Another Template"
		},
		{
			name:           "Search with keyword 'Script'",
			keyword:        "Script",
			user:           user,
			expectedStatus: 200,
			minResults:     1, // Should find "Test Script"
		},
		{
			name:           "Search with no results",
			keyword:        "NonExistent",
			user:           user,
			expectedStatus: 200,
			minResults:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest("GET", fmt.Sprintf("/js-templates/search?keyword=%s", tt.keyword), nil)
			if tt.user != nil {
				c.Set("user", tt.user)
			}
			c.Set(constants.DbField, db)

			h.SearchJSTemplates(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == 200 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				data := response["data"].(map[string]interface{})
				templates := data["data"].([]interface{})
				assert.GreaterOrEqual(t, len(templates), tt.minResults)
			}
		})
	}
}

// Benchmark tests
func BenchmarkCreateJSTemplate(b *testing.B) {
	db := setupTestDB(&testing.T{})

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	db.Create(user)

	template := models.JSTemplate{
		Name:    "Benchmark Template",
		Type:    "custom",
		Content: "console.log('benchmark');",
		Status:  "active",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		template.Name = fmt.Sprintf("Benchmark Template %d", i)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		jsonData, _ := json.Marshal(template)
		c.Request = httptest.NewRequest("POST", "/js-templates", bytes.NewBuffer(jsonData))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)
		c.Set(constants.DbField, db)

		h.CreateJSTemplate(c)
	}
}

func BenchmarkListJSTemplates(b *testing.B) {
	db := setupTestDB(&testing.T{})

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	db.Create(user)

	// Create test templates
	for i := 0; i < 100; i++ {
		template := &models.JSTemplate{
			Name:    fmt.Sprintf("Template %d", i),
			Type:    "custom",
			Content: "console.log('test');",
			Status:  "active",
			UserID:  user.ID,
		}
		models.CreateJSTemplate(db, template)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Request = httptest.NewRequest("GET", "/js-templates", nil)
		c.Set("user", user)
		c.Set(constants.DbField, db)

		h.ListJSTemplates(c)
	}
}

// Helper function to create test template
func createTestJSTemplate(t *testing.T, db *gorm.DB, userID uint, name string) *models.JSTemplate {
	template := &models.JSTemplate{
		Name:    name,
		Type:    "custom",
		Content: "console.log('test');",
		Status:  "active",
		UserID:  userID,
	}
	require.NoError(t, models.CreateJSTemplate(db, template))
	return template
}

// Test edge cases
func TestJSTemplateEdgeCases(t *testing.T) {
	db := setupTestDB(t)

	h := &Handlers{db: db}
	gin.SetMode(gin.TestMode)

	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	t.Run("Create template with very long content", func(t *testing.T) {
		longContent := make([]byte, 10000)
		for i := range longContent {
			longContent[i] = 'a'
		}

		template := models.JSTemplate{
			Name:    "Long Content Template",
			Type:    "custom",
			Content: string(longContent),
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		jsonData, _ := json.Marshal(template)
		c.Request = httptest.NewRequest("POST", "/js-templates", bytes.NewBuffer(jsonData))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)
		c.Set(constants.DbField, db)

		h.CreateJSTemplate(c)

		// Should handle long content gracefully
		assert.True(t, w.Code == 200 || w.Code == 400)
	})

	t.Run("Update non-existent template", func(t *testing.T) {
		updates := map[string]interface{}{
			"name": "Updated Name",
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		jsonData, _ := json.Marshal(updates)
		c.Request = httptest.NewRequest("PUT", "/js-templates/non-existent-id", bytes.NewBuffer(jsonData))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = []gin.Param{{Key: "id", Value: "non-existent-id"}}
		c.Set("user", user)
		c.Set(constants.DbField, db)

		h.UpdateJSTemplate(c)

		assert.Equal(t, 400, w.Code)
	})

	t.Run("Concurrent template creation", func(t *testing.T) {
		const numGoroutines = 10
		results := make(chan int, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				template := models.JSTemplate{
					Name:    fmt.Sprintf("Concurrent Template %d", index),
					Type:    "custom",
					Content: "console.log('concurrent');",
				}

				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)

				jsonData, _ := json.Marshal(template)
				c.Request = httptest.NewRequest("POST", "/js-templates", bytes.NewBuffer(jsonData))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set("user", user)
				c.Set(constants.DbField, db)

				h.CreateJSTemplate(c)
				results <- w.Code
			}(i)
		}

		// Collect results
		successCount := 0
		for i := 0; i < numGoroutines; i++ {
			code := <-results
			if code == 200 {
				successCount++
			}
		}

		// Should handle concurrent requests
		assert.Greater(t, successCount, 0)
	})
}
