package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/constants"
)

func setupBillingTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate models
	err = db.AutoMigrate(
		&models.User{},
		&models.UsageRecord{},
		&models.Bill{},
	)
	require.NoError(t, err)

	return db
}

func setupBillingTestRouter(handlers *Handlers) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add middleware to set database
	router.Use(func(c *gin.Context) {
		c.Set(constants.DbField, handlers.db)
		c.Next()
	})

	// Billing routes
	router.GET("/billing/records", handlers.GetUsageRecords)
	router.GET("/billing/usage", handlers.GetUsageStatistics)
	router.GET("/billing/daily", handlers.GetDailyUsageData)
	router.POST("/billing/generate", handlers.GenerateBill)
	router.GET("/billing/bills", handlers.GetBills)
	router.GET("/billing/bills/:id", handlers.GetBill)
	router.PUT("/billing/bills/:id", handlers.UpdateBill)
	router.DELETE("/billing/bills/:id", handlers.DeleteBill)
	router.POST("/billing/export", handlers.ExportUsageRecords)

	return router
}

func createTestBillingRecord(t *testing.T, db *gorm.DB, userID uint) *models.Bill {
	record := &models.Bill{
		UserID:    userID,
		BillNo:    "TEST-001",
		Title:     "Test billing record",
		Status:    models.BillStatusGenerated,
		StartTime: time.Now().AddDate(0, 0, -30),
		EndTime:   time.Now(),
	}

	err := db.Create(record).Error
	require.NoError(t, err)

	return record
}

func createTestUsageStatistics(t *testing.T, db *gorm.DB, userID uint) *models.UsageStatistics {
	stats := &models.UsageStatistics{
		StartTime:        time.Now().AddDate(0, 0, -30),
		EndTime:          time.Now(),
		LLMCalls:         100,
		LLMTokens:        5000,
		PromptTokens:     3000,
		CompletionTokens: 2000,
		CallDuration:     3600,
		CallCount:        10,
		AvgCallDuration:  360.0,
		ASRDuration:      1800,
		ASRCount:         20,
		TTSDuration:      900,
		TTSCount:         15,
		APICalls:         50,
	}

	return stats
}

func TestHandlers_handleGetBillingRecords(t *testing.T) {
	db := setupBillingTestDB(t)
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test billing records
	record1 := createTestBillingRecord(t, db, user.ID)
	record2 := createTestBillingRecord(t, db, user.ID)
	record2.BillNo = "TEST-002"
	record2.Title = "Second test bill"
	record2.Status = models.BillStatusDraft
	db.Save(record2)

	// Verify records were created
	assert.NotNil(t, record1)
	assert.NotNil(t, record2)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Get all billing records",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Get billing records with pagination",
			queryParams:    "?page=1&size=10",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Filter by type",
			queryParams:    "?type=charge",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Filter by status",
			queryParams:    "?status=completed",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Filter by date range",
			queryParams:    "?start_date=2024-01-01&end_date=2024-12-31",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/billing/records"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "records")
				assert.Contains(t, data, "total")
				assert.Contains(t, data, "page")
				assert.Contains(t, data, "size")

				records := data["records"].([]interface{})
				if len(records) > 0 {
					firstRecord := records[0].(map[string]interface{})
					assert.Contains(t, firstRecord, "id")
					assert.Contains(t, firstRecord, "amount")
					assert.Contains(t, firstRecord, "currency")
					assert.Contains(t, firstRecord, "type")
					assert.Contains(t, firstRecord, "status")
				}
			}
		})
	}
}

func TestHandlers_handleGetUsageStatistics(t *testing.T) {
	db := setupBillingTestDB(t)
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create test usage statistics
	stats1 := createTestUsageStatistics(t, db, user.ID)
	stats2 := createTestUsageStatistics(t, db, user.ID)
	stats2.EndTime = time.Now().AddDate(0, -1, 0)
	stats2.LLMCalls = 150
	stats2.LLMTokens = 7500

	// Verify stats were created
	assert.NotNil(t, stats1)
	assert.NotNil(t, stats2)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Get current usage statistics",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Get usage for specific period",
			queryParams:    "?period=2024-01",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Get usage with date range",
			queryParams:    "?start_date=2024-01-01&end_date=2024-02-28",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/billing/usage"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "apiCallCount")
				assert.Contains(t, data, "tokensUsed")
				assert.Contains(t, data, "storageUsed")
				assert.Contains(t, data, "bandwidthUsed")
			}
		})
	}
}

func TestHandlers_handleCreateCharge(t *testing.T) {
	db := setupBillingTestDB(t)
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Valid charge creation",
			requestBody: map[string]interface{}{
				"amount":      50.00,
				"currency":    "USD",
				"description": "API usage charge",
				"metadata": map[string]interface{}{
					"service": "api_calls",
					"count":   "1000",
				},
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Charge with different currency",
			requestBody: map[string]interface{}{
				"amount":      100.00,
				"currency":    "EUR",
				"description": "Premium service charge",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Missing amount",
			requestBody: map[string]interface{}{
				"currency":    "USD",
				"description": "Invalid charge",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Invalid amount (negative)",
			requestBody: map[string]interface{}{
				"amount":      -10.00,
				"currency":    "USD",
				"description": "Negative charge",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Missing currency",
			requestBody: map[string]interface{}{
				"amount":      25.00,
				"description": "No currency charge",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Invalid currency",
			requestBody: map[string]interface{}{
				"amount":      25.00,
				"currency":    "INVALID",
				"description": "Invalid currency charge",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/billing/charge", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "id")
				assert.Contains(t, data, "amount")
				assert.Contains(t, data, "currency")
				assert.Contains(t, data, "status")
			}
		})
	}
}

func TestHandlers_handleGetBalance(t *testing.T) {
	db := setupBillingTestDB(t)
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create some billing records to affect balance
	createTestBillingRecord(t, db, user.ID) // First bill
	record2 := createTestBillingRecord(t, db, user.ID)
	record2.BillNo = "TEST-002"
	record2.Title = "Refund bill"
	record2.Status = models.BillStatusArchived
	db.Save(record2)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	req := httptest.NewRequest(http.MethodGet, "/billing/balance", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "data")

	data := response["data"].(map[string]interface{})
	assert.Contains(t, data, "balance")
	assert.Contains(t, data, "currency")
	assert.Contains(t, data, "lastUpdated")

	// Balance should be calculated from billing records
	balance := data["balance"].(float64)
	assert.True(t, balance >= 0) // Should be positive or zero
}

func TestHandlers_handleTopUp(t *testing.T) {
	db := setupBillingTestDB(t)
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Valid top-up",
			requestBody: map[string]interface{}{
				"amount":        100.00,
				"currency":      "USD",
				"paymentMethod": "credit_card",
				"paymentToken":  "tok_test_123456",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Top-up with different payment method",
			requestBody: map[string]interface{}{
				"amount":        50.00,
				"currency":      "USD",
				"paymentMethod": "paypal",
				"paymentToken":  "pp_test_789012",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Missing amount",
			requestBody: map[string]interface{}{
				"currency":      "USD",
				"paymentMethod": "credit_card",
				"paymentToken":  "tok_test_123456",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Invalid amount (too small)",
			requestBody: map[string]interface{}{
				"amount":        0.50,
				"currency":      "USD",
				"paymentMethod": "credit_card",
				"paymentToken":  "tok_test_123456",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Missing payment token",
			requestBody: map[string]interface{}{
				"amount":        25.00,
				"currency":      "USD",
				"paymentMethod": "credit_card",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/billing/topup", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "transactionId")
				assert.Contains(t, data, "status")
				assert.Contains(t, data, "amount")
			}
		})
	}
}

func TestHandlers_handleGetBillingHistory(t *testing.T) {
	db := setupBillingTestDB(t)
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create billing history
	for i := 0; i < 5; i++ {
		record := createTestBillingRecord(t, db, user.ID)
		record.BillNo = fmt.Sprintf("TEST-%03d", i+1)
		record.Title = fmt.Sprintf("Transaction %d", i+1)
		record.TotalLLMCalls = int64(10 + i*5)
		db.Save(record)
	}

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Get billing history",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Get billing history with pagination",
			queryParams:    "?page=1&size=3",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Get billing history with date filter",
			queryParams:    "?start_date=2024-01-01",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/billing/history"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "history")
				assert.Contains(t, data, "total")
				assert.Contains(t, data, "summary")

				history := data["history"].([]interface{})
				if len(history) > 0 {
					firstItem := history[0].(map[string]interface{})
					assert.Contains(t, firstItem, "id")
					assert.Contains(t, firstItem, "amount")
					assert.Contains(t, firstItem, "description")
					assert.Contains(t, firstItem, "createdAt")
				}
			}
		})
	}
}

func TestHandlers_handleRefund(t *testing.T) {
	db := setupBillingTestDB(t)
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create a charge to refund
	charge := createTestBillingRecord(t, db, user.ID)
	charge.Title = "Charge bill"
	charge.Status = models.BillStatusGenerated
	db.Save(charge)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Valid full refund",
			requestBody: map[string]interface{}{
				"chargeId": 1,
				"reason":   "Customer request",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Valid partial refund",
			requestBody: map[string]interface{}{
				"chargeId": 1,
				"amount":   5.25,
				"reason":   "Partial service issue",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Missing charge ID",
			requestBody: map[string]interface{}{
				"reason": "Missing charge ID",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "Invalid charge ID",
			requestBody: map[string]interface{}{
				"chargeId": 999,
				"reason":   "Non-existent charge",
			},
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name: "Refund amount exceeds charge",
			requestBody: map[string]interface{}{
				"chargeId": 1,
				"amount":   100.00,
				"reason":   "Amount too large",
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/billing/refund", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "refundId")
				assert.Contains(t, data, "status")
				assert.Contains(t, data, "amount")
			}
		})
	}
}

func TestHandlers_handleGetInvoice(t *testing.T) {
	db := setupBillingTestDB(t)
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	// Create test user
	user := createTestUser(t, db)

	// Create billing record that can serve as invoice
	invoice := createTestBillingRecord(t, db, user.ID)
	invoice.Title = "Invoice bill"
	invoice.Status = models.BillStatusExported
	db.Save(invoice)

	// Mock current user middleware
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	tests := []struct {
		name           string
		invoiceID      string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Valid invoice ID",
			invoiceID:      "1",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Invalid invoice ID",
			invoiceID:      "999",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:           "Non-numeric invoice ID",
			invoiceID:      "abc",
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/billing/invoice/"+tt.invoiceID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "data")

				data := response["data"].(map[string]interface{})
				assert.Contains(t, data, "id")
				assert.Contains(t, data, "amount")
				assert.Contains(t, data, "status")
				assert.Contains(t, data, "createdAt")
			}
		})
	}
}

func TestHandlers_handleBillingWebhook(t *testing.T) {
	db := setupBillingTestDB(t)
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	tests := []struct {
		name           string
		requestBody    interface{}
		headers        map[string]string
		expectedStatus int
		expectError    bool
	}{
		{
			name: "Valid payment webhook",
			requestBody: map[string]interface{}{
				"type": "payment.succeeded",
				"data": map[string]interface{}{
					"id":       "pay_123456",
					"amount":   5000, // $50.00 in cents
					"currency": "usd",
					"customer": "cus_test123",
				},
			},
			headers: map[string]string{
				"Stripe-Signature": "test_signature",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Payment failed webhook",
			requestBody: map[string]interface{}{
				"type": "payment.failed",
				"data": map[string]interface{}{
					"id":       "pay_789012",
					"amount":   2500,
					"currency": "usd",
					"customer": "cus_test456",
					"error": map[string]interface{}{
						"code":    "card_declined",
						"message": "Your card was declined.",
					},
				},
			},
			headers: map[string]string{
				"Stripe-Signature": "test_signature",
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "Invalid webhook signature",
			requestBody: map[string]interface{}{
				"type": "payment.succeeded",
				"data": map[string]interface{}{
					"id": "pay_invalid",
				},
			},
			headers: map[string]string{
				"Stripe-Signature": "invalid_signature",
			},
			expectedStatus: http.StatusUnauthorized,
			expectError:    true,
		},
		{
			name: "Missing webhook signature",
			requestBody: map[string]interface{}{
				"type": "payment.succeeded",
				"data": map[string]interface{}{
					"id": "pay_no_sig",
				},
			},
			headers:        map[string]string{},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/billing/webhook", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectError {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
			}
		})
	}
}

func BenchmarkHandlers_handleGetBillingRecords(b *testing.B) {
	db := setupBillingTestDB(&testing.T{})
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	user := createTestUser(&testing.T{}, db)
	for i := 0; i < 10; i++ {
		createTestBillingRecord(&testing.T{}, db, user.ID)
	}

	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/billing/records", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkHandlers_handleGetBalance(b *testing.B) {
	db := setupBillingTestDB(&testing.T{})
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	user := createTestUser(&testing.T{}, db)
	createTestBillingRecord(&testing.T{}, db, user.ID)

	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/billing/balance", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func TestHandlers_billingCompleteFlow(t *testing.T) {
	db := setupBillingTestDB(t)
	handlers := &Handlers{db: db}
	router := setupBillingTestRouter(handlers)

	user := createTestUser(t, db)
	router.Use(func(c *gin.Context) {
		c.Set(constants.UserField, user)
		c.Next()
	})

	// Test complete billing lifecycle
	t.Run("Complete billing lifecycle", func(t *testing.T) {
		// 1. Check initial balance
		req := httptest.NewRequest(http.MethodGet, "/billing/balance", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 2. Top up account
		topUpBody := map[string]interface{}{
			"amount":        100.00,
			"currency":      "USD",
			"paymentMethod": "credit_card",
			"paymentToken":  "tok_test_123456",
		}
		body, _ := json.Marshal(topUpBody)
		req = httptest.NewRequest(http.MethodPost, "/billing/topup", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 3. Create a charge
		chargeBody := map[string]interface{}{
			"amount":      25.00,
			"currency":    "USD",
			"description": "API usage charge",
		}
		body, _ = json.Marshal(chargeBody)
		req = httptest.NewRequest(http.MethodPost, "/billing/charge", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 4. Get billing records
		req = httptest.NewRequest(http.MethodGet, "/billing/records", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 5. Get usage statistics
		req = httptest.NewRequest(http.MethodGet, "/billing/usage", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 6. Get billing history
		req = httptest.NewRequest(http.MethodGet, "/billing/history", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 7. Check final balance
		req = httptest.NewRequest(http.MethodGet, "/billing/balance", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
