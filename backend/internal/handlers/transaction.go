package handlers

import (
	"errors"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/pdf"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/service"
)

type TransactionHandler struct {
	DB      *gorm.DB
	Org     pdf.Org
	BastOrg pdf.BastOrg
}

var txSort = map[string]string{
	"order_number": "transactions.order_number", "type": "transactions.type", "employee": "employees.name",
	"note": "transactions.note", "created_at": "transactions.created_at",
}

func (h *TransactionHandler) List(w http.ResponseWriter, r *http.Request) {
	lq := httpx.ParseListQuery(r)
	q := h.DB.Model(&models.Transaction{}).
		Joins("LEFT JOIN employees ON employees.id = transactions.employee_id")
	if lq.Search != "" {
		l := "%" + httpx.EscapeLike(lq.Search) + "%"
		q = q.Where(`(transactions.order_number LIKE ? ESCAPE '!' OR transactions.note LIKE ? ESCAPE '!'
			OR employees.name LIKE ? ESCAPE '!'
			OR EXISTS (SELECT 1 FROM transaction_details td JOIN assets a ON a.id = td.asset_id
				WHERE td.transaction_id = transactions.id
				AND (a.tag LIKE ? ESCAPE '!' OR a.brand LIKE ? ESCAPE '!' OR a.model LIKE ? ESCAPE '!')))`,
			l, l, l, l, l, l)
	}
	if t := r.URL.Query().Get("type"); t == models.TxOut || t == models.TxIn {
		q = q.Where("transactions.type = ?", t)
	}
	if e := r.URL.Query().Get("employee_id"); e != "" {
		q = q.Where("transactions.employee_id = ?", e)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	order := "transactions.id DESC"
	if col, ok := txSort[lq.Sort]; ok {
		order = col + map[bool]string{true: " DESC", false: " ASC"}[lq.Desc]
	}
	items := []models.Transaction{}
	err := q.Select("transactions.*").
		Preload("Employee").Preload("FromEmployee").Preload("Details.Asset.Category").
		Order(order).Limit(lq.PerPage).Offset(lq.Offset()).Find(&items).Error
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.List(w, items, httpx.Meta{Total: total, Page: lq.Page, PerPage: lq.PerPage})
}

func (h *TransactionHandler) load(id uint) (*models.Transaction, error) {
	t := new(models.Transaction)
	err := h.DB.Preload("Employee").Preload("FromEmployee").Preload("CreatedBy").Preload("Details.Asset.Category").
		Preload("Inspections").First(t, id).Error
	return t, err
}

func (h *TransactionHandler) notFound(w http.ResponseWriter, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.NotFound(w, "Dokumen")
		return
	}
	httpx.ServerError(w, err)
}

func (h *TransactionHandler) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Dokumen")
		return
	}
	t, err := h.load(id)
	if err != nil {
		h.notFound(w, err)
		return
	}
	httpx.OK(w, "Dokumen berhasil diambil", t)
}

func (h *TransactionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in service.CreateTransactionInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	t, err := service.CreateTransaction(h.DB, in, userID(r))
	if err != nil {
		serviceError(w, err, "Dokumen")
		return
	}
	full, err := h.load(t.ID)
	if err != nil {
		full = t
	}
	msg := map[string]string{
		models.TxOut: "Serah terima berhasil dicatat", models.TxIn: "Pengembalian berhasil dicatat",
		models.TxTransfer: "Pindah tangan berhasil dicatat",
	}[t.Type]
	httpx.Created(w, msg, full)
}

func (h *TransactionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Dokumen")
		return
	}
	if err := service.DeleteTransaction(h.DB, id, userID(r)); err != nil {
		serviceError(w, err, "Dokumen")
		return
	}
	httpx.OK(w, "Dokumen dibatalkan dan status aset dikembalikan", nil)
}

func (h *TransactionHandler) PDF(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Dokumen")
		return
	}
	t, err := h.load(id)
	if err != nil {
		h.notFound(w, err)
		return
	}
	operator := ""
	if t.CreatedBy != nil {
		operator = t.CreatedBy.Name
	} else if u := CurrentUser(r); u != nil {
		operator = u.Name
	}
	b, err := pdf.Receipt(t, operator, h.Org)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	name := strings.ReplaceAll(t.OrderNumber, "/", "-")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+name+`.pdf"`)
	_, _ = w.Write(b)
}

// BAST serves the Berita Acara with the physical-check results of the document.
func (h *TransactionHandler) BAST(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Dokumen")
		return
	}
	t, err := h.load(id)
	if err != nil {
		h.notFound(w, err)
		return
	}
	op := pdf.BastParty{}
	if t.CreatedBy != nil {
		op.Name = t.CreatedBy.Name
	} else if u := CurrentUser(r); u != nil {
		op.Name = u.Name
	}
	b, err := pdf.BAST(t, op, h.BastOrg)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="BAST-`+t.OrderNumber+`.pdf"`)
	_, _ = w.Write(b)
}
