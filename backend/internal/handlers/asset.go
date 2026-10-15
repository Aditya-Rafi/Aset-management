package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/service"
)

type AssetHandler struct {
	DB            *gorm.DB
	Images        ImageStore
	PublicBaseURL string
}

var assetSort = map[string]string{
	"tag": "assets.tag", "category": "categories.name", "brand": "assets.brand", "model": "assets.model",
	"status": "assets.status", "holder": "employees.name", "location": "assets.location",
	"warranty_until": "assets.warranty_until", "purchase_date": "assets.purchase_date", "created_at": "assets.created_at",
}

func userID(r *http.Request) *uint {
	if u := CurrentUser(r); u != nil {
		id := u.ID
		return &id
	}
	return nil
}

// filtered builds the shared list/export query from the URL parameters.
func (h *AssetHandler) filtered(r *http.Request) *gorm.DB {
	q := h.DB.Model(&models.Asset{}).
		Joins("LEFT JOIN categories ON categories.id = assets.category_id").
		Joins("LEFT JOIN employees ON employees.id = assets.holder_id")
	p := r.URL.Query()

	if s := strings.TrimSpace(p.Get("search")); s != "" {
		l := "%" + httpx.EscapeLike(s) + "%"
		q = q.Where(`(assets.tag LIKE ? ESCAPE '!' OR assets.brand LIKE ? ESCAPE '!' OR assets.model LIKE ? ESCAPE '!'
			OR assets.serial_number LIKE ? ESCAPE '!' OR assets.cpu LIKE ? ESCAPE '!' OR assets.location LIKE ? ESCAPE '!'
			OR assets.notes LIKE ? ESCAPE '!' OR assets.vendor LIKE ? ESCAPE '!'
			OR categories.name LIKE ? ESCAPE '!' OR employees.name LIKE ? ESCAPE '!')`,
			l, l, l, l, l, l, l, l, l, l)
	}
	if v := p.Get("status"); v != "" {
		var sts []string
		for _, s := range strings.Split(v, ",") {
			if models.ValidStatus(s) {
				sts = append(sts, s)
			}
		}
		if len(sts) > 0 {
			q = q.Where("assets.status IN ?", sts)
		}
	}
	if v := p.Get("category_id"); v != "" {
		q = q.Where("assets.category_id = ?", v)
	}
	if v := p.Get("holder_id"); v != "" {
		q = q.Where("assets.holder_id = ?", v)
	}
	if v := p.Get("location"); v != "" {
		q = q.Where("assets.location = ?", v)
	}
	if v := p.Get("ids"); v != "" {
		var ids []uint
		for _, s := range strings.Split(v, ",") {
			if n, err := strconv.ParseUint(s, 10, 64); err == nil {
				ids = append(ids, uint(n))
			}
		}
		q = q.Where("assets.id IN ?", ids)
	}
	return q
}

func (h *AssetHandler) List(w http.ResponseWriter, r *http.Request) {
	lq := httpx.ParseListQuery(r)
	q := h.filtered(r)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	order := "assets.id DESC"
	if col, ok := assetSort[lq.Sort]; ok {
		order = col + map[bool]string{true: " DESC", false: " ASC"}[lq.Desc]
	}
	items := []models.Asset{}
	err := q.Select("assets.*").Preload("Category").Preload("Holder").Preload("Images").
		Order(order).Limit(lq.PerPage).Offset(lq.Offset()).Find(&items).Error
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.List(w, items, httpx.Meta{Total: total, Page: lq.Page, PerPage: lq.PerPage})
}

func (h *AssetHandler) load(where string, arg interface{}) (*models.Asset, error) {
	a := new(models.Asset)
	err := h.DB.Preload("Category").Preload("Holder").Preload("Images").Where(where, arg).First(a).Error
	return a, err
}

func (h *AssetHandler) notFound(w http.ResponseWriter, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.NotFound(w, "Aset")
		return
	}
	httpx.ServerError(w, err)
}

func (h *AssetHandler) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Aset")
		return
	}
	a, err := h.load("id = ?", id)
	if err != nil {
		h.notFound(w, err)
		return
	}
	httpx.OK(w, "Aset berhasil diambil", a)
}

// ByTag resolves what a scanned sticker contains.
func (h *AssetHandler) ByTag(w http.ResponseWriter, r *http.Request) {
	a, err := h.load("tag = ?", strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "tag"))))
	if err != nil {
		h.notFound(w, err)
		return
	}
	httpx.OK(w, "Aset berhasil diambil", a)
}

func (h *AssetHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in service.AssetInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	a, err := service.CreateAsset(h.DB, in, userID(r))
	if err != nil {
		serviceError(w, err, "Aset")
		return
	}
	full, err := h.load("id = ?", a.ID)
	if err != nil {
		full = a
	}
	httpx.Created(w, "Aset berhasil ditambahkan", full)
}

func (h *AssetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Aset")
		return
	}
	var in service.AssetInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := service.UpdateAsset(h.DB, id, in, userID(r)); err != nil {
		serviceError(w, err, "Aset")
		return
	}
	full, err := h.load("id = ?", id)
	if err != nil {
		h.notFound(w, err)
		return
	}
	httpx.OK(w, "Aset berhasil diperbarui", full)
}

func (h *AssetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Aset")
		return
	}
	a, err := h.load("id = ?", id)
	if err != nil {
		h.notFound(w, err)
		return
	}
	var used int64
	h.DB.Model(&models.TransactionDetail{}).Where("asset_id = ?", id).Count(&used)
	if used > 0 || a.Status == models.StatusInUse {
		httpx.Fail(w, http.StatusConflict, "Aset sudah punya riwayat serah terima dan tidak bisa dihapus. Ubah statusnya menjadi Dimusnahkan")
		return
	}
	if err := h.DB.Delete(&models.Asset{}, id).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	for _, img := range a.Images {
		h.Images.Remove(img.Name)
	}
	httpx.OK(w, "Aset berhasil dihapus", nil)
}

func (h *AssetHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Aset")
		return
	}
	var in struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := service.ChangeStatus(h.DB, id, in.Status, in.Note, userID(r)); err != nil {
		serviceError(w, err, "Aset")
		return
	}
	full, err := h.load("id = ?", id)
	if err != nil {
		h.notFound(w, err)
		return
	}
	httpx.OK(w, "Status aset diperbarui", full)
}

func (h *AssetHandler) AddNote(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Aset")
		return
	}
	var in struct {
		Message string `json:"message"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := service.AddNote(h.DB, id, in.Message, userID(r)); err != nil {
		serviceError(w, err, "Aset")
		return
	}
	httpx.Created(w, "Catatan ditambahkan", nil)
}

func (h *AssetHandler) Logs(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Aset")
		return
	}
	logs := []models.AssetLog{}
	if err := h.DB.Preload("User").Where("asset_id = ?", id).Order("id DESC").Limit(300).Find(&logs).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.OK(w, "Riwayat berhasil diambil", logs)
}

// ---- photos ----

func (h *AssetHandler) UploadImages(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Aset")
		return
	}
	var a models.Asset
	if err := h.DB.Select("id").Preload("Images").First(&a, id).Error; err != nil {
		h.notFound(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(MaxImagesPerAsset)*maxImageBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Form tidak valid atau ukuran file terlalu besar")
		return
	}
	files := r.MultipartForm.File["images"]
	if len(files) == 0 {
		httpx.Invalid(w, map[string]string{"images": "Pilih foto terlebih dahulu"})
		return
	}
	if len(a.Images)+len(files) > MaxImagesPerAsset {
		httpx.Invalid(w, map[string]string{"images": "Maksimal 5 foto per aset"})
		return
	}
	var saved []string
	for _, fh := range files {
		name, err := h.Images.Save(fh)
		if err != nil {
			h.Images.Remove(saved...)
			httpx.Invalid(w, map[string]string{"images": err.Error()})
			return
		}
		saved = append(saved, name)
	}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		for _, n := range saved {
			if err := tx.Create(&models.AssetImage{AssetID: id, Name: n}).Error; err != nil {
				return err
			}
		}
		return service.LogPhoto(tx, id, userID(r), strconv.Itoa(len(saved))+" foto ditambahkan")
	})
	if err != nil {
		h.Images.Remove(saved...)
		httpx.ServerError(w, err)
		return
	}
	full, _ := h.load("id = ?", id)
	httpx.Created(w, "Foto ditambahkan", full)
}

func (h *AssetHandler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	imgID, err := strconv.ParseUint(chi.URLParam(r, "imageId"), 10, 64)
	if !ok || err != nil {
		httpx.NotFound(w, "Foto")
		return
	}
	var img models.AssetImage
	if err := h.DB.Where("id = ? AND asset_id = ?", imgID, id).First(&img).Error; err != nil {
		httpx.NotFound(w, "Foto")
		return
	}
	if err := h.DB.Delete(&img).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	h.Images.Remove(img.Name)
	_ = service.LogPhoto(h.DB, id, userID(r), "Foto dihapus")
	httpx.OK(w, "Foto dihapus", nil)
}

func (h *AssetHandler) Image(w http.ResponseWriter, r *http.Request) {
	h.Images.Serve(w, r, chi.URLParam(r, "name"))
}

// ---- stickers ----

func (h *AssetHandler) tagOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Aset")
		return "", false
	}
	var a models.Asset
	if err := h.DB.Select("id", "tag").First(&a, id).Error; err != nil {
		h.notFound(w, err)
		return "", false
	}
	return a.Tag, true
}

func writePNG(w http.ResponseWriter, b []byte, err error) {
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(b)
}

// QR encodes the asset page URL, so any phone camera opens the asset directly.
func (h *AssetHandler) QR(w http.ResponseWriter, r *http.Request) {
	tag, ok := h.tagOf(w, r)
	if !ok {
		return
	}
	b, err := QRPNG(AssetURL(BaseURL(h.PublicBaseURL, r), tag), sizeParam(r, 300))
	writePNG(w, b, err)
}

// Barcode renders the plain tag as Code 128 for handheld barcode scanners.
func (h *AssetHandler) Barcode(w http.ResponseWriter, r *http.Request) {
	tag, ok := h.tagOf(w, r)
	if !ok {
		return
	}
	b, err := BarcodePNG(tag, sizeParam(r, 400))
	writePNG(w, b, err)
}

// ConditionReport lists the physical checks of an asset and what changed between them.
func (h *AssetHandler) ConditionReport(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(r)
	if !ok {
		httpx.NotFound(w, "Aset")
		return
	}
	var n int64
	h.DB.Model(&models.Asset{}).Where("id = ?", id).Count(&n)
	if n == 0 {
		httpx.NotFound(w, "Aset")
		return
	}
	rep, err := service.BuildConditionReport(h.DB, id)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.OK(w, "Laporan kondisi berhasil diambil", rep)
}
