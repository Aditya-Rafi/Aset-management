// Package service holds the business rules that span several tables.
package service

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

// ValidationError carries field-level messages back to the HTTP layer.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, e.Fields[k])
	}
	return strings.Join(parts, "; ")
}

func invalid(field, msg string) error { return &ValidationError{map[string]string{field: msg}} }

var ErrNotFound = errors.New("not found")

// ConflictError means the input is fine but the current state forbids the action.
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

// IsDuplicate reports a unique-constraint violation on MySQL or SQLite.
func IsDuplicate(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "duplicate") || strings.Contains(s, "unique constraint")
}

// NextTag returns the next asset tag for a category code: LPT-0001, LPT-0002, ...
// The unique index is the final guard; callers retry on IsDuplicate.
func NextTag(tx *gorm.DB, code string) (string, error) {
	var last string
	err := tx.Model(&models.Asset{}).
		Where("tag LIKE ?", code+"-%").
		Order("LENGTH(tag) DESC, tag DESC").Limit(1).Pluck("tag", &last).Error
	if err != nil {
		return "", err
	}
	n := 0
	if last != "" {
		n, _ = strconv.Atoi(strings.TrimPrefix(last, code+"-"))
	}
	return fmt.Sprintf("%s-%04d", code, n+1), nil
}

// NextDocNumber returns OUT-20261014-001 style numbers, one sequence per day and type.
func NextDocNumber(tx *gorm.DB, prefix string, at time.Time) (string, error) {
	base := fmt.Sprintf("%s-%s-", prefix, at.Format("20060102"))
	var last string
	err := tx.Model(&models.Transaction{}).
		Where("order_number LIKE ?", base+"%").
		Order("order_number DESC").Limit(1).Pluck("order_number", &last).Error
	if err != nil {
		return "", err
	}
	n := 0
	if last != "" {
		n, _ = strconv.Atoi(strings.TrimPrefix(last, base))
	}
	return fmt.Sprintf("%s%03d", base, n+1), nil
}

func addLog(tx *gorm.DB, assetID uint, userID *uint, action string, from, to *string, msg string, txID *uint) error {
	l := &models.AssetLog{AssetID: assetID, UserID: userID, Action: action, FromStatus: from, ToStatus: to, TransactionID: txID}
	if msg != "" {
		l.Message = &msg
	}
	return tx.Create(l).Error
}

func strp(s string) *string { return &s }

func clean(p *string) *string {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return nil
	}
	return &s
}

func val(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
