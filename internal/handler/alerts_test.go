package handlers

import (
	"testing"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto migrate tables
	err = db.AutoMigrate(
		&models.User{},
		&models.AlertRule{},
		&models.Alert{},
		&models.AlertNotification{},
	)
	require.NoError(t, err)

	return db
}

func cleanupTestDB(db *gorm.DB) {
	sqlDB, _ := db.DB()
	sqlDB.Close()
}
