// Package auth owns accounts, roles, sessions and the authentication audit trail.
//
// It is a port of the NetAudit login system (SPEK-07 and SPEK-14): short-lived
// JWT access tokens, rotating opaque refresh tokens, PBKDF2 passwords, login
// throttling, self registration with admin approval, e-mail OTP password reset
// and a guest mode. Behaviour, messages and limits are kept identical.
package auth

import "github.com/Aditya-Rafi/asset-management-system/backend/internal/models"

// RefPrefix is the prefix of account refs.
const RefPrefix = "usr"

type (
	User          = models.User
	PasswordReset = models.PasswordReset
	RefreshToken  = models.RefreshToken
	AuditLog      = models.AuditLog
	ApprovalLink  = models.ApprovalLink
)
