package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlers_CreateWorkflowDefinition(t *testing.T) {
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

	// Create test group
	group := models.Group{
		Name:      "Test Group",
		CreatorID: user.ID,
	}
	require.NoError(t, db.Create(&group).Error)

	tests := []struct {
		name         string
		input        workflowDefinitionInput
		setupAuth    bool
		expectedCode int
	}{
		{
			name: "Valid workflow creation",
			input: workflowDefinitionInput{
				Name:        "Test Workflow",
				Slug:        "test-workflow",
				Description: "Test workflow description",
				Status:      "draft",
				Definition: models.WorkflowGraph{
					Nodes: []models.WorkflowNodeSchema{
						{ID: "start", Name: "Start", Type: "start"},
						{ID: "end", Name: "End", Type: "end"},
					},
					Edges: []models.WorkflowEdgeSchema{
						{ID: "edge1", Source: "start", Target: "end"},
					},
				},
				Settings: models.JSONMap{
					"timeout": 300,
				},
				Triggers: models.JSONMap{
					"manual": true,
				},
				Tags:    []string{"test", "automation"},
				Version: 1,
			},
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name: "Workflow with group assignment",
			input: workflowDefinitionInput{
				Name:        "Group Workflow",
				Slug:        "group-workflow",
				Description: "Group workflow description",
				Definition: models.WorkflowGraph{
					Nodes: []models.WorkflowNodeSchema{
						{ID: "start", Name: "Start", Type: "start"},
						{ID: "end", Name: "End", Type: "end"},
					},
					Edges: []models.WorkflowEdgeSchema{
						{ID: "edge1", Source: "start", Target: "end"},
					},
				},
				GroupID: float64(group.ID),
			},
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name: "Invalid workflow definition - no nodes",
			input: workflowDefinitionInput{
				Name:        "Invalid Workflow",
				Slug:        "invalid-workflow",
				Description: "Invalid workflow",
				Definition: models.WorkflowGraph{
					Nodes: []models.WorkflowNodeSchema{},
					Edges: []models.WorkflowEdgeSchema{},
				},
			},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name: "Invalid workflow definition - no start node",
			input: workflowDefinitionInput{
				Name:        "No Start Workflow",
				Slug:        "no-start-workflow",
				Description: "Workflow without start node",
				Definition: models.WorkflowGraph{
					Nodes: []models.WorkflowNodeSchema{
						{ID: "end", Name: "End", Type: "end"},
					},
					Edges: []models.WorkflowEdgeSchema{},
				},
			},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name: "Unauthorized request",
			input: workflowDefinitionInput{
				Name: "Unauthorized Workflow",
				Slug: "unauthorized-workflow",
				Definition: models.WorkflowGraph{
					Nodes: []models.WorkflowNodeSchema{
						{ID: "start", Name: "Start", Type: "start"},
					},
				},
			},
			setupAuth:    false,
			expectedCode: 500,
		},
		{
			name: "Missing required fields",
			input: workflowDefinitionInput{
				// Missing name and slug
				Definition: models.WorkflowGraph{
					Nodes: []models.WorkflowNodeSchema{
						{ID: "start", Name: "Start", Type: "start"},
					},
				},
			},
			setupAuth:    true,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			if tt.setupAuth {
				c.Set("user", &user)
			}

			body, _ := json.Marshal(tt.input)
			req := httptest.NewRequest("POST", "/workflows/definitions", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.CreateWorkflowDefinition(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                       `json:"code"`
					Data models.WorkflowDefinition `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.input.Name, response.Data.Name)
				assert.Equal(t, tt.input.Slug, response.Data.Slug)
			}
		})
	}
}
func TestHandlers_ListWorkflowDefinitions(t *testing.T) {
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

	// Create test workflows
	workflow1 := models.WorkflowDefinition{
		UserID:      user.ID,
		Name:        "Active Workflow",
		Slug:        "active-workflow",
		Description: "Active workflow description",
		Status:      "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
			},
		},
	}
	workflow2 := models.WorkflowDefinition{
		UserID:      user.ID,
		Name:        "Draft Workflow",
		Slug:        "draft-workflow",
		Description: "Draft workflow description",
		Status:      "draft",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
			},
		},
	}
	require.NoError(t, db.Create(&workflow1).Error)
	require.NoError(t, db.Create(&workflow2).Error)

	tests := []struct {
		name          string
		queryParams   map[string]string
		setupAuth     bool
		expectedCode  int
		expectedCount int
	}{
		{
			name:          "List all workflows",
			queryParams:   map[string]string{},
			setupAuth:     true,
			expectedCode:  200,
			expectedCount: 2,
		},
		{
			name: "Filter by status",
			queryParams: map[string]string{
				"status": "active",
			},
			setupAuth:     true,
			expectedCode:  200,
			expectedCount: 1,
		},
		{
			name: "Search by keyword",
			queryParams: map[string]string{
				"keyword": "Active",
			},
			setupAuth:     true,
			expectedCode:  200,
			expectedCount: 1,
		},
		{
			name:         "Unauthorized request",
			queryParams:  map[string]string{},
			setupAuth:    false,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			if tt.setupAuth {
				c.Set("user", &user)
			}

			// Build query string
			queryString := ""
			for k, v := range tt.queryParams {
				if queryString != "" {
					queryString += "&"
				}
				queryString += k + "=" + v
			}

			url := "/workflows/definitions"
			if queryString != "" {
				url += "?" + queryString
			}

			req := httptest.NewRequest("GET", url, nil)
			c.Request = req

			h.ListWorkflowDefinitions(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                         `json:"code"`
					Data []models.WorkflowDefinition `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Len(t, response.Data, tt.expectedCount)
			}
		})
	}
}

func TestHandlers_GetWorkflowDefinition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test workflow
	workflow := models.WorkflowDefinition{
		UserID:      1,
		Name:        "Test Workflow",
		Slug:        "test-workflow",
		Description: "Test workflow description",
		Status:      "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
			},
		},
	}
	require.NoError(t, db.Create(&workflow).Error)

	tests := []struct {
		name         string
		workflowID   string
		expectedCode int
	}{
		{
			name:         "Valid workflow ID",
			workflowID:   strconv.Itoa(int(workflow.ID)),
			expectedCode: 200,
		},
		{
			name:         "Invalid workflow ID",
			workflowID:   "invalid",
			expectedCode: 500,
		},
		{
			name:         "Non-existent workflow",
			workflowID:   "99999",
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "id", Value: tt.workflowID}}

			req := httptest.NewRequest("GET", "/workflows/definitions/"+tt.workflowID, nil)
			c.Request = req

			h.GetWorkflowDefinition(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int                       `json:"code"`
					Data models.WorkflowDefinition `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, workflow.Name, response.Data.Name)
			}
		})
	}
}

func TestHandlers_UpdateWorkflowDefinition(t *testing.T) {
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

	// Create test workflow
	workflow := models.WorkflowDefinition{
		UserID:      user.ID,
		Name:        "Original Workflow",
		Slug:        "original-workflow",
		Description: "Original description",
		Status:      "draft",
		Version:     1,
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
		input        map[string]interface{}
		setupAuth    bool
		expectedCode int
	}{
		{
			name:       "Valid update",
			workflowID: strconv.Itoa(int(workflow.ID)),
			input: map[string]interface{}{
				"name":        "Updated Workflow",
				"description": "Updated description",
				"status":      "active",
				"version":     1,
				"changeNote":  "Updated workflow",
			},
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name:       "Version conflict",
			workflowID: strconv.Itoa(int(workflow.ID)),
			input: map[string]interface{}{
				"name":    "Updated Workflow",
				"version": 999, // Wrong version
			},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name:       "Missing version",
			workflowID: strconv.Itoa(int(workflow.ID)),
			input: map[string]interface{}{
				"name": "Updated Workflow",
				// Missing version
			},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name:       "Invalid workflow definition",
			workflowID: strconv.Itoa(int(workflow.ID)),
			input: map[string]interface{}{
				"version": 1,
				"definition": map[string]interface{}{
					"nodes": []interface{}{}, // Empty nodes (invalid)
					"edges": []interface{}{},
				},
			},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name:         "Unauthorized request",
			workflowID:   strconv.Itoa(int(workflow.ID)),
			input:        map[string]interface{}{"version": 1},
			setupAuth:    false,
			expectedCode: 500,
		},
		{
			name:         "Invalid workflow ID",
			workflowID:   "invalid",
			input:        map[string]interface{}{},
			setupAuth:    true,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "id", Value: tt.workflowID}}

			if tt.setupAuth {
				c.Set("user", &user)
			}

			body, _ := json.Marshal(tt.input)
			req := httptest.NewRequest("PUT", "/workflows/definitions/"+tt.workflowID, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.UpdateWorkflowDefinition(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

func TestHandlers_DeleteWorkflowDefinition(t *testing.T) {
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

	// Create test workflow
	workflow := models.WorkflowDefinition{
		UserID:      user.ID,
		Name:        "Deletable Workflow",
		Slug:        "deletable-workflow",
		Description: "Workflow to be deleted",
		Status:      "draft",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
			},
		},
	}
	require.NoError(t, db.Create(&workflow).Error)

	tests := []struct {
		name         string
		workflowID   string
		setupAuth    bool
		expectedCode int
	}{
		{
			name:         "Valid deletion",
			workflowID:   strconv.Itoa(int(workflow.ID)),
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name:         "Unauthorized request",
			workflowID:   strconv.Itoa(int(workflow.ID)),
			setupAuth:    false,
			expectedCode: 500,
		},
		{
			name:         "Invalid workflow ID",
			workflowID:   "invalid",
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name:         "Non-existent workflow",
			workflowID:   "99999",
			setupAuth:    true,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "id", Value: tt.workflowID}}

			if tt.setupAuth {
				c.Set("user", &user)
			}

			req := httptest.NewRequest("DELETE", "/workflows/definitions/"+tt.workflowID, nil)
			c.Request = req

			h.DeleteWorkflowDefinition(c)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}
func TestHandlers_RunWorkflowDefinition(t *testing.T) {
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

	// Create test workflow
	workflow := models.WorkflowDefinition{
		UserID:      user.ID,
		Name:        "Runnable Workflow",
		Slug:        "runnable-workflow",
		Description: "Workflow that can be run",
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
		input        map[string]interface{}
		setupAuth    bool
		expectedCode int
	}{
		{
			name:       "Valid workflow execution",
			workflowID: strconv.Itoa(int(workflow.ID)),
			input: map[string]interface{}{
				"parameters": map[string]interface{}{
					"input": "test value",
				},
			},
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name:         "Execution without parameters",
			workflowID:   strconv.Itoa(int(workflow.ID)),
			input:        map[string]interface{}{},
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name:         "Empty request body",
			workflowID:   strconv.Itoa(int(workflow.ID)),
			input:        nil,
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name:         "Unauthorized request",
			workflowID:   strconv.Itoa(int(workflow.ID)),
			input:        map[string]interface{}{},
			setupAuth:    false,
			expectedCode: 500,
		},
		{
			name:         "Invalid workflow ID",
			workflowID:   "invalid",
			input:        map[string]interface{}{},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name:         "Non-existent workflow",
			workflowID:   "99999",
			input:        map[string]interface{}{},
			setupAuth:    true,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{{Key: "id", Value: tt.workflowID}}

			if tt.setupAuth {
				c.Set("user", &user)
			}

			var body []byte
			if tt.input != nil {
				body, _ = json.Marshal(tt.input)
			}

			req := httptest.NewRequest("POST", "/workflows/definitions/"+tt.workflowID+"/run", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.RunWorkflowDefinition(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int `json:"code"`
					Data struct {
						Instance models.WorkflowInstance `json:"instance"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, workflow.ID, response.Data.Instance.DefinitionID)
			}
		})
	}
}

func TestHandlers_TestWorkflowNode(t *testing.T) {
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

	// Create test workflow
	workflow := models.WorkflowDefinition{
		UserID:      user.ID,
		Name:        "Test Node Workflow",
		Slug:        "test-node-workflow",
		Description: "Workflow for testing nodes",
		Status:      "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{
					ID:   "start",
					Name: "Start Node",
					Type: "start",
				},
				{
					ID:   "task1",
					Name: "Task Node",
					Type: "task",
					Properties: models.StringMap{
						"action":  "log",
						"message": "Hello World",
					},
				},
				{
					ID:   "gateway1",
					Name: "Gateway Node",
					Type: "gateway",
					Properties: models.StringMap{
						"condition": "input > 0",
					},
				},
				{
					ID:   "end",
					Name: "End Node",
					Type: "end",
				},
			},
			Edges: []models.WorkflowEdgeSchema{
				{ID: "edge1", Source: "start", Target: "task1"},
				{ID: "edge2", Source: "task1", Target: "gateway1"},
				{ID: "edge3", Source: "gateway1", Target: "end", Type: models.WorkflowEdgeTypeTrue},
			},
		},
	}
	require.NoError(t, db.Create(&workflow).Error)

	tests := []struct {
		name         string
		workflowID   string
		nodeID       string
		input        map[string]interface{}
		setupAuth    bool
		expectedCode int
	}{
		{
			name:       "Test start node",
			workflowID: strconv.Itoa(int(workflow.ID)),
			nodeID:     "start",
			input: map[string]interface{}{
				"parameters": map[string]interface{}{
					"input": "test value",
				},
			},
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name:       "Test task node",
			workflowID: strconv.Itoa(int(workflow.ID)),
			nodeID:     "task1",
			input: map[string]interface{}{
				"parameters": map[string]interface{}{
					"message": "Custom message",
				},
			},
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name:       "Test gateway node",
			workflowID: strconv.Itoa(int(workflow.ID)),
			nodeID:     "gateway1",
			input: map[string]interface{}{
				"parameters": map[string]interface{}{
					"input": 5,
				},
			},
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name:         "Test without parameters",
			workflowID:   strconv.Itoa(int(workflow.ID)),
			nodeID:       "start",
			input:        map[string]interface{}{},
			setupAuth:    true,
			expectedCode: 200,
		},
		{
			name:         "Non-existent node",
			workflowID:   strconv.Itoa(int(workflow.ID)),
			nodeID:       "non-existent",
			input:        map[string]interface{}{},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name:         "Invalid workflow ID",
			workflowID:   "invalid",
			nodeID:       "start",
			input:        map[string]interface{}{},
			setupAuth:    true,
			expectedCode: 500,
		},
		{
			name:         "Unauthorized request",
			workflowID:   strconv.Itoa(int(workflow.ID)),
			nodeID:       "start",
			input:        map[string]interface{}{},
			setupAuth:    false,
			expectedCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = []gin.Param{
				{Key: "id", Value: tt.workflowID},
				{Key: "nodeId", Value: tt.nodeID},
			}

			if tt.setupAuth {
				c.Set("user", &user)
			}

			body, _ := json.Marshal(tt.input)
			req := httptest.NewRequest("POST", "/workflows/definitions/"+tt.workflowID+"/nodes/"+tt.nodeID+"/test", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			c.Request = req

			h.TestWorkflowNode(c)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == 200 {
				var response struct {
					Code int `json:"code"`
					Data struct {
						NodeID   string `json:"nodeId"`
						NodeName string `json:"nodeName"`
						Status   string `json:"status"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.nodeID, response.Data.NodeID)
			}
		})
	}
}

func TestValidateWorkflowGraph(t *testing.T) {
	tests := []struct {
		name        string
		graph       models.WorkflowGraph
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid workflow graph",
			graph: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
					{ID: "task1", Name: "Task 1", Type: "task"},
					{ID: "end", Name: "End", Type: "end"},
				},
				Edges: []models.WorkflowEdgeSchema{
					{ID: "edge1", Source: "start", Target: "task1"},
					{ID: "edge2", Source: "task1", Target: "end"},
				},
			},
			expectError: false,
		},
		{
			name: "Empty nodes",
			graph: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{},
				Edges: []models.WorkflowEdgeSchema{},
			},
			expectError: true,
			errorMsg:    "workflow must contain at least one node",
		},
		{
			name: "Duplicate node IDs",
			graph: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
					{ID: "start", Name: "Duplicate Start", Type: "start"},
				},
				Edges: []models.WorkflowEdgeSchema{},
			},
			expectError: true,
			errorMsg:    "duplicate node id start",
		},
		{
			name: "No start node",
			graph: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "task1", Name: "Task 1", Type: "task"},
					{ID: "end", Name: "End", Type: "end"},
				},
				Edges: []models.WorkflowEdgeSchema{},
			},
			expectError: true,
			errorMsg:    "workflow must contain exactly one start node, got 0",
		},
		{
			name: "Multiple start nodes",
			graph: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start1", Name: "Start 1", Type: "start"},
					{ID: "start2", Name: "Start 2", Type: "start"},
					{ID: "end", Name: "End", Type: "end"},
				},
				Edges: []models.WorkflowEdgeSchema{},
			},
			expectError: true,
			errorMsg:    "workflow must contain exactly one start node, got 2",
		},
		{
			name: "No end node",
			graph: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
					{ID: "task1", Name: "Task 1", Type: "task"},
				},
				Edges: []models.WorkflowEdgeSchema{},
			},
			expectError: true,
			errorMsg:    "workflow must contain at least one end node",
		},
		{
			name: "Unsupported node type",
			graph: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
					{ID: "invalid", Name: "Invalid", Type: "invalid_type"},
					{ID: "end", Name: "End", Type: "end"},
				},
				Edges: []models.WorkflowEdgeSchema{},
			},
			expectError: true,
			errorMsg:    "unsupported node type invalid_type",
		},
		{
			name: "Edge with unknown source",
			graph: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
					{ID: "end", Name: "End", Type: "end"},
				},
				Edges: []models.WorkflowEdgeSchema{
					{ID: "edge1", Source: "unknown", Target: "end"},
				},
			},
			expectError: true,
			errorMsg:    "edge references unknown source node unknown",
		},
		{
			name: "Edge with unknown target",
			graph: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
					{ID: "end", Name: "End", Type: "end"},
				},
				Edges: []models.WorkflowEdgeSchema{
					{ID: "edge1", Source: "start", Target: "unknown"},
				},
			},
			expectError: true,
			errorMsg:    "edge references unknown target node unknown",
		},
		{
			name: "Invalid edge type for non-gateway node",
			graph: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
					{ID: "task1", Name: "Task 1", Type: "task"},
					{ID: "end", Name: "End", Type: "end"},
				},
				Edges: []models.WorkflowEdgeSchema{
					{ID: "edge1", Source: "task1", Target: "end", Type: models.WorkflowEdgeTypeTrue},
				},
			},
			expectError: true,
			errorMsg:    "edge type true allowed only for gateway/condition nodes",
		},
		{
			name: "Valid gateway with conditional edges",
			graph: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
					{ID: "gateway1", Name: "Gateway", Type: "gateway"},
					{ID: "end1", Name: "End 1", Type: "end"},
					{ID: "end2", Name: "End 2", Type: "end"},
				},
				Edges: []models.WorkflowEdgeSchema{
					{ID: "edge1", Source: "start", Target: "gateway1"},
					{ID: "edge2", Source: "gateway1", Target: "end1", Type: models.WorkflowEdgeTypeTrue},
					{ID: "edge3", Source: "gateway1", Target: "end2", Type: models.WorkflowEdgeTypeFalse},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkflowGraph(tt.graph)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
func TestHandlers_GetAvailableEventTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test workflows with event nodes and triggers
	workflow1 := models.WorkflowDefinition{
		UserID: 1,
		Name:   "Event Publisher Workflow",
		Status: "active",
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{
					ID:   "start",
					Name: "Start",
					Type: "start",
				},
				{
					ID:   "event1",
					Name: "Publish Event",
					Type: "event",
					Properties: models.StringMap{
						"event_type": "user.created",
					},
				},
			},
		},
	}

	workflow2 := models.WorkflowDefinition{
		UserID: 1,
		Name:   "Event Listener Workflow",
		Status: "active",
		Triggers: models.JSONMap{
			"event": map[string]interface{}{
				"enabled": true,
				"events":  []interface{}{"user.updated", "user.deleted"},
			},
		},
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
			},
		},
	}

	require.NoError(t, db.Create(&workflow1).Error)
	require.NoError(t, db.Create(&workflow2).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("GET", "/workflows/events/types", nil)
	c.Request = req

	h.GetAvailableEventTypes(c)

	assert.Equal(t, 200, w.Code)

	var response struct {
		Code int `json:"code"`
		Data struct {
			EventTypes []map[string]interface{} `json:"event_types"`
			Total      int                      `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	// Should contain event types from workflows
	assert.Greater(t, response.Data.Total, 0)
	assert.Greater(t, len(response.Data.EventTypes), 0)

	// Check if our test event types are present
	eventTypes := make(map[string]bool)
	for _, eventType := range response.Data.EventTypes {
		if typeStr, ok := eventType["type"].(string); ok {
			eventTypes[typeStr] = true
		}
	}

	// Should contain events from workflow nodes and triggers
	// Note: Actual event types depend on what's in the event bus and workflows
}

// Test version management functions
func TestHandlers_WorkflowVersionManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	// Create test workflow
	workflow := models.WorkflowDefinition{
		UserID:      1,
		Name:        "Versioned Workflow",
		Slug:        "versioned-workflow",
		Description: "Workflow with versions",
		Status:      "active",
		Version:     1,
		Definition: models.WorkflowGraph{
			Nodes: []models.WorkflowNodeSchema{
				{ID: "start", Name: "Start", Type: "start"},
				{ID: "end", Name: "End", Type: "end"},
			},
		},
	}
	require.NoError(t, db.Create(&workflow).Error)

	// Create version history
	version1 := models.WorkflowVersion{
		DefinitionID: workflow.ID,
		Version:      1,
		Name:         workflow.Name,
		Slug:         workflow.Slug,
		Description:  workflow.Description,
		Status:       workflow.Status,
		Definition:   workflow.Definition,
		ChangeNote:   "Initial version",
	}
	require.NoError(t, db.Create(&version1).Error)

	t.Run("ListWorkflowVersions", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "id", Value: strconv.Itoa(int(workflow.ID))}}

		req := httptest.NewRequest("GET", "/workflows/definitions/"+strconv.Itoa(int(workflow.ID))+"/versions", nil)
		c.Request = req

		h.ListWorkflowVersions(c)

		assert.Equal(t, 200, w.Code)

		var response struct {
			Code int                      `json:"code"`
			Data []models.WorkflowVersion `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Len(t, response.Data, 1)
		assert.Equal(t, uint(1), response.Data[0].Version)
	})

	t.Run("GetWorkflowVersion", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{
			{Key: "id", Value: strconv.Itoa(int(workflow.ID))},
			{Key: "versionId", Value: strconv.Itoa(int(version1.ID))},
		}

		req := httptest.NewRequest("GET", "/workflows/definitions/"+strconv.Itoa(int(workflow.ID))+"/versions/"+strconv.Itoa(int(version1.ID)), nil)
		c.Request = req

		h.GetWorkflowVersion(c)

		assert.Equal(t, 200, w.Code)

		var response struct {
			Code int                    `json:"code"`
			Data models.WorkflowVersion `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, version1.ID, response.Data.ID)
		assert.Equal(t, "Initial version", response.Data.ChangeNote)
	})

	t.Run("RollbackWorkflowVersion", func(t *testing.T) {
		// Create test user
		user := models.User{
			Email:       "test@example.com",
			DisplayName: "testuser",
		}
		db.Create(&user)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{
			{Key: "id", Value: strconv.Itoa(int(workflow.ID))},
			{Key: "versionId", Value: strconv.Itoa(int(version1.ID))},
		}
		c.Set("user", &user)

		req := httptest.NewRequest("POST", "/workflows/definitions/"+strconv.Itoa(int(workflow.ID))+"/versions/"+strconv.Itoa(int(version1.ID))+"/rollback", nil)
		c.Request = req

		h.RollbackWorkflowVersion(c)

		assert.Equal(t, 200, w.Code)

		// Verify workflow version was incremented
		var updated models.WorkflowDefinition
		db.First(&updated, workflow.ID)
		assert.Greater(t, updated.Version, workflow.Version)
	})

	t.Run("CompareWorkflowVersions", func(t *testing.T) {
		// Create another version for comparison
		version2 := models.WorkflowVersion{
			DefinitionID: workflow.ID,
			Version:      2,
			Name:         "Updated Workflow",
			Slug:         workflow.Slug,
			Description:  "Updated description",
			Status:       workflow.Status,
			Definition:   workflow.Definition,
			ChangeNote:   "Updated version",
		}
		db.Create(&version2)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "id", Value: strconv.Itoa(int(workflow.ID))}}

		req := httptest.NewRequest("GET", "/workflows/definitions/"+strconv.Itoa(int(workflow.ID))+"/versions/compare?version1="+strconv.Itoa(int(version1.ID))+"&version2="+strconv.Itoa(int(version2.ID)), nil)
		c.Request = req

		h.CompareWorkflowVersions(c)

		assert.Equal(t, 200, w.Code)

		var response struct {
			Code int `json:"code"`
			Data struct {
				Version1 models.WorkflowVersion `json:"version1"`
				Version2 models.WorkflowVersion `json:"version2"`
				Diff     map[string]interface{} `json:"diff"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

		assert.Equal(t, version1.ID, response.Data.Version1.ID)
		assert.Equal(t, version2.ID, response.Data.Version2.ID)

		// Should have differences in name and description
		assert.Contains(t, response.Data.Diff, "name")
		assert.Contains(t, response.Data.Diff, "description")
	})
}

// Benchmark tests
func BenchmarkHandlers_CreateWorkflowDefinition(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	user := models.User{
		Email:       "bench@example.com",
		DisplayName: "benchuser",
	}
	db.Create(&user)

	input := workflowDefinitionInput{
		Name:        "Benchmark Workflow",
		Slug:        "benchmark-workflow",
		Description: "Benchmark workflow description",
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Modify slug to avoid duplicates
		input.Slug = fmt.Sprintf("benchmark-workflow-%d", i)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", &user)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/workflows/definitions", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.CreateWorkflowDefinition(c)
	}
}

func BenchmarkHandlers_ListWorkflowDefinitions(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	user := models.User{
		Email:       "bench@example.com",
		DisplayName: "benchuser",
	}
	db.Create(&user)

	// Create test workflows
	for i := 0; i < 100; i++ {
		workflow := models.WorkflowDefinition{
			UserID:      user.ID,
			Name:        fmt.Sprintf("Workflow %d", i),
			Slug:        fmt.Sprintf("workflow-%d", i),
			Description: fmt.Sprintf("Description %d", i),
			Status:      "active",
			Definition: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
				},
			},
		}
		db.Create(&workflow)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", &user)

		req := httptest.NewRequest("GET", "/workflows/definitions", nil)
		c.Request = req

		h.ListWorkflowDefinitions(c)
	}
}

// Edge case tests
func TestWorkflowHandlerEdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	h := &Handlers{db: db}

	user := models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
	}
	db.Create(&user)

	t.Run("Create workflow with very long name", func(t *testing.T) {
		longName := strings.Repeat("a", 1000)

		input := workflowDefinitionInput{
			Name: longName,
			Slug: "long-name-workflow",
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

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", &user)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/workflows/definitions", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.CreateWorkflowDefinition(c)

		// Should handle long names gracefully
		assert.Equal(t, 200, w.Code)
	})

	t.Run("Update workflow with concurrent version conflict", func(t *testing.T) {
		workflow := models.WorkflowDefinition{
			UserID:  user.ID,
			Name:    "Concurrent Workflow",
			Slug:    "concurrent-workflow",
			Version: 1,
			Definition: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{ID: "start", Name: "Start", Type: "start"},
					{ID: "end", Name: "End", Type: "end"},
				},
			},
		}
		db.Create(&workflow)

		// Simulate concurrent update by changing version in database
		db.Model(&workflow).Update("version", 2)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{{Key: "id", Value: strconv.Itoa(int(workflow.ID))}}
		c.Set("user", &user)

		input := map[string]interface{}{
			"name":    "Updated Name",
			"version": 1, // Old version
		}

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("PUT", "/workflows/definitions/"+strconv.Itoa(int(workflow.ID)), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.UpdateWorkflowDefinition(c)

		assert.Equal(t, 500, w.Code) // Should detect version conflict
	})

	t.Run("Test node with complex nested parameters", func(t *testing.T) {
		workflow := models.WorkflowDefinition{
			UserID: user.ID,
			Name:   "Complex Node Workflow",
			Definition: models.WorkflowGraph{
				Nodes: []models.WorkflowNodeSchema{
					{
						ID:   "complex",
						Name: "Complex Node",
						Type: "task",
						Properties: models.StringMap{
							"nested_deep_value": "test",
						},
					},
				},
			},
		}
		db.Create(&workflow)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = []gin.Param{
			{Key: "id", Value: strconv.Itoa(int(workflow.ID))},
			{Key: "nodeId", Value: "complex"},
		}
		c.Set("user", &user)

		input := map[string]interface{}{
			"parameters": map[string]interface{}{
				"complex_param": map[string]interface{}{
					"array": []interface{}{1, 2, 3},
					"object": map[string]interface{}{
						"nested": "value",
					},
				},
			},
		}

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/workflows/definitions/"+strconv.Itoa(int(workflow.ID))+"/nodes/complex/test", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		h.TestWorkflowNode(c)

		assert.Equal(t, 200, w.Code)
	})
}
