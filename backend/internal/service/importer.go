package service

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

// ImportTemplateHeader is the column order of the downloadable template.
var ImportTemplateHeader = []string{
	"Kategori", "Merk", "Tipe", "Serial Number", "CPU", "RAM", "Storage", "OS", "Ukuran Layar",
	"Tahun Pembuatan", "Tanggal Pembelian", "Harga Beli", "Garansi Sampai", "Vendor",
	"Status", "Lokasi", "Pengguna", "Jumlah", "Keterangan",
}

type RowError struct {
	Row     int    `json:"row"` // spreadsheet line number, header = 1
	Message string `json:"message"`
}

type ImportResult struct {
	DryRun            bool       `json:"dry_run"`
	Rows              int        `json:"rows"`
	Created           int        `json:"created"`
	CategoriesCreated []string   `json:"categories_created"`
	EmployeesCreated  []string   `json:"employees_created"`
	Errors            []RowError `json:"errors"`
}

var headerAliases = map[string]string{
	"kategori": "category", "category": "category", "jenis": "category", "jenisbarang": "category",
	"merk": "brand", "merek": "brand", "brand": "brand",
	"tipe": "model", "type": "model", "model": "model",
	"serialnumber": "serial", "serial": "serial", "sn": "serial", "noseri": "serial",
	"cpu": "cpu", "processor": "cpu", "prosesor": "cpu",
	"ram": "ram", "memory": "ram", "memori": "ram",
	"storage": "storage", "hddssd": "storage", "hdd": "storage", "ssd": "storage", "penyimpanan": "storage",
	"os": "os", "sistemoperasi": "os",
	"ukuranlayar": "screen", "ukuran": "screen", "screensize": "screen", "screen": "screen",
	"tahunpembuatan": "year", "tahunproduksi": "year", "manufactureyear": "year",
	"tanggalpembelian": "purchase_date", "tahunpembelian": "purchase_date", "tglbeli": "purchase_date", "purchasedate": "purchase_date",
	"hargabeli": "price", "harga": "price", "price": "price",
	"garansisampai": "warranty", "garansi": "warranty", "warranty": "warranty",
	"vendor": "vendor", "supplier": "vendor", "toko": "vendor",
	"status": "status", "statuskondisi": "status",
	"lokasi": "location", "location": "location",
	"pengguna": "holder", "pemegang": "holder", "karyawan": "holder", "user": "holder", "holder": "holder",
	"jumlah": "qty", "qty": "qty", "quantity": "qty",
	"keterangan": "notes", "catatan": "notes", "notes": "notes",
}

var statusAliases = map[string]string{
	"tersedia": models.StatusAvailable, "available": models.StatusAvailable, "stok": models.StatusAvailable,
	"stock": models.StatusAvailable, "siap": models.StatusAvailable, "siapdigunakan": models.StatusAvailable,
	"bisadipakai": models.StatusAvailable, "baik": models.StatusAvailable,
	"dipakai": models.StatusInUse, "digunakan": models.StatusInUse, "inuse": models.StatusInUse, "terpakai": models.StatusInUse,
	"diservis": models.StatusRepair, "servis": models.StatusRepair, "service": models.StatusRepair,
	"repair": models.StatusRepair, "perbaikan": models.StatusRepair, "diperbaiki": models.StatusRepair,
	"rusak": models.StatusBroken, "broken": models.StatusBroken, "tidakbisadipakai": models.StatusBroken,
	"dimusnahkan": models.StatusDisposed, "musnah": models.StatusDisposed, "disposed": models.StatusDisposed,
}

var idMonths = strings.NewReplacer(
	"januari", "january", "februari", "february", "maret", "march", "mei", "may", "juni", "june", "juli", "july",
	"agustus", "august", "oktober", "october", "desember", "december",
	"agu", "aug", "okt", "oct", "des", "dec",
)

var dateLayouts = []string{
	"2006-01-02", "02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006", "02.01.2006",
	"2 January 2006", "02 January 2006", "2 Jan 2006", "02 Jan 2006", "January 2, 2006", "Jan 2, 2006",
}

// ParseFlexDate accepts ISO dates, dd/mm/yyyy and "10 October 2025" in English or Indonesian.
func ParseFlexDate(s string) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "-" {
		return time.Time{}, nil
	}
	s = idMonths.Replace(s)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("tanggal %q tidak dikenali (pakai 2025-10-10 atau 10/10/2025)", s)
}

var nonDigit = regexp.MustCompile(`[^0-9]`)

func normKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var errDryRun = errors.New("dry run")

// ImportAssets reads a CSV (comma or semicolon separated, as exported by Excel)
// and creates the assets in one transaction: either every row is imported or,
// when any row has a problem, none is and all problems are reported.
func ImportAssets(db *gorm.DB, data []byte, dryRun bool, userID *uint) (*ImportResult, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	delim := ','
	if first, _, _ := bytes.Cut(data, []byte("\n")); bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		delim = ';'
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = delim
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	head, err := r.Read()
	if err != nil {
		return nil, invalid("file", "File CSV kosong atau tidak terbaca")
	}
	col := map[string]int{}
	for i, h := range head {
		if f, ok := headerAliases[normKey(h)]; ok {
			if _, dup := col[f]; !dup {
				col[f] = i
			}
		}
	}
	if _, ok := col["category"]; !ok {
		return nil, invalid("file", "Kolom \"Kategori\" tidak ditemukan. Unduh template untuk melihat format yang benar")
	}

	res := &ImportResult{DryRun: dryRun, CategoriesCreated: []string{}, EmployeesCreated: []string{}, Errors: []RowError{}}
	type rowData struct {
		line int
		rec  []string
	}
	var rows []rowData
	for line := 2; ; line++ {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, invalid("file", fmt.Sprintf("Baris %d tidak terbaca: %v", line, err))
		}
		blank := true
		for _, c := range rec {
			if strings.TrimSpace(c) != "" {
				blank = false
			}
		}
		if !blank {
			rows = append(rows, rowData{line, rec})
		}
	}
	if len(rows) == 0 {
		return nil, invalid("file", "Tidak ada baris data")
	}
	if len(rows) > 5000 {
		return nil, invalid("file", "Maksimal 5000 baris per impor")
	}
	res.Rows = len(rows)

	err = db.Transaction(func(tx *gorm.DB) error {
		cats := map[string]*models.Category{}
		var existing []models.Category
		if err := tx.Find(&existing).Error; err != nil {
			return err
		}
		for i := range existing {
			cats[strings.ToLower(existing[i].Name)] = &existing[i]
			cats[strings.ToLower(existing[i].Code)] = &existing[i]
		}
		emps := map[string]uint{}
		inactive := map[string]bool{}
		var elist []models.Employee
		if err := tx.Select("id", "name", "status").Find(&elist).Error; err != nil {
			return err
		}
		for _, e := range elist {
			emps[strings.ToLower(e.Name)] = e.ID
			inactive[strings.ToLower(e.Name)] = e.Status == models.EmployeeInactive
		}

		for _, row := range rows {
			get := func(f string) string {
				if i, ok := col[f]; ok && i < len(row.rec) {
					return strings.TrimSpace(row.rec[i])
				}
				return ""
			}
			fail := func(msg string) { res.Errors = append(res.Errors, RowError{row.line, msg}) }
			ptr := func(f string) *string { return clean(strp(get(f))) }

			catName := get("category")
			if catName == "" {
				fail("Kategori wajib diisi")
				continue
			}
			cat := cats[strings.ToLower(catName)]
			if cat == nil {
				code, err := uniqueCode(tx, catName, cats)
				if err != nil {
					fail(err.Error())
					continue
				}
				cat = &models.Category{Name: catName, Code: code, SpecFields: guessFields(catName)}
				if err := tx.Create(cat).Error; err != nil {
					return err
				}
				cats[strings.ToLower(cat.Name)] = cat
				cats[strings.ToLower(cat.Code)] = cat
				res.CategoriesCreated = append(res.CategoriesCreated, cat.Name)
			}

			in := AssetInput{
				CategoryID: cat.ID, Brand: ptr("brand"), Model: ptr("model"), SerialNumber: ptr("serial"),
				CPU: ptr("cpu"), RAM: ptr("ram"), Storage: ptr("storage"), OS: ptr("os"), ScreenSize: ptr("screen"),
				Vendor: ptr("vendor"), Location: ptr("location"), Notes: ptr("notes"), Quantity: 1,
			}
			bad := false
			if s := get("year"); s != "" && s != "-" {
				y, err := strconv.Atoi(s)
				if err != nil {
					fail("Tahun pembuatan harus berupa angka, mis. 2023")
					bad = true
				} else {
					in.ManufactureYear = &y
				}
			}
			for _, f := range []struct {
				key string
				dst *models.Date
			}{{"purchase_date", &in.PurchaseDate}, {"warranty", &in.WarrantyUntil}} {
				t, err := ParseFlexDate(get(f.key))
				if err != nil {
					fail(err.Error())
					bad = true
				}
				f.dst.Time = t
			}
			if s := nonDigit.ReplaceAllString(get("price"), ""); s != "" {
				p, _ := strconv.ParseInt(s, 10, 64)
				in.PurchasePrice = &p
			}
			if s := get("qty"); s != "" {
				q, err := strconv.Atoi(nonDigit.ReplaceAllString(s, ""))
				if err != nil || q < 1 {
					fail("Jumlah harus angka minimal 1")
					bad = true
				} else {
					in.Quantity = q
				}
			}

			holderName := get("holder")
			status := ""
			if s := get("status"); s != "" {
				st, ok := statusAliases[normKey(s)]
				if !ok {
					fail(fmt.Sprintf("Status %q tidak dikenal (Tersedia, Dipakai, Diservis, Rusak, Dimusnahkan)", s))
					bad = true
				}
				status = st
			}
			if status == "" {
				status = models.StatusAvailable
				if holderName != "" {
					status = models.StatusInUse
				}
			}
			if status == models.StatusInUse && holderName == "" {
				fail("Status Dipakai membutuhkan kolom Pengguna")
				bad = true
			}
			if holderName != "" && status != models.StatusInUse {
				n := "Pengguna terakhir: " + holderName
				if in.Notes != nil {
					n = *in.Notes + " — " + n
				}
				in.Notes = &n
				holderName = ""
			}
			if holderName != "" && inactive[strings.ToLower(holderName)] {
				fail("Karyawan " + holderName + " sudah nonaktif, tidak bisa menjadi pengguna aset")
				bad = true
			}
			in.Status = models.StatusAvailable
			if errs := in.Validate(false); len(errs) > 0 {
				for _, m := range errs {
					fail(m)
				}
				bad = true
			}
			if bad {
				continue
			}

			var holderID *uint
			if holderName != "" {
				id, ok := emps[strings.ToLower(holderName)]
				if !ok {
					e := models.Employee{Name: holderName}
					if err := tx.Create(&e).Error; err != nil {
						return err
					}
					id = e.ID
					emps[strings.ToLower(holderName)] = id
					res.EmployeesCreated = append(res.EmployeesCreated, holderName)
				}
				holderID = &id
			}

			tag, err := NextTag(tx, cat.Code)
			if err != nil {
				return err
			}
			a := &models.Asset{Tag: tag, Status: status, HolderID: holderID}
			in.apply(a)
			if holderID != nil {
				a.Location = nil
			}
			if err := tx.Omit(clause.Associations).Create(a).Error; err != nil {
				return err
			}
			to := status
			if err := addLog(tx, a.ID, userID, models.LogCreated, nil, &to, "Diimpor dari CSV", nil); err != nil {
				return err
			}
			res.Created++
		}
		if len(res.Errors) > 0 {
			return errors.New("row errors")
		}
		if dryRun {
			return errDryRun
		}
		return nil
	})
	if err != nil && !errors.Is(err, errDryRun) && len(res.Errors) == 0 {
		return nil, err
	}
	if len(res.Errors) > 0 {
		res.Created = 0
		res.CategoriesCreated, res.EmployeesCreated = []string{}, []string{}
	}
	return res, nil
}

// uniqueCode derives a 3-letter tag prefix (Laptop -> LPT) that is not taken yet.
func uniqueCode(tx *gorm.DB, name string, taken map[string]*models.Category) (string, error) {
	var letters []rune
	for _, r := range strings.ToUpper(name) {
		if r >= 'A' && r <= 'Z' {
			letters = append(letters, r)
		}
	}
	vowels := "AEIOU"
	var cons []rune
	for i, r := range letters {
		if i == 0 || !strings.ContainsRune(vowels, r) {
			cons = append(cons, r)
		}
	}
	base := string(cons)
	if len(base) < 3 {
		base = string(letters)
	}
	for len(base) < 2 {
		base += "X"
	}
	if len(base) > 3 {
		base = base[:3]
	}
	free := func(c string) bool {
		if _, ok := taken[strings.ToLower(c)]; ok {
			return false
		}
		var n int64
		tx.Model(&models.Category{}).Where("code = ?", c).Count(&n)
		return n == 0
	}
	if free(base) {
		return base, nil
	}
	for _, a := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		if c := base[:2] + string(a); free(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("tidak bisa membuat kode untuk kategori %q, buat manual di menu Kategori", name)
}

// guessFields enables sensible hardware fields for a category created by name.
func guessFields(name string) []string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "laptop") || strings.Contains(n, "notebook") || n == "pc" || strings.Contains(n, "komputer") || strings.Contains(n, "server"):
		return []string{"cpu", "ram", "storage", "os"}
	case strings.Contains(n, "monitor") || strings.Contains(n, "layar") || strings.Contains(n, "tv") || strings.Contains(n, "proyektor"):
		return []string{"screen_size"}
	case strings.Contains(n, "handphone") || strings.Contains(n, "hp") || strings.Contains(n, "tablet") || strings.Contains(n, "smartphone"):
		return []string{"ram", "storage", "os"}
	case n == "ram":
		return []string{}
	case n == "hdd" || n == "ssd" || strings.Contains(n, "storage"):
		return []string{"storage"}
	}
	return []string{}
}
