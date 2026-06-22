package database

import (
	"final-review-platform/services/api/internal/platform/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Connect(databaseURL string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
}

func EnsureExtensions(db *gorm.DB) error {
	return db.Exec(`CREATE EXTENSION IF NOT EXISTS "pgcrypto"`).Error
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(model.AllModels()...)
}
