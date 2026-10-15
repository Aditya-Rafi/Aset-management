// Package database opens the connection and migrates the schema.
package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/config"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/idgen"
)

func Open(c *config.Config) (*gorm.DB, error) {
	gcfg := &gorm.Config{
		Logger: logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
			SlowThreshold:             500 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
		}),
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	}

	var db *gorm.DB
	var err error
	switch c.DBDriver {
	case "mysql":
		db, err = gorm.Open(mysql.Open(c.MySQLDSN()), gcfg)
	case "sqlite":
		if c.DBSQLitePath != ":memory:" {
			if err = os.MkdirAll(filepath.Dir(c.DBSQLitePath), 0o755); err != nil {
				return nil, err
			}
		}
		db, err = gorm.Open(sqlite.Open(c.DBSQLitePath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"), gcfg)
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER %q (use mysql or sqlite)", c.DBDriver)
	}
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	if c.DBDriver == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
	}
	return db, nil
}

func Migrate(db *gorm.DB) error {
	// Databases created before the inspection feature get sensible defaults once,
	// without overriding choices made later in the Kategori screen.
	hadInspection := db.Migrator().HasTable(&models.Category{}) && db.Migrator().HasColumn(&models.Category{}, "inspection")
	hadCategories := db.Migrator().HasTable(&models.Category{})
	legacyUsers := db.Migrator().HasTable("users") && !db.Migrator().HasColumn("users", "username")
	if legacyUsers {
		// The first version had only name/email/password. Its accounts were
		// all admins, so they stay active; the column needs a default for
		// SQLite to accept it on a populated table.
		if err := db.Exec("ALTER TABLE users ADD COLUMN active BOOLEAN NOT NULL DEFAULT 1").Error; err != nil {
			return err
		}
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		return err
	}
	if legacyUsers {
		if err := upgradeLegacyUsers(db); err != nil {
			return err
		}
	}
	if hadCategories && !hadInspection {
		for profile, codes := range map[string][]string{
			models.ProfileLaptop:  {"LPT"},
			models.ProfileGeneral: {"PC", "MON", "HP", "PRT", "CAM"},
		} {
			if err := db.Model(&models.Category{}).Where("code IN ?", codes).Update("inspection", profile).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

var nonUsername = regexp.MustCompile(`[^a-z0-9._-]+`)

// upgradeLegacyUsers gives accounts from the first version a username, a public
// ref, the admin role and an approval stamp. Their bcrypt hashes keep working
// (auth rehashes them on the first sign-in).
func upgradeLegacyUsers(db *gorm.DB) error {
	var rows []models.User
	if err := db.Where("username IS NULL OR username = ''").Order("id").Find(&rows).Error; err != nil {
		return err
	}
	taken := map[string]bool{}
	var existing []string
	if err := db.Model(&models.User{}).Where("username IS NOT NULL AND username <> ''").Pluck("username", &existing).Error; err != nil {
		return err
	}
	for _, e := range existing {
		taken[e] = true
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, u := range rows {
		base := ""
		if u.Email != nil {
			base = strings.SplitN(strings.ToLower(*u.Email), "@", 2)[0]
		}
		base = nonUsername.ReplaceAllString(base, "")
		if len(base) < 3 {
			base = "admin"
		}
		if len(base) > 60 {
			base = base[:60]
		}
		name := base
		for i := 2; taken[name]; i++ {
			name = fmt.Sprintf("%s%d", base, i)
		}
		taken[name] = true
		if err := db.Model(&models.User{}).Where("id = ?", u.ID).Updates(map[string]any{
			"username": name, "ref": idgen.New("usr"), "role": "admin", "active": true, "approved_at": now,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
