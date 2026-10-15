package handlers

import (
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

func trimPtr(p **string) {
	if *p == nil {
		return
	}
	s := strings.TrimSpace(**p)
	if s == "" {
		*p = nil
		return
	}
	*p = &s
}

func maxPtr(v *httpx.Validator, field string, p *string, label string, n int) {
	if p != nil {
		v.MaxLen(field, *p, label, n)
	}
}

func NewEmployeeResource(db *gorm.DB) *Resource[models.Employee] {
	return &Resource[models.Employee]{
		DB: db, Label: "Karyawan", Table: "employees",
		Search: []string{"employees.name", "employees.nik", "employees.department", "employees.hp", "employees.email"},
		Sort: map[string]string{
			"name": "employees.name", "department": "employees.department", "hp": "employees.hp",
			"email": "employees.email", "created_at": "employees.created_at",
		},
		Default: "employees.name ASC",
		Columns: "employees.*, (SELECT COUNT(*) FROM assets a WHERE a.holder_id = employees.id) AS asset_count",
		Filter: func(q *gorm.DB, r *http.Request) *gorm.DB {
			switch r.URL.Query().Get("status") {
			case models.EmployeeActive, models.EmployeeInactive:
				q = q.Where("employees.status = ?", r.URL.Query().Get("status"))
			}
			return q
		},
		Prepare: func(m *models.Employee) {
			// Status is never taken from the client: new people start active,
			// leaving is a separate action (Offboard).
			m.Status, m.LeftAt, m.ExitReason = models.EmployeeActive, nil, nil
			if m.JoinedAt == nil {
				today := time.Now().Format("2006-01-02")
				m.JoinedAt = &today
			}
		},
		Preserve: func(before, after *models.Employee) {
			after.Status, after.LeftAt, after.ExitReason = before.Status, before.LeftAt, before.ExitReason
		},
		BeforeDelete: func(db *gorm.DB, id uint) string {
			var held, docs int64
			db.Model(&models.Asset{}).Where("holder_id = ?", id).Count(&held)
			db.Model(&models.Transaction{}).Where("employee_id = ? OR from_employee_id = ?", id, id).Count(&docs)
			switch {
			case held > 0:
				return "Karyawan masih memegang aset. Buat dokumen pengembalian, lalu nonaktifkan."
			case docs > 0:
				return "Karyawan punya riwayat dokumen serah terima. Nonaktifkan saja agar riwayatnya tetap utuh."
			}
			return ""
		},
		Normalize: func(m *models.Employee) {
			m.Name = strings.TrimSpace(m.Name)
			trimPtr(&m.JoinedAt)
			trimPtr(&m.NIK)
			trimPtr(&m.Department)
			trimPtr(&m.Hp)
			trimPtr(&m.Email)
			trimPtr(&m.Address)
		},
		Validate: func(m *models.Employee, v *httpx.Validator) {
			v.Required("name", m.Name, "Nama")
			v.MaxLen("name", m.Name, "Nama", 255)
			v.Email("email", m.Email, "Email")
			if m.JoinedAt != nil {
				if _, err := time.Parse("2006-01-02", *m.JoinedAt); err != nil {
					v.Add("joined_at", "Tanggal masuk tidak valid (YYYY-MM-DD)")
				}
			}
			maxPtr(v, "nik", m.NIK, "NIK / ID", 64)
			maxPtr(v, "department", m.Department, "Departemen", 255)
			maxPtr(v, "hp", m.Hp, "HP", 255)
			maxPtr(v, "email", m.Email, "Email", 255)
		},
	}
}

func NewCategoryResource(db *gorm.DB) *Resource[models.Category] {
	return &Resource[models.Category]{
		DB: db, Label: "Kategori", Table: "categories",
		Search:  []string{"categories.name", "categories.code"},
		Sort:    map[string]string{"name": "categories.name", "code": "categories.code", "created_at": "categories.created_at"},
		Default: "categories.name ASC",
		Normalize: func(m *models.Category) {
			m.Name = strings.TrimSpace(m.Name)
			m.Code = strings.ToUpper(strings.TrimSpace(m.Code))
			if m.Inspection == "" {
				m.Inspection = models.ProfileNone
			}
		},
		Validate: func(m *models.Category, v *httpx.Validator) {
			v.Required("name", m.Name, "Nama")
			v.MaxLen("name", m.Name, "Nama", 255)
			if !models.ValidCode(m.Code) {
				v.Add("code", "Kode harus 2-5 huruf (A-Z), mis. LPT")
			}
			if !models.ValidInspectionProfile(m.Inspection) {
				v.Add("inspection", "Jenis pemeriksaan tidak dikenal")
			}
		},
		Check: func(db *gorm.DB, m *models.Category, v *httpx.Validator) {
			var n int64
			db.Model(&models.Category{}).Where("id <> ? AND LOWER(name) = LOWER(?)", m.ID, m.Name).Count(&n)
			if n > 0 {
				v.Add("name", "Nama kategori sudah ada")
			}
			db.Model(&models.Category{}).Where("id <> ? AND code = ?", m.ID, m.Code).Count(&n)
			if n > 0 {
				v.Add("code", "Kode sudah dipakai kategori lain")
			}
		},
	}
}
