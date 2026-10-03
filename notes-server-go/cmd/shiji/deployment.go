package main

import (
	"fmt"
	"gorm.io/gorm"
	"shiji/internal/store"
)

// deploymentCheck reads metadata only, before a startup is allowed to migrate.
func deploymentCheck(db *gorm.DB) error {
	if store.Check(db) == nil {
		return nil
	}
	var count int64
	var err error
	if db.Dialector.Name() == "postgres" {
		err = db.Raw("SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f', 'S') AND n.nspname !~ '^pg_' AND n.nspname <> 'information_schema'").Scan(&count).Error
	} else {
		err = db.Raw("SELECT count(*) FROM sqlite_master WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'").Scan(&count).Error
	}
	if err != nil {
		return fmt.Errorf("database metadata check failed")
	}
	if count != 0 {
		return fmt.Errorf("refusing deployment into a database with unrelated or incompatible tables; use a dedicated empty notes database")
	}
	return nil
}

func bootstrapState(db *gorm.DB) (string, error) {
	var count int64
	if err := db.Model(&store.User{}).Count(&count).Error; err != nil {
		return "", fmt.Errorf("account initialization check failed")
	}
	if count > 0 {
		return "registered", nil
	}
	if err := db.Model(&store.Invitation{}).Where("role = ? AND used_at IS NULL AND expires_at > ?", "admin", store.Now()).Count(&count).Error; err != nil {
		return "", fmt.Errorf("invitation initialization check failed")
	}
	if count > 0 {
		return "pending", nil
	}
	return "empty", nil
}
