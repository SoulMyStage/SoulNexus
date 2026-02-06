package hardware

import (
	"errors"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// ErrHandler error handler
type ErrHandler struct {
	logger *zap.Logger
	mu     sync.RWMutex
}

// NewErrHandler create error handler
func NewErrHandler(logger *zap.Logger) *ErrHandler {
	return &ErrHandler{
		logger: logger,
	}
}

// IsFatal check if error is fatal
func (h *ErrHandler) IsFatal(err error) bool {
	if err == nil {
		return false
	}

	// check if it's our unified error type
	var e *Error
	if errors.As(err, &e) {
		return e.Type == ErrorTypeFatal
	}

	// check keywords in error message
	errMsg := strings.ToLower(err.Error())
	fatalKeywords := []string{
		"quota exceeded",
		"quota exhausted",
		"pkg exhausted",
		"allowance has been exhausted",
		"insufficient quota",
		"quota limit",
		"unauthorized",
		"authentication failed",
		"invalid credentials",
		"api key invalid",
		"api key expired",
		"account suspended",
		"account disabled",
		"permission denied",
		"access denied",
	}

	for _, keyword := range fatalKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// IsTransient check if error is transient
func (h *ErrHandler) IsTransient(err error) bool {
	if err == nil {
		return false
	}

	// check if it's our unified error type
	var e *Error
	if errors.As(err, &e) {
		return e.Type == ErrorTypeTransient
	}

	errMsg := strings.ToLower(err.Error())
	transientKeywords := []string{
		"timeout",
		"connection reset",
		"connection refused",
		"network",
		"temporary",
		"retry",
		"rate limit",
		"too many requests",
		"service unavailable",
		"internal server error",
	}

	for _, keyword := range transientKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// Classify error type
func (h *ErrHandler) Classify(err error, service string) *Error {
	if err == nil {
		return nil
	}

	// if already unified error type, return directly
	var e *Error
	if errors.As(err, &e) {
		return e
	}

	// classify error
	errType := ErrorTypeRecoverable
	if h.IsFatal(err) {
		errType = ErrorTypeFatal
	} else if h.IsTransient(err) {
		errType = ErrorTypeTransient
	}

	return &Error{
		Type:    errType,
		Service: service,
		Message: err.Error(),
		Err:     err,
	}
}

// HandleError handle error with logging
func (h *ErrHandler) HandleError(err error, service string) error {
	if err == nil {
		return nil
	}

	classified := h.Classify(err, service)

	h.mu.Lock()
	defer h.mu.Unlock()

	switch classified.Type {
	case ErrorTypeFatal:
		h.logger.Error("fatal error",
			zap.String("service", service),
			zap.String("type", classified.Type.String()),
			zap.Error(err),
			zap.String("message", classified.Message),
		)
	case ErrorTypeRecoverable:
		h.logger.Warn("recoverable error",
			zap.String("service", service),
			zap.String("type", classified.Type.String()),
			zap.Error(err),
			zap.String("message", classified.Message),
		)
	case ErrorTypeTransient:
		h.logger.Debug("transient error",
			zap.String("service", service),
			zap.String("type", classified.Type.String()),
			zap.Error(err),
			zap.String("message", classified.Message),
		)
	}

	return classified
}

// NewFatalError create fatal error
func NewFatalError(service, message string, err error) *Error {
	return &Error{
		Type:    ErrorTypeFatal,
		Service: service,
		Message: message,
		Err:     err,
	}
}

// NewRecoverableError create recoverable error
func NewRecoverableError(service, message string, err error) *Error {
	return &Error{
		Type:    ErrorTypeRecoverable,
		Service: service,
		Message: message,
		Err:     err,
	}
}

// NewTransientError create transient error
func NewTransientError(service, message string, err error) *Error {
	return &Error{
		Type:    ErrorTypeTransient,
		Service: service,
		Message: message,
		Err:     err,
	}
}
