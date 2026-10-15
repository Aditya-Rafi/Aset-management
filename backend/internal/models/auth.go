package models

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// Account status, derived from Active + ApprovedAt (the account has no status column).
const (
	StatusMenunggu = "menunggu" // registered, waiting for an admin
	StatusAktif    = "aktif"
	StatusNonaktif = "nonaktif"
)

// User is a login account (not an employee: employees only hold assets).
//
// The table reuses the columns of the first version of the app (name, password,
// email) so existing installations keep their admin; see database.Migrate.
type User struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Ref           string    `gorm:"size:40;uniqueIndex" json:"ref"`      // usr_<ULID>, the public identity
	Username      string    `gorm:"size:64;uniqueIndex" json:"username"` // normalised: lower case, a-z0-9._-
	PasswordHash  string    `gorm:"column:password;size:255" json:"-"`
	FullName      string    `gorm:"column:name;size:255" json:"full_name"`
	Name          string    `gorm:"-" json:"name"` // same as FullName, for display code
	Role          string    `gorm:"size:12;index" json:"role"`
	Active        bool      `gorm:"not null" json:"-"`
	LastLoginAt   *string   `gorm:"size:40" json:"-"`
	Email         *string   `gorm:"size:255;uniqueIndex" json:"-"`
	ApprovedAt    *string   `gorm:"size:40" json:"-"` // nil: still waiting for approval
	ApprovedByRef *string   `gorm:"size:40" json:"-"`
	CreatedAt     time.Time `json:"-"`
	UpdatedAt     time.Time `json:"-"`
}

func (u *User) AfterFind(*gorm.DB) error {
	u.Name = u.FullName
	return nil
}

// Status derives the account status: active, waiting for approval, or disabled.
func (u User) Status() string {
	switch {
	case u.Active:
		return StatusAktif
	case u.ApprovedAt == nil:
		return StatusMenunggu
	default:
		return StatusNonaktif
	}
}

// PasswordReset is one forgot-password request. The code itself is never
// stored, only SHA-256(ref ":" code).
type PasswordReset struct {
	ID        uint    `gorm:"primaryKey"`
	Ref       string  `gorm:"size:40;uniqueIndex"`
	UserID    uint    `gorm:"index:idx_resets_user,priority:1;constraint:OnDelete:CASCADE"`
	CodeHash  string  `gorm:"size:64"`
	Attempts  int     `gorm:"not null;default:0"`
	ExpiresAt string  `gorm:"size:40"`
	UsedAt    *string `gorm:"size:40"`
	CreatedAt string  `gorm:"size:40;index:idx_resets_user,priority:2"`
}

func (PasswordReset) TableName() string { return "password_resets" }

// RefreshToken stores only the SHA-256 of the opaque refresh token.
type RefreshToken struct {
	ID        uint    `gorm:"primaryKey"`
	UserID    uint    `gorm:"index;constraint:OnDelete:CASCADE"`
	TokenHash string  `gorm:"size:64;uniqueIndex"`
	ExpiresAt string  `gorm:"size:40;index"`
	RevokedAt *string `gorm:"size:40"`
	CreatedAt string  `gorm:"size:40"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }

// AuditLog is the authentication audit trail. The username is copied on purpose:
// the trail must stay readable after the user is deleted.
type AuditLog struct {
	ID         uint    `gorm:"primaryKey"`
	UserRef    *string `gorm:"size:40;index:idx_audit_user_action,priority:1"`
	Username   *string `gorm:"size:255"`
	Action     string  `gorm:"size:64;index:idx_audit_user_action,priority:2"`
	Entity     *string `gorm:"size:64"`
	EntityRef  *string `gorm:"size:40"`
	DetailJSON *string `gorm:"column:detail_json;type:text"`
	IP         *string `gorm:"column:ip;size:64"`
	CreatedAt  string  `gorm:"size:40;index"`
}

func (AuditLog) TableName() string { return "audit_log" }

// ApprovalLink is the Accept/Reject link mailed to an admin for a registration.
type ApprovalLink struct {
	ID        uint    `gorm:"primaryKey"`
	Ref       string  `gorm:"size:40;uniqueIndex"`
	UserID    uint    `gorm:"index;constraint:OnDelete:CASCADE"`
	AdminRef  string  `gorm:"size:40"`
	TokenHash string  `gorm:"size:64;uniqueIndex"`
	ExpiresAt string  `gorm:"size:40"`
	UsedAt    *string `gorm:"size:40"`
	CreatedAt string  `gorm:"size:40"`
}

func (ApprovalLink) TableName() string { return "approval_links" }

// IsDuplicateKey recognises a unique-index violation on SQLite and MySQL.
func IsDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "unique constraint") || strings.Contains(m, "constraint failed: unique") || strings.Contains(m, "duplicate entry")
}
