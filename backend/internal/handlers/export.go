package handlers

import (
	"bytes"
	"encoding/csv"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/service"
)

// csvCell neutralises spreadsheet formulas ("=cmd|..." injection) in text cells.
func csvCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '@', '\t', '\r':
		return "'" + s
	case '-':
		if len(s) > 1 {
			return "'" + s
		}
	}
	return s
}

func sp(p *string) string {
	if p == nil {
		return ""
	}
	return csvCell(*p)
}

func dateStr(d models.Date) string {
	if d.IsZero() {
		return ""
	}
	return d.Format("2006-01-02")
}

func writeCSV(w http.ResponseWriter, r *http.Request, filename string, rows [][]string) {
	var buf bytes.Buffer
	buf.WriteString("\xef\xbb\xbf") // BOM so Excel reads UTF-8 correctly
	cw := csv.NewWriter(&buf)
	// Excel in Indonesian locales expects ';'; the importer reads both.
	cw.Comma = ';'
	if r.URL.Query().Get("delimiter") == "comma" {
		cw.Comma = ','
	}
	_ = cw.WriteAll(rows)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write(buf.Bytes())
}

// Export downloads the (filtered) inventory as CSV.
func (h *AssetHandler) Export(w http.ResponseWriter, r *http.Request) {
	items := []models.Asset{}
	if err := h.filtered(r).Select("assets.*").Preload("Category").Preload("Holder").
		Order("categories.name ASC, assets.tag ASC").Limit(20000).Find(&items).Error; err != nil {
		httpx.ServerError(w, err)
		return
	}
	rows := [][]string{append([]string{"Kode"}, service.ImportTemplateHeader...)}
	for _, a := range items {
		cat, holder := "", ""
		if a.Category != nil {
			cat = csvCell(a.Category.Name)
		}
		if a.Holder != nil {
			holder = csvCell(a.Holder.Name)
		}
		year, price := "", ""
		if a.ManufactureYear != nil {
			year = strconv.Itoa(*a.ManufactureYear)
		}
		if a.PurchasePrice != nil {
			price = strconv.FormatInt(*a.PurchasePrice, 10)
		}
		rows = append(rows, []string{
			a.Tag, cat, sp(a.Brand), sp(a.Model), sp(a.SerialNumber), sp(a.CPU), sp(a.RAM), sp(a.Storage), sp(a.OS), sp(a.ScreenSize),
			year, dateStr(a.PurchaseDate), price, dateStr(a.WarrantyUntil), sp(a.Vendor),
			service.StatusLabel(a.Status), sp(a.Location), holder, strconv.Itoa(a.Quantity), sp(a.Notes),
		})
	}
	writeCSV(w, r, "inventaris-"+time.Now().Format("20060102")+".csv", rows)
}

// ImportTemplate serves an example file with the expected columns.
func (h *AssetHandler) ImportTemplate(w http.ResponseWriter, r *http.Request) {
	writeCSV(w, r, "template-impor-aset.csv", [][]string{
		service.ImportTemplateHeader,
		{"Laptop", "Lenovo", "IdeaPad Slim 3", "PF3ABC12", "Intel Core i5-12450H", "16 GB", "512 GB SSD", "Windows 11", "", "2023", "2025-02-11", "8500000", "2027-02-11", "Toko Contoh", "Dipakai", "", "Budi Santoso", "1", ""},
		{"Monitor", "LG", "24MR400-B", "", "", "", "", "", "24 inch", "2025", "2025-06-10", "", "", "", "Tersedia", "Ruang Server", "", "1", "Cadangan"},
	})
}

// Import creates assets from an uploaded CSV. ?dry_run=1 only validates.
func (h *AssetHandler) Import(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "File terlalu besar atau form tidak valid (maks. 8 MB)")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		httpx.Invalid(w, map[string]string{"file": "Pilih file CSV terlebih dahulu"})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "File tidak terbaca")
		return
	}
	dry := r.URL.Query().Get("dry_run") == "1" || strings.EqualFold(r.URL.Query().Get("dry_run"), "true")
	res, err := service.ImportAssets(h.DB, data, dry, userID(r))
	if err != nil {
		serviceError(w, err, "Aset")
		return
	}
	status := http.StatusOK
	if len(res.Errors) > 0 {
		status = http.StatusUnprocessableEntity
	}
	msg := "Impor selesai"
	switch {
	case len(res.Errors) > 0:
		msg = "Impor dibatalkan: ada baris yang bermasalah, tidak ada data yang disimpan"
	case dry:
		msg = "Pemeriksaan berhasil, belum ada data yang disimpan"
	}
	httpx.JSON(w, status, map[string]interface{}{"success": len(res.Errors) == 0, "message": msg, "data": res})
}
