// Package config loads runtime settings from the environment (.env supported).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/surel"
)

type Config struct {
	Env            string
	Port           string
	DBDriver       string // mysql | sqlite
	DBHost         string
	DBPort         string
	DBName         string
	DBUser         string
	DBPassword     string
	DBSQLitePath   string
	JWTSecret      string
	SessionTTL     time.Duration
	CookieSecure   bool
	UploadDir      string
	FrontendDir    string
	AllowedOrigins []string
	AutoSeed       bool
	CompanyName    string
	// ApproverName is printed under "Diketahui Oleh" on handover forms.
	ApproverName string
	// Letterhead and place printed on the Berita Acara (BAST).
	CompanyAddress string
	CompanyPlace   string
	// PublicBaseURL is the address encoded into QR stickers (e.g. http://192.168.1.10:8090).
	// When empty the address of the incoming request is used.
	PublicBaseURL string
	AdminEmail    string
	AdminUser     string
	AdminPassword string

	AccessTokenTTL time.Duration
	GuestEnabled   bool
	GuestTTL       time.Duration
	TrustedProxies string
	SMTP           surel.Konfigurasi
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	c := &Config{
		Env:           get("APP_ENV", "local"),
		Port:          get("PORT", "8080"),
		DBDriver:      get("DB_DRIVER", "mysql"),
		DBHost:        get("DB_HOST", "127.0.0.1"),
		DBPort:        get("DB_PORT", "3306"),
		DBName:        get("DB_DATABASE", "asset_management_system"),
		DBUser:        get("DB_USERNAME", "root"),
		DBPassword:    get("DB_PASSWORD", ""),
		DBSQLitePath:  get("DB_SQLITE_PATH", "data/app.db"),
		JWTSecret:     get("JWT_SECRET", ""),
		UploadDir:     get("UPLOAD_DIR", "uploads"),
		FrontendDir:   get("FRONTEND_DIR", "../frontend/dist"),
		CompanyName:   get("COMPANY_NAME", "Departemen IT"),
		ApproverName:  get("APPROVER_NAME", "Siti Nurchayati"),
		PublicBaseURL: strings.TrimRight(get("PUBLIC_BASE_URL", ""), "/"),
		AdminEmail:    strings.ToLower(get("ADMIN_EMAIL", "")),
		AdminUser:     get("ADMIN_USER", "admin"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),

		TrustedProxies: get("TRUSTED_PROXIES", ""),
		SMTP: surel.Konfigurasi{
			Host: get("SMTP_HOST", ""), Port: get("SMTP_PORT", "587"), User: get("SMTP_USER", ""),
			Sandi: get("SMTP_PASSWORD", ""), Dari: get("SMTP_FROM", ""), Mode: get("SMTP_TLS", "starttls"),
		},
	}

	ttl, err := strconv.Atoi(get("ACCESS_TOKEN_MINUTES", "15"))
	if err != nil || ttl <= 0 {
		return nil, fmt.Errorf("ACCESS_TOKEN_MINUTES must be a positive integer")
	}
	c.AccessTokenTTL = time.Duration(ttl) * time.Minute
	c.GuestEnabled = get("GUEST_ENABLED", "false") == "true"
	gm, err := strconv.Atoi(get("GUEST_TTL_MINUTES", "30"))
	if err != nil || gm <= 0 {
		return nil, fmt.Errorf("GUEST_TTL_MINUTES must be a positive integer")
	}
	c.GuestTTL = time.Duration(gm) * time.Minute
	if err := c.SMTP.Periksa(); err != nil {
		return nil, err
	}
	c.AutoSeed = get("AUTO_SEED", "false") == "true"
	for _, o := range strings.Split(get("ALLOWED_ORIGINS", "http://localhost:5173"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}

	if c.JWTSecret == "" {
		if c.Env == "production" {
			return nil, fmt.Errorf("JWT_SECRET is required in production")
		}
		c.JWTSecret = "dev-only-insecure-secret-change-me-0123456789"
	}
	if len(c.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	return c, nil
}

func (c *Config) MySQLDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

func get(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}
