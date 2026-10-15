package handlers

import (
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

type MonitorHandler struct{ DB *gorm.DB }

// Employees lists every employee with how many assets they hold right now.
func (h *MonitorHandler) Employees(w http.ResponseWriter, r *http.Request) {
	lq := httpx.ParseListQuery(r)
	q := h.DB.Table("employees")
	if lq.Search != "" {
		l := "%" + httpx.EscapeLike(lq.Search) + "%"
		q = q.Where(`(employees.name LIKE ? ESCAPE '!' OR employees.department LIKE ? ESCAPE '!')`, l, l)
	}
	if r.URL.Query().Get("holding") == "1" {
		q = q.Where("EXISTS (SELECT 1 FROM assets a WHERE a.holder_id = employees.id)")
	}
	// Default view: people still here, plus anyone (even inactive) who still
	// holds assets, since those need attention.
	switch r.URL.Query().Get("status") {
	case models.EmployeeActive, models.EmployeeInactive:
		q = q.Where("employees.status = ?", r.URL.Query().Get("status"))
	case "all":
	default:
		q = q.Where("(employees.status = ? OR EXISTS (SELECT 1 FROM assets a WHERE a.holder_id = employees.id))", models.EmployeeActive)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	order := "employees.name ASC"
	sorts := map[string]string{
		"name": "employees.name", "department": "employees.department",
		"asset_count": "asset_count", "transaction_count": "transaction_count",
	}
	if col, ok := sorts[lq.Sort]; ok {
		order = col + map[bool]string{true: " DESC", false: " ASC"}[lq.Desc]
	}
	rows := []struct {
		ID               uint    `json:"id"`
		Name             string  `json:"name"`
		Department       *string `json:"department"`
		Status           string  `json:"status"`
		AssetCount       int64   `json:"asset_count"`
		TransactionCount int64   `json:"transaction_count"`
	}{}
	err := q.Select(`employees.id, employees.name, employees.department, employees.status,
		(SELECT COUNT(*) FROM assets a WHERE a.holder_id = employees.id) AS asset_count,
		(SELECT COUNT(*) FROM transactions t WHERE t.employee_id = employees.id) AS transaction_count`).
		Order(order).Limit(lq.PerPage).Offset(lq.Offset()).Scan(&rows).Error
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.List(w, rows, httpx.Meta{Total: total, Page: lq.Page, PerPage: lq.PerPage})
}

// EmployeeAssets is the "listing inventaris karyawan": what one person holds now.
func (h *MonitorHandler) EmployeeAssets(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Karyawan")
		return
	}
	var emp models.Employee
	if err := h.DB.First(&emp, id).Error; err != nil {
		httpx.NotFound(w, "Karyawan")
		return
	}
	assets := []models.Asset{}
	if err := h.DB.Preload("Category").Where("holder_id = ?", id).Order("tag ASC").Find(&assets).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.OK(w, "Inventaris karyawan berhasil diambil", map[string]interface{}{"employee": emp, "assets": assets})
}

// EmployeeHistory lists every document that involved the employee.
func (h *MonitorHandler) EmployeeHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Karyawan")
		return
	}
	details := []models.TransactionDetail{}
	err := h.DB.Joins("JOIN transactions ON transactions.id = transaction_details.transaction_id").
		Where("transactions.employee_id = ?", id).
		Preload("Transaction").Preload("Asset.Category").
		Order("transaction_details.id DESC").Find(&details).Error
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.OK(w, "Riwayat berhasil diambil", details)
}

// Logs is the global activity feed ("update aset" log).
func (h *MonitorHandler) Logs(w http.ResponseWriter, r *http.Request) {
	lq := httpx.ParseListQuery(r)
	q := h.DB.Model(&models.AssetLog{}).Joins("JOIN assets ON assets.id = asset_logs.asset_id")
	if lq.Search != "" {
		l := "%" + httpx.EscapeLike(lq.Search) + "%"
		q = q.Where(`(assets.tag LIKE ? ESCAPE '!' OR assets.brand LIKE ? ESCAPE '!' OR assets.model LIKE ? ESCAPE '!' OR asset_logs.message LIKE ? ESCAPE '!')`, l, l, l, l)
	}
	if a := r.URL.Query().Get("action"); a != "" {
		q = q.Where("asset_logs.action = ?", a)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	logs := []models.AssetLog{}
	err := q.Select("asset_logs.*").Preload("Asset").Preload("User").
		Order("asset_logs.id DESC").Limit(lq.PerPage).Offset(lq.Offset()).Find(&logs).Error
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.List(w, logs, httpx.Meta{Total: total, Page: lq.Page, PerPage: lq.PerPage})
}

type DashboardHandler struct{ DB *gorm.DB }

type categoryRow struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Total     int64  `json:"total"`
	Available int64  `json:"available"`
	InUse     int64  `json:"in_use"`
	Repair    int64  `json:"repair"`
	Broken    int64  `json:"broken"`
	Disposed  int64  `json:"disposed"`
}

// Stats feeds the dashboard.
func (h *DashboardHandler) Stats(w http.ResponseWriter, r *http.Request) {
	byStatus := map[string]int64{}
	for _, s := range models.AllStatuses {
		byStatus[s] = 0
	}
	var sc []struct {
		Status string
		N      int64
	}
	if err := h.DB.Model(&models.Asset{}).Select("status, COUNT(*) AS n").Group("status").Scan(&sc).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	var total int64
	for _, s := range sc {
		byStatus[s.Status] = s.N
		total += s.N
	}

	cats := []categoryRow{}
	err := h.DB.Table("categories").Select(`categories.id, categories.name, COUNT(assets.id) AS total,
		COALESCE(SUM(CASE WHEN assets.status = 'available' THEN 1 ELSE 0 END), 0) AS available,
		COALESCE(SUM(CASE WHEN assets.status = 'in_use' THEN 1 ELSE 0 END), 0) AS in_use,
		COALESCE(SUM(CASE WHEN assets.status = 'repair' THEN 1 ELSE 0 END), 0) AS repair,
		COALESCE(SUM(CASE WHEN assets.status = 'broken' THEN 1 ELSE 0 END), 0) AS broken,
		COALESCE(SUM(CASE WHEN assets.status = 'disposed' THEN 1 ELSE 0 END), 0) AS disposed`).
		Joins("LEFT JOIN assets ON assets.category_id = categories.id").
		Group("categories.id, categories.name").Order("total DESC, categories.name ASC").Scan(&cats).Error
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	today := time.Now().Format("2006-01-02")
	soon := time.Now().AddDate(0, 0, 60).Format("2006-01-02")
	warranty := []models.Asset{}
	err = h.DB.Preload("Category").Preload("Holder").
		Where("warranty_until IS NOT NULL AND warranty_until >= ? AND warranty_until <= ? AND status <> ?", today, soon, models.StatusDisposed).
		Order("warranty_until ASC").Limit(8).Find(&warranty).Error
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	var expired int64
	h.DB.Model(&models.Asset{}).
		Where("warranty_until IS NOT NULL AND warranty_until < ? AND status NOT IN ?", today, []string{models.StatusDisposed}).Count(&expired)

	attention := []models.Asset{}
	err = h.DB.Preload("Category").Where("status IN ?", []string{models.StatusRepair, models.StatusBroken}).
		Order("updated_at DESC").Limit(8).Find(&attention).Error
	if err != nil {
		httpx.ServerError(w, err)
		return
	}

	var employees, holders int64
	h.DB.Model(&models.Employee{}).Where("status = ?", models.EmployeeActive).Count(&employees)
	var leftHolding int64
	h.DB.Model(&models.Asset{}).Joins("JOIN employees e ON e.id = assets.holder_id").
		Where("e.status = ?", models.EmployeeInactive).Distinct("assets.holder_id").Count(&leftHolding)
	h.DB.Model(&models.Asset{}).Where("holder_id IS NOT NULL").Distinct("holder_id").Count(&holders)

	logs := []models.AssetLog{}
	if err := h.DB.Preload("Asset").Preload("User").Order("id DESC").Limit(8).Find(&logs).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}

	httpx.OK(w, "OK", map[string]interface{}{
		"total": total, "by_status": byStatus, "by_category": cats,
		"employees": employees, "employees_holding": holders, "inactive_holding": leftHolding,
		"warranty_soon": warranty, "warranty_expired": expired,
		"attention": attention, "recent_logs": logs,
	})
}

type LookupHandler struct {
	DB            *gorm.DB
	PublicBaseURL string
	CompanyName   string
}

type option struct {
	ID   uint   `json:"id"`
	Text string `json:"text"`
}

const lookupLimit = 10

func likeParam(r *http.Request) (string, bool) {
	s := r.URL.Query().Get("search")
	if s == "" {
		return "", false
	}
	return "%" + httpx.EscapeLike(s) + "%", true
}

// Employees: ?holding=1 limits to people who currently hold assets (for returns).
func (h *LookupHandler) Employees(w http.ResponseWriter, r *http.Request) {
	q := h.DB.Table("employees").Select("id, name AS text").Order("name ASC").Limit(lookupLimit)
	if l, ok := likeParam(r); ok {
		q = q.Where(`name LIKE ? ESCAPE '!'`, l)
	}
	if r.URL.Query().Get("holding") == "1" {
		// Returns: anyone holding assets, even if already marked inactive.
		q = q.Where("EXISTS (SELECT 1 FROM assets a WHERE a.holder_id = employees.id)")
	} else if r.URL.Query().Get("include_inactive") != "1" {
		q = q.Where("employees.status = ?", models.EmployeeActive)
	}
	out := []option{}
	if err := q.Scan(&out).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *LookupHandler) Categories(w http.ResponseWriter, r *http.Request) {
	q := h.DB.Table("categories").Select("id, name AS text").Order("name ASC").Limit(50)
	if l, ok := likeParam(r); ok {
		q = q.Where(`name LIKE ? ESCAPE '!'`, l)
	}
	out := []option{}
	if err := q.Scan(&out).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// Assets searches tag/brand/model. ?status= and ?holder_id= narrow it down,
// e.g. only Tersedia assets for a handover.
func (h *LookupHandler) Assets(w http.ResponseWriter, r *http.Request) {
	q := h.DB.Table("assets").Select("assets.id, assets.tag AS text").Order("assets.tag ASC").Limit(lookupLimit)
	if l, ok := likeParam(r); ok {
		q = q.Where(`(assets.tag LIKE ? ESCAPE '!' OR assets.brand LIKE ? ESCAPE '!' OR assets.model LIKE ? ESCAPE '!' OR assets.serial_number LIKE ? ESCAPE '!')`, l, l, l, l)
	}
	if s := r.URL.Query().Get("status"); models.ValidStatus(s) {
		q = q.Where("assets.status = ?", s)
	}
	if hid := r.URL.Query().Get("holder_id"); hid != "" {
		q = q.Where("assets.holder_id = ?", hid)
	}
	out := []option{}
	if err := q.Scan(&out).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

var suggestFields = map[string][2]string{
	"brand": {"assets", "brand"}, "model": {"assets", "model"}, "location": {"assets", "location"},
	"vendor": {"assets", "vendor"}, "os": {"assets", "os"}, "cpu": {"assets", "cpu"},
	"ram": {"assets", "ram"}, "storage": {"assets", "storage"}, "screen_size": {"assets", "screen_size"},
	"department": {"employees", "department"},
}

// Suggest returns distinct values already used, to autocomplete free-text fields.
func (h *LookupHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	f, ok := suggestFields[r.URL.Query().Get("field")]
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "field tidak dikenal")
		return
	}
	q := h.DB.Table(f[0]).Where(f[1] + " IS NOT NULL AND " + f[1] + " <> ''").Distinct(f[1]).Order(f[1]).Limit(15)
	if l, ok := likeParam(r); ok {
		q = q.Where(f[1]+` LIKE ? ESCAPE '!'`, l)
	}
	var out []string
	if err := q.Pluck(f[1], &out).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	if out == nil {
		out = []string{}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// Config tells the SPA which address the QR stickers will contain.
func (h *LookupHandler) Config(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, "OK", map[string]interface{}{
		"public_base_url": BaseURL(h.PublicBaseURL, r),
		"configured":      h.PublicBaseURL != "",
		"company_name":    h.CompanyName,
	})
}
