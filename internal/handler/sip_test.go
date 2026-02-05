package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockSipServer implements SipServerInterface for testing
type MockSipServer struct {
	mock.Mock
}

func (m *MockSipServer) MakeOutgoingCall(targetURI string) (string, error) {
	args := m.Called(targetURI)
	return args.String(0), args.Error(1)
}

func (m *MockSipServer) GetOutgoingSession(callID string) (interface{}, bool) {
	args := m.Called(callID)
	return args.Get(0), args.Bool(1)
}

func (m *MockSipServer) CancelOutgoingCall(callID string) error {
	args := m.Called(callID)
	return args.Error(0)
}

func (m *MockSipServer) HangupOutgoingCall(callID string) error {
	args := m.Called(callID)
	return args.Error(0)
}

// MockOutgoingSession represents a mock SIP session
type MockOutgoingSession struct {
	RemoteRTPAddr string
	CallID        string
	TargetURI     string
	Status        string
	StartTime     time.Time
	AnswerTime    *time.Time
	EndTime       *time.Time
	Error         string
}

func TestSipHandler_MakeOutgoingCall(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	mockSipServer := new(MockSipServer)
	handler := NewSipHandler(db, mockSipServer)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    interface{}
		mockSetup      func()
		expectedStatus int
	}{
		{
			name: "successful outgoing call",
			requestBody: MakeOutgoingCallRequest{
				TargetURI: "sip:test@example.com",
				Notes:     "Test call",
			},
			mockSetup: func() {
				mockSipServer.On("MakeOutgoingCall", "sip:test@example.com").Return("call-123", nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing target URI",
			requestBody: MakeOutgoingCallRequest{
				Notes: "Test call",
			},
			mockSetup:      func() {},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid request body",
			requestBody:    "invalid json",
			mockSetup:      func() {},
			expectedStatus: http.StatusOK,
		},
		{
			name: "sip server error",
			requestBody: MakeOutgoingCallRequest{
				TargetURI: "sip:test@example.com",
			},
			mockSetup: func() {
				mockSipServer.On("MakeOutgoingCall", "sip:test@example.com").Return("", fmt.Errorf("server error"))
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockSipServer.ExpectedCalls = nil
			tt.mockSetup()

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/sip/calls/outgoing", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			handler.MakeOutgoingCall(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSipHandler_GetOutgoingCallStatus(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	mockSipServer := new(MockSipServer)
	handler := NewSipHandler(db, mockSipServer)
	gin.SetMode(gin.TestMode)

	// Create test SIP call record
	sipCall := &models.SipCall{
		CallID:    "call-123",
		Direction: models.SipCallDirectionOutbound,
		Status:    models.SipCallStatusCalling,
		ToURI:     "sip:test@example.com",
		StartTime: time.Now(),
	}
	require.NoError(t, db.Create(sipCall).Error)

	tests := []struct {
		name           string
		callID         string
		mockSetup      func()
		expectedStatus int
	}{
		{
			name:   "successful get call status from database",
			callID: "call-123",
			mockSetup: func() {
				mockSession := &MockOutgoingSession{
					CallID:    "call-123",
					TargetURI: "sip:test@example.com",
					Status:    "calling",
					StartTime: time.Now(),
				}
				mockSipServer.On("GetOutgoingSession", "call-123").Return(mockSession, true)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "call not found in database or server",
			callID: "call-999",
			mockSetup: func() {
				mockSipServer.On("GetOutgoingSession", "call-999").Return(nil, false)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "empty call ID",
			callID:         "",
			mockSetup:      func() {},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockSipServer.ExpectedCalls = nil
			tt.mockSetup()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/sip/calls/outgoing/"+tt.callID, nil)
			c.Params = gin.Params{{Key: "callId", Value: tt.callID}}

			handler.GetOutgoingCallStatus(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSipHandler_CancelOutgoingCall(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	mockSipServer := new(MockSipServer)
	handler := NewSipHandler(db, mockSipServer)
	gin.SetMode(gin.TestMode)

	// Create test SIP call record
	sipCall := &models.SipCall{
		CallID:    "call-123",
		Direction: models.SipCallDirectionOutbound,
		Status:    models.SipCallStatusCalling,
		ToURI:     "sip:test@example.com",
		StartTime: time.Now(),
	}
	require.NoError(t, db.Create(sipCall).Error)

	tests := []struct {
		name           string
		callID         string
		mockSetup      func()
		expectedStatus int
	}{
		{
			name:   "successful cancel call",
			callID: "call-123",
			mockSetup: func() {
				mockSipServer.On("CancelOutgoingCall", "call-123").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "cancel call error",
			callID: "call-123",
			mockSetup: func() {
				mockSipServer.On("CancelOutgoingCall", "call-123").Return(fmt.Errorf("cancel error"))
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "empty call ID",
			callID:         "",
			mockSetup:      func() {},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockSipServer.ExpectedCalls = nil
			tt.mockSetup()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/sip/calls/outgoing/"+tt.callID+"/cancel", nil)
			c.Params = gin.Params{{Key: "callId", Value: tt.callID}}

			handler.CancelOutgoingCall(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSipHandler_HangupOutgoingCall(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	mockSipServer := new(MockSipServer)
	handler := NewSipHandler(db, mockSipServer)
	gin.SetMode(gin.TestMode)

	// Create test SIP call record
	answerTime := time.Now().Add(-30 * time.Second)
	sipCall := &models.SipCall{
		CallID:     "call-123",
		Direction:  models.SipCallDirectionOutbound,
		Status:     models.SipCallStatusAnswered,
		ToURI:      "sip:test@example.com",
		StartTime:  time.Now().Add(-60 * time.Second),
		AnswerTime: &answerTime,
	}
	require.NoError(t, db.Create(sipCall).Error)

	tests := []struct {
		name           string
		callID         string
		mockSetup      func()
		expectedStatus int
	}{
		{
			name:   "successful hangup call",
			callID: "call-123",
			mockSetup: func() {
				mockSipServer.On("HangupOutgoingCall", "call-123").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "hangup call not found",
			callID: "call-999",
			mockSetup: func() {
				mockSipServer.On("HangupOutgoingCall", "call-999").Return(fmt.Errorf("call not found"))
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "empty call ID",
			callID:         "",
			mockSetup:      func() {},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockSipServer.ExpectedCalls = nil
			tt.mockSetup()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/sip/calls/outgoing/"+tt.callID+"/hangup", nil)
			c.Params = gin.Params{{Key: "callId", Value: tt.callID}}

			handler.HangupOutgoingCall(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSipHandler_GetCallHistory(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	mockSipServer := new(MockSipServer)
	handler := NewSipHandler(db, mockSipServer)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test SIP calls
	call1 := &models.SipCall{
		CallID:    "call-1",
		Direction: models.SipCallDirectionOutbound,
		Status:    models.SipCallStatusEnded,
		ToURI:     "sip:test1@example.com",
		StartTime: time.Now().Add(-2 * time.Hour),
		UserID:    &user.ID,
	}
	call2 := &models.SipCall{
		CallID:    "call-2",
		Direction: models.SipCallDirectionInbound,
		Status:    models.SipCallStatusEnded,
		FromURI:   "sip:test2@example.com",
		StartTime: time.Now().Add(-1 * time.Hour),
		UserID:    &user.ID,
	}
	require.NoError(t, db.Create(call1).Error)
	require.NoError(t, db.Create(call2).Error)

	tests := []struct {
		name           string
		queryParams    string
		setupAuth      func(*gin.Context)
		expectedStatus int
		expectedCount  int
	}{
		{
			name:        "successful get call history",
			queryParams: "",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:        "get call history with status filter",
			queryParams: "?status=completed",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:        "get call history with pagination",
			queryParams: "?limit=1&page=1",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name:           "no authentication",
			queryParams:    "",
			setupAuth:      func(c *gin.Context) {},
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/sip/calls"+tt.queryParams, nil)

			tt.setupAuth(c)
			handler.GetCallHistory(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedCount >= 0 {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)

				if data, ok := response["data"].(map[string]interface{}); ok {
					if list, ok := data["list"].([]interface{}); ok {
						assert.Equal(t, tt.expectedCount, len(list))
					}
				}
			}
		})
	}
}

func TestSipHandler_GetCallDetail(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	mockSipServer := new(MockSipServer)
	handler := NewSipHandler(db, mockSipServer)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test SIP call
	sipCall := &models.SipCall{
		CallID:    "call-123",
		Direction: models.SipCallDirectionOutbound,
		Status:    models.SipCallStatusEnded,
		ToURI:     "sip:test@example.com",
		StartTime: time.Now().Add(-1 * time.Hour),
		UserID:    &user.ID,
	}
	require.NoError(t, db.Create(sipCall).Error)

	tests := []struct {
		name           string
		callID         string
		setupAuth      func(*gin.Context)
		expectedStatus int
	}{
		{
			name:   "successful get call detail",
			callID: "call-123",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "call not found",
			callID: "call-999",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "empty call ID",
			callID:         "",
			setupAuth:      func(c *gin.Context) {},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/sip/calls/"+tt.callID+"/detail", nil)
			c.Params = gin.Params{{Key: "callId", Value: tt.callID}}

			tt.setupAuth(c)
			handler.GetCallDetail(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSipHandler_GetSipUsers(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	mockSipServer := new(MockSipServer)
	handler := NewSipHandler(db, mockSipServer)
	gin.SetMode(gin.TestMode)

	// Create test SIP users
	sipUser1 := &models.SipUser{
		DisplayName: "sip_user_1",
		Enabled:     true,
	}
	sipUser2 := &models.SipUser{
		DisplayName: "sip_user_2",
		Enabled:     false,
	}
	require.NoError(t, db.Create(sipUser1).Error)
	require.NoError(t, db.Create(sipUser2).Error)

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
	}{
		{
			name:           "successful get all sip users",
			queryParams:    "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "get enabled sip users only",
			queryParams:    "?enabled=true",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "get sip users with status filter",
			queryParams:    "?status=active",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/sip/users"+tt.queryParams, nil)

			handler.GetSipUsers(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestSipHandler_RequestTranscription(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	mockSipServer := new(MockSipServer)
	handler := NewSipHandler(db, mockSipServer)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	require.NoError(t, db.Create(user).Error)

	// Create test SIP call with recording
	sipCall := &models.SipCall{
		CallID:    "call-123",
		Direction: models.SipCallDirectionOutbound,
		Status:    models.SipCallStatusEnded,
		ToURI:     "sip:test@example.com",
		StartTime: time.Now().Add(-1 * time.Hour),
		UserID:    &user.ID,
		RecordURL: "/recordings/call-123.wav",
	}
	require.NoError(t, db.Create(sipCall).Error)

	tests := []struct {
		name           string
		callID         string
		requestBody    interface{}
		setupAuth      func(*gin.Context)
		expectedStatus int
	}{
		{
			name:   "successful request transcription",
			callID: "call-123",
			requestBody: TranscribeCallRequest{
				AudioURL: "/recordings/call-123.wav",
				Language: "zh-CN",
			},
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "call not found",
			callID: "call-999",
			requestBody: TranscribeCallRequest{
				AudioURL: "/recordings/call-999.wav",
				Language: "zh-CN",
			},
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "invalid request body",
			callID:      "call-123",
			requestBody: "invalid json",
			setupAuth: func(c *gin.Context) {
				c.Set("user", user)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/sip/calls/"+tt.callID+"/transcribe", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "callId", Value: tt.callID}}

			tt.setupAuth(c)
			handler.RequestTranscription(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

// Test helper functions
func TestSipHandler_ConvertOutgoingSession(t *testing.T) {
	// Test the convertOutgoingSession function
	mockSession := &MockOutgoingSession{
		CallID:        "call-123",
		TargetURI:     "sip:test@example.com",
		Status:        "calling",
		StartTime:     time.Now(),
		RemoteRTPAddr: "192.168.1.100:5004",
		Error:         "",
	}

	result := convertOutgoingSession(mockSession)

	assert.NotNil(t, result)
	assert.Equal(t, "call-123", result.CallID)
	assert.Equal(t, "sip:test@example.com", result.TargetURI)
	assert.Equal(t, "calling", result.Status)
	assert.Equal(t, "192.168.1.100:5004", result.RemoteRTPAddr)
}

func TestSipHandler_ParseWAVFile(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(db)

	mockSipServer := new(MockSipServer)
	handler := NewSipHandler(db, mockSipServer)

	// Create a minimal WAV header for testing
	wavHeader := []byte{
		// RIFF header
		'R', 'I', 'F', 'F',
		0x24, 0x08, 0x00, 0x00, // File size - 8
		'W', 'A', 'V', 'E',
		// fmt chunk
		'f', 'm', 't', ' ',
		0x10, 0x00, 0x00, 0x00, // Chunk size
		0x01, 0x00, // Audio format (PCM)
		0x01, 0x00, // Number of channels
		0x44, 0xAC, 0x00, 0x00, // Sample rate (44100)
		0x88, 0x58, 0x01, 0x00, // Byte rate
		0x02, 0x00, // Block align
		0x10, 0x00, // Bits per sample
		// data chunk
		'd', 'a', 't', 'a',
		0x00, 0x08, 0x00, 0x00, // Data size
	}

	// Add some dummy PCM data
	pcmData := make([]byte, 2048)
	for i := range pcmData {
		pcmData[i] = byte(i % 256)
	}

	wavData := append(wavHeader, pcmData...)

	pcm, sampleRate, err := handler.parseWAVFile(wavData)

	assert.NoError(t, err)
	assert.Equal(t, 44100, sampleRate)
	assert.Equal(t, len(pcmData), len(pcm))

	// Test with invalid WAV data
	invalidData := []byte("not a wav file")
	_, _, err = handler.parseWAVFile(invalidData)
	assert.Error(t, err)
}

// Benchmark tests
func BenchmarkSipHandler_MakeOutgoingCall(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	mockSipServer := new(MockSipServer)
	mockSipServer.On("MakeOutgoingCall", mock.AnythingOfType("string")).Return("call-123", nil)

	handler := NewSipHandler(db, mockSipServer)
	gin.SetMode(gin.TestMode)

	requestBody := MakeOutgoingCallRequest{
		TargetURI: "sip:benchmark@example.com",
		Notes:     "Benchmark call",
	}
	body, _ := json.Marshal(requestBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/sip/calls/outgoing", bytes.NewBuffer(body))
		c.Request.Header.Set("Content-Type", "application/json")

		handler.MakeOutgoingCall(c)
	}
}

func BenchmarkSipHandler_GetCallHistory(b *testing.B) {
	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(db)

	mockSipServer := new(MockSipServer)
	handler := NewSipHandler(db, mockSipServer)
	gin.SetMode(gin.TestMode)

	// Create test user
	user := &models.User{
		Email:       "test@example.com",
		DisplayName: "testuser",
		Password:    "hashedpassword",
	}
	db.Create(user)

	// Create multiple call records for benchmarking
	for i := 0; i < 100; i++ {
		call := &models.SipCall{
			CallID:    fmt.Sprintf("call-%d", i),
			Direction: models.SipCallDirectionOutbound,
			Status:    models.SipCallStatusEnded,
			ToURI:     fmt.Sprintf("sip:test%d@example.com", i),
			StartTime: time.Now().Add(-time.Duration(i) * time.Minute),
			UserID:    &user.ID,
		}
		db.Create(call)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/sip/calls", nil)
		c.Set("user", user)

		handler.GetCallHistory(c)
	}
}
