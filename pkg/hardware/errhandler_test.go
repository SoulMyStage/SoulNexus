package hardware

import (
	"errors"
	"testing"

	"go.uber.org/zap"
)

func TestErrorType_String(t *testing.T) {
	testCases := []struct {
		errorType ErrorType
		expected  string
	}{
		{ErrorTypeFatal, "fatal"},
		{ErrorTypeRecoverable, "recoverable"},
		{ErrorTypeTransient, "transient"},
		{ErrorType(999), "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.errorType.String()
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	testCases := []struct {
		name     string
		error    *Error
		expected string
	}{
		{
			name: "error with underlying error",
			error: &Error{
				Type:    ErrorTypeFatal,
				Service: "test-service",
				Message: "test message",
				Err:     errors.New("underlying error"),
			},
			expected: "[test-service] test message: underlying error",
		},
		{
			name: "error without underlying error",
			error: &Error{
				Type:    ErrorTypeRecoverable,
				Service: "test-service",
				Message: "test message",
				Err:     nil,
			},
			expected: "[test-service] test message",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.error.Error()
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	underlyingErr := errors.New("underlying error")
	err := &Error{
		Type:    ErrorTypeFatal,
		Service: "test-service",
		Message: "test message",
		Err:     underlyingErr,
	}

	unwrapped := err.Unwrap()
	if unwrapped != underlyingErr {
		t.Errorf("Expected %v, got %v", underlyingErr, unwrapped)
	}
}

func TestHandler_IsFatal(t *testing.T) {
	logger := zap.NewNop()
	handler := NewErrHandler(logger)

	testCases := []struct {
		name     string
		error    error
		expected bool
	}{
		{"nil error", nil, false},
		{"quota exceeded", errors.New("quota exceeded"), true},
		{"unauthorized", errors.New("Unauthorized access"), true},
		{"api key invalid", errors.New("API key invalid"), true},
		{"network timeout", errors.New("network timeout"), false},
		{"regular error", errors.New("some regular error"), false},
		{
			"unified fatal error",
			&Error{Type: ErrorTypeFatal, Service: "test", Message: "fatal"},
			true,
		},
		{
			"unified recoverable error",
			&Error{Type: ErrorTypeRecoverable, Service: "test", Message: "recoverable"},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := handler.IsFatal(tc.error)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v for error: %v", tc.expected, result, tc.error)
			}
		})
	}
}

func TestHandler_IsTransient(t *testing.T) {
	logger := zap.NewNop()
	handler := NewErrHandler(logger)

	testCases := []struct {
		name     string
		error    error
		expected bool
	}{
		{"nil error", nil, false},
		{"timeout", errors.New("connection timeout"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"rate limit", errors.New("rate limit exceeded"), true},
		{"service unavailable", errors.New("service unavailable"), true},
		{"quota exceeded", errors.New("quota exceeded"), false},
		{"regular error", errors.New("some regular error"), false},
		{
			"unified transient error",
			&Error{Type: ErrorTypeTransient, Service: "test", Message: "transient"},
			true,
		},
		{
			"unified fatal error",
			&Error{Type: ErrorTypeFatal, Service: "test", Message: "fatal"},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := handler.IsTransient(tc.error)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v for error: %v", tc.expected, result, tc.error)
			}
		})
	}
}

func TestHandler_Classify(t *testing.T) {
	logger := zap.NewNop()
	handler := NewErrHandler(logger)

	testCases := []struct {
		name         string
		error        error
		service      string
		expectedType ErrorType
	}{
		{"nil error", nil, "test", ErrorType(0)}, // will return nil
		{"quota exceeded", errors.New("quota exceeded"), "test", ErrorTypeFatal},
		{"timeout", errors.New("connection timeout"), "test", ErrorTypeTransient},
		{"regular error", errors.New("some error"), "test", ErrorTypeRecoverable},
		{
			"already classified",
			&Error{Type: ErrorTypeFatal, Service: "original", Message: "fatal"},
			"test",
			ErrorTypeFatal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := handler.Classify(tc.error, tc.service)

			if tc.error == nil {
				if result != nil {
					t.Errorf("Expected nil for nil error, got %v", result)
				}
				return
			}

			if result == nil {
				t.Fatalf("Expected non-nil result for error: %v", tc.error)
			}

			if result.Type != tc.expectedType {
				t.Errorf("Expected type %v, got %v", tc.expectedType, result.Type)
			}

			// For already classified errors, service should remain original
			if _, ok := tc.error.(*Error); ok {
				if result.Service != "original" {
					t.Errorf("Expected service 'original', got %s", result.Service)
				}
			} else {
				if result.Service != tc.service {
					t.Errorf("Expected service %s, got %s", tc.service, result.Service)
				}
			}
		})
	}
}

func TestHandler_HandleError(t *testing.T) {
	logger := zap.NewNop()
	handler := NewErrHandler(logger)

	testCases := []struct {
		name    string
		error   error
		service string
	}{
		{"nil error", nil, "test"},
		{"fatal error", errors.New("quota exceeded"), "test"},
		{"recoverable error", errors.New("some error"), "test"},
		{"transient error", errors.New("timeout"), "test"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := handler.HandleError(tc.error, tc.service)

			if tc.error == nil {
				if result != nil {
					t.Errorf("Expected nil for nil error, got %v", result)
				}
				return
			}

			if result == nil {
				t.Fatalf("Expected non-nil result for error: %v", tc.error)
			}

			// Should return classified error
			if classifiedErr, ok := result.(*Error); ok {
				if classifiedErr.Service != tc.service {
					t.Errorf("Expected service %s, got %s", tc.service, classifiedErr.Service)
				}
			} else {
				t.Errorf("Expected *Error type, got %T", result)
			}
		})
	}
}

func TestNewErrorFunctions(t *testing.T) {
	underlyingErr := errors.New("underlying")
	service := "test-service"
	message := "test message"

	testCases := []struct {
		name         string
		createFunc   func() *Error
		expectedType ErrorType
	}{
		{
			"NewFatalError",
			func() *Error { return NewFatalError(service, message, underlyingErr) },
			ErrorTypeFatal,
		},
		{
			"NewRecoverableError",
			func() *Error { return NewRecoverableError(service, message, underlyingErr) },
			ErrorTypeRecoverable,
		},
		{
			"NewTransientError",
			func() *Error { return NewTransientError(service, message, underlyingErr) },
			ErrorTypeTransient,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.createFunc()

			if err.Type != tc.expectedType {
				t.Errorf("Expected type %v, got %v", tc.expectedType, err.Type)
			}
			if err.Service != service {
				t.Errorf("Expected service %s, got %s", service, err.Service)
			}
			if err.Message != message {
				t.Errorf("Expected message %s, got %s", message, err.Message)
			}
			if err.Err != underlyingErr {
				t.Errorf("Expected underlying error %v, got %v", underlyingErr, err.Err)
			}
		})
	}
}

func BenchmarkHandler_IsFatal(b *testing.B) {
	logger := zap.NewNop()
	handler := NewErrHandler(logger)
	err := errors.New("quota exceeded")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.IsFatal(err)
	}
}

func BenchmarkHandler_Classify(b *testing.B) {
	logger := zap.NewNop()
	handler := NewErrHandler(logger)
	err := errors.New("some error message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.Classify(err, "test-service")
	}
}
