package store

import (
	"embed"
	"fmt"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"strings"
)

//go:embed *.sql
var schemas embed.FS

const RevisionHead = "8942cd035480"

func Open(url string) (*gorm.DB, error) {
	var d gorm.Dialector
	if strings.HasPrefix(url, "sqlite:///") {
		path := strings.TrimPrefix(url, "sqlite:///")
		if path != ":memory:" {
			if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
				return nil, e
			}
		}
		d = sqlite.Open(path)
	} else if strings.HasPrefix(url, "postgresql://") || strings.HasPrefix(url, "postgres://") {
		d = postgres.Open(url)
	} else {
		return nil, fmt.Errorf("unsupported DATABASE_URL")
	}
	db, e := gorm.Open(d, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		return nil, fmt.Errorf("database connection failed")
	}
	sqlDB, e := db.DB()
	if e != nil {
		return nil, e
	}
	sqlDB.SetMaxOpenConns(16)
	sqlDB.SetMaxIdleConns(4)
	if db.Dialector.Name() == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		for _, sql := range []string{"PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=10000"} {
			if e = db.Exec(sql).Error; e != nil {
				return nil, e
			}
		}
	}
	return db, nil
}
func Close(db *gorm.DB) {
	s, e := db.DB()
	if e == nil {
		s.Close()
	}
}
func Check(db *gorm.DB) error {
	var head string
	if e := db.Raw("SELECT version_num FROM alembic_version").Scan(&head).Error; e != nil || head != RevisionHead {
		return fmt.Errorf("schema is not %s; upgrade legacy database first or run migrate on a new database", RevisionHead)
	}
	return nil
}
func Migrate(db *gorm.DB) error {
	if db.Migrator().HasTable("alembic_version") {
		if e := Check(db); e != nil {
			return e
		}
		return migrateAgentGrants(db)
	}
	if db.Migrator().HasTable("users") {
		return fmt.Errorf("refusing unknown existing schema")
	}
	kind := db.Dialector.Name()
	if kind == "postgres" {
		kind = "postgres"
	}
	b, e := schemas.ReadFile(kind + ".sql")
	if e != nil {
		return e
	}
	e = db.Transaction(func(tx *gorm.DB) error {
		for _, s := range strings.Split(string(b), ";") {
			if strings.TrimSpace(s) != "" {
				if e := tx.Exec(s).Error; e != nil {
					return e
				}
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	return migrateAgentGrants(db)
}

// This additive extension deliberately leaves the legacy revision and all existing data intact.
func migrateAgentGrants(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if db.Dialector.Name() == "postgres" {
			if e := tx.Exec("SELECT pg_advisory_xact_lock(84721094)").Error; e != nil {
				return e
			}
		}
		return tx.Exec(`CREATE TABLE IF NOT EXISTS agent_grants (
		 id VARCHAR(36) PRIMARY KEY, device_hash VARCHAR(64) NOT NULL UNIQUE,
		 user_code VARCHAR(9) NOT NULL UNIQUE, name VARCHAR(60) NOT NULL,
		 token_hash VARCHAR(64) NOT NULL, token_prefix VARCHAR(10) NOT NULL,
		 status VARCHAR(12) NOT NULL, token_id VARCHAR(36) REFERENCES api_tokens(id),
		 created_at TIMESTAMP NOT NULL, expires_at TIMESTAMP NOT NULL
		)`).Error
	})
}

// Write locks the owner row in PostgreSQL; SQLite's single connection serializes writes.
func Write(db *gorm.DB, userID string, fn func(*gorm.DB) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if userID != "" && db.Dialector.Name() == "postgres" {
			if e := tx.Exec("SELECT id FROM users WHERE id = ? FOR UPDATE", userID).Error; e != nil {
				return e
			}
		}
		return fn(tx)
	})
}
