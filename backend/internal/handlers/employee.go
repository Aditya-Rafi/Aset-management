package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/akses"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

// EmployeeHandler covers the onboarding/offboarding flow of employees.
// Plain create/update/delete live in NewEmployeeResource.
type EmployeeHandler struct{ DB *gorm.DB }

// Offboard marks an employee as no longer working here. It refuses while the
// person still holds assets: those must come back through a return document
// first, otherwise the inventory would show assets held by someone who left.
func (h *EmployeeHandler) Offboard(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Karyawan")
		return
	}
	var in struct {
		LeftAt *string `json:"left_at"`
		Reason *string `json:"reason"`
	}
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(r, &in); err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	trimPtr(&in.LeftAt)
	trimPtr(&in.Reason)
	leftAt := time.Now().Format("2006-01-02")
	if in.LeftAt != nil {
		if _, err := time.Parse("2006-01-02", *in.LeftAt); err != nil {
			httpx.Invalid(w, map[string]string{"left_at": "Tanggal keluar tidak valid (YYYY-MM-DD)"})
			return
		}
		leftAt = *in.LeftAt
	}
	if in.Reason != nil && len([]rune(*in.Reason)) > 255 {
		httpx.Invalid(w, map[string]string{"reason": "Alasan maksimal 255 karakter"})
		return
	}

	var emp models.Employee
	status := 0
	msg := ""
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&emp, id).Error; err != nil {
			status, msg = http.StatusNotFound, "Karyawan tidak ditemukan"
			return nil
		}
		if emp.Status == models.EmployeeInactive {
			status, msg = http.StatusConflict, "Karyawan sudah nonaktif"
			return nil
		}
		var held int64
		if err := tx.Model(&models.Asset{}).Where("holder_id = ?", id).Count(&held).Error; err != nil {
			return err
		}
		if held > 0 {
			status = http.StatusConflict
			msg = fmt.Sprintf("Karyawan masih memegang %d aset. Buat dokumen pengembalian dulu, baru nonaktifkan.", held)
			return nil
		}
		if emp.JoinedAt != nil && leftAt < *emp.JoinedAt {
			status, msg = http.StatusUnprocessableEntity, "Tanggal keluar tidak boleh sebelum tanggal masuk"
			return nil
		}
		emp.Status, emp.LeftAt, emp.ExitReason = models.EmployeeInactive, &leftAt, in.Reason
		if err := tx.Omit("AssetCount").Save(&emp).Error; err != nil {
			return err
		}
		return h.audit(tx, r, "karyawan_nonaktif", emp, map[string]any{"left_at": leftAt, "reason": in.Reason})
	})
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if status != 0 {
		httpx.Fail(w, status, msg)
		return
	}
	httpx.OK(w, "Karyawan dinonaktifkan", emp)
}

// Reactivate brings a former employee back (rehire). The leaving date and
// reason are cleared; the audit log keeps the earlier stint.
func (h *EmployeeHandler) Reactivate(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Karyawan")
		return
	}
	var emp models.Employee
	status, msg := 0, ""
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&emp, id).Error; err != nil {
			status, msg = http.StatusNotFound, "Karyawan tidak ditemukan"
			return nil
		}
		if emp.Status == models.EmployeeActive {
			status, msg = http.StatusConflict, "Karyawan sudah aktif"
			return nil
		}
		prev := map[string]any{"left_at": emp.LeftAt, "reason": emp.ExitReason}
		today := time.Now().Format("2006-01-02")
		emp.Status, emp.LeftAt, emp.ExitReason, emp.JoinedAt = models.EmployeeActive, nil, nil, &today
		if err := tx.Omit("AssetCount").Save(&emp).Error; err != nil {
			return err
		}
		return h.audit(tx, r, "karyawan_aktif_kembali", emp, prev)
	})
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if status != 0 {
		httpx.Fail(w, status, msg)
		return
	}
	httpx.OK(w, "Karyawan diaktifkan kembali", emp)
}

func (h *EmployeeHandler) audit(tx *gorm.DB, r *http.Request, action string, e models.Employee, detail map[string]any) error {
	b, _ := json.Marshal(detail)
	d := string(b)
	ent, ref := "employees", fmt.Sprint(e.ID)
	ip := strings.TrimSpace(r.Header.Get(akses.HeaderIPKlien))
	row := models.AuditLog{Action: action, Entity: &ent, EntityRef: &ref, DetailJSON: &d, IP: &ip,
		CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if u := CurrentUser(r); u != nil {
		row.UserRef, row.Username = &u.Ref, &u.Username
	}
	return tx.Create(&row).Error
}
