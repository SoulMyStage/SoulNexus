package handlers

import (
	"testing"

	"github.com/code-100-precent/LingEcho/internal/apidocs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHandlers_GetDocs(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Create handlers instance
	handlers := &Handlers{db: db}

	// Test GetDocs method directly
	docs := handlers.GetDocs()

	// Verify that docs are returned
	assert.NotNil(t, docs)
	assert.IsType(t, []apidocs.UriDoc{}, docs)
	assert.Greater(t, len(docs), 0, "Should return at least one doc")
}

func TestHandlers_GetObjs(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Create handlers instance
	handlers := &Handlers{db: db}

	// Test GetObjs method
	objs := handlers.GetObjs()

	// Verify that objects are returned
	assert.NotNil(t, objs)
	assert.Greater(t, len(objs), 0, "Should return at least one object")
}
