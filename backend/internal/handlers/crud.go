package handlers

import (
	"net/http"
	"reflect"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
)

// Resource is a generic CRUD endpoint set for a simple master-data model.
type Resource[T any] struct {
	DB       *gorm.DB
	Label    string            // human label, e.g. "Karyawan"
	Table    string            // SQL table name
	Search   []string          // qualified columns searched with LIKE
	Sort     map[string]string // api sort key -> qualified column
	Default  string            // default ORDER BY expression
	Joins    []string          // joins needed for search/sort
	Preload  []string
	Validate func(m *T, v *httpx.Validator)
	// Normalize trims/cleans input before validation.
	Normalize func(m *T)
	// Check runs database-backed validation (e.g. referenced rows exist).
	Check func(db *gorm.DB, m *T, v *httpx.Validator)
	// Filter narrows List by extra URL parameters.
	Filter func(q *gorm.DB, r *http.Request) *gorm.DB
	// Columns replaces the default "<table>.*" selection (e.g. computed counts).
	Columns string
	// Preserve copies server-owned fields from the stored row (before) back
	// onto a row decoded from the client (after) on Update.
	Preserve func(before, after *T)
	// BeforeDelete returns a reason to refuse the delete, or "".
	BeforeDelete func(db *gorm.DB, id uint) string
	// Prepare fills server-owned defaults on a row about to be created.
	Prepare func(m *T)
}

func (rs *Resource[T]) base() *gorm.DB {
	q := rs.DB.Model(new(T))
	for _, j := range rs.Joins {
		q = q.Joins(j)
	}
	return q
}

func (rs *Resource[T]) List(w http.ResponseWriter, r *http.Request) {
	lq := httpx.ParseListQuery(r)

	q := rs.base()
	if lq.Search != "" && len(rs.Search) > 0 {
		like := "%" + httpx.EscapeLike(lq.Search) + "%"
		conds := make([]string, len(rs.Search))
		args := make([]interface{}, len(rs.Search))
		for i, c := range rs.Search {
			conds[i] = c + " LIKE ? ESCAPE '!'"
			args[i] = like
		}
		q = q.Where("("+strings.Join(conds, " OR ")+")", args...)
	}

	if rs.Filter != nil {
		q = rs.Filter(q, r)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}

	order := rs.Default
	if col, ok := rs.Sort[lq.Sort]; ok {
		order = col
		if lq.Desc {
			order += " DESC"
		} else {
			order += " ASC"
		}
	}

	for _, p := range rs.Preload {
		q = q.Preload(p)
	}
	items := make([]T, 0, lq.PerPage)
	cols := rs.Table + ".*"
	if rs.Columns != "" {
		cols = rs.Columns
	}
	if err := q.Select(cols).Order(order).Limit(lq.PerPage).Offset(lq.Offset()).Find(&items).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.List(w, items, httpx.Meta{Total: total, Page: lq.Page, PerPage: lq.PerPage})
}

func (rs *Resource[T]) find(id uint) (*T, error) {
	m := new(T)
	q := rs.DB
	for _, p := range rs.Preload {
		q = q.Preload(p)
	}
	if err := q.First(m, id).Error; err != nil {
		return nil, err
	}
	return m, nil
}

func (rs *Resource[T]) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, rs.Label)
		return
	}
	m, err := rs.find(id)
	if err != nil {
		rs.notFoundOrErr(w, err)
		return
	}
	httpx.OK(w, rs.Label+" berhasil diambil", m)
}

func (rs *Resource[T]) Create(w http.ResponseWriter, r *http.Request) {
	m := new(T)
	if err := httpx.DecodeJSON(r, m); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	resetServerFields(m)
	if rs.Prepare != nil {
		rs.Prepare(m)
	}
	if !rs.validate(w, m) {
		return
	}
	if err := rs.DB.Omit(clause.Associations).Create(m).Error; err != nil {
		rs.dbError(w, err)
		return
	}
	created, err := rs.find(idOf(m))
	if err != nil {
		created = m
	}
	httpx.Created(w, rs.Label+" berhasil ditambahkan", created)
}

func (rs *Resource[T]) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, rs.Label)
		return
	}
	// Plain First (no preload): associations must not be written back.
	m := new(T)
	if err := rs.DB.First(m, id).Error; err != nil {
		rs.notFoundOrErr(w, err)
		return
	}
	created := reflect.ValueOf(m).Elem().FieldByName("CreatedAt").Interface()
	before := *m
	if err := httpx.DecodeJSON(r, m); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	setID(m, id)
	if rs.Preserve != nil {
		rs.Preserve(&before, m)
	}
	reflect.ValueOf(m).Elem().FieldByName("CreatedAt").Set(reflect.ValueOf(created))
	if !rs.validate(w, m) {
		return
	}
	if err := rs.DB.Omit(clause.Associations).Save(m).Error; err != nil {
		rs.dbError(w, err)
		return
	}
	updated, err := rs.find(id)
	if err != nil {
		updated = m
	}
	httpx.OK(w, rs.Label+" berhasil diperbarui", updated)
}

func (rs *Resource[T]) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, rs.Label)
		return
	}
	if rs.BeforeDelete != nil {
		if why := rs.BeforeDelete(rs.DB, id); why != "" {
			httpx.Fail(w, http.StatusConflict, why)
			return
		}
	}
	res := rs.DB.Delete(new(T), id)
	if res.Error != nil {
		rs.dbError(w, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		httpx.NotFound(w, rs.Label)
		return
	}
	httpx.OK(w, rs.Label+" berhasil dihapus", nil)
}

func (rs *Resource[T]) validate(w http.ResponseWriter, m *T) bool {
	if rs.Normalize != nil {
		rs.Normalize(m)
	}
	v := httpx.NewValidator()
	if rs.Validate != nil {
		rs.Validate(m, v)
	}
	if v.OK() && rs.Check != nil {
		rs.Check(rs.DB, m, v)
	}
	if !v.OK() {
		httpx.Invalid(w, v.Errors)
		return false
	}
	return true
}

func (rs *Resource[T]) notFoundOrErr(w http.ResponseWriter, err error) {
	if err == gorm.ErrRecordNotFound {
		httpx.NotFound(w, rs.Label)
		return
	}
	httpx.ServerError(w, err)
}

func (rs *Resource[T]) dbError(w http.ResponseWriter, err error) {
	if isFKError(err) {
		httpx.Fail(w, http.StatusConflict, rs.Label+" tidak dapat diproses karena masih terhubung dengan data lain")
		return
	}
	httpx.ServerError(w, err)
}

func isFKError(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "foreign key")
}

// --- reflection helpers for server-controlled fields ---

func resetServerFields(m interface{}) {
	setID(m, 0)
	v := reflect.ValueOf(m).Elem()
	for _, f := range []string{"CreatedAt", "UpdatedAt"} {
		if fv := v.FieldByName(f); fv.IsValid() && fv.CanSet() {
			fv.Set(reflect.Zero(fv.Type()))
		}
	}
}

func setID(m interface{}, id uint) {
	if fv := reflect.ValueOf(m).Elem().FieldByName("ID"); fv.IsValid() && fv.CanSet() {
		fv.SetUint(uint64(id))
	}
}

func idOf(m interface{}) uint {
	return uint(reflect.ValueOf(m).Elem().FieldByName("ID").Uint())
}
