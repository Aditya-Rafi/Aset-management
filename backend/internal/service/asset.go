package service

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

// AssetInput is the editable part of an asset. Status and holder are changed
// only through ChangeStatus and handover documents, so they leave an audit trail.
type AssetInput struct {
	CategoryID      uint        `json:"category_id"`
	Brand           *string     `json:"brand"`
	Model           *string     `json:"model"`
	SerialNumber    *string     `json:"serial_number"`
	CPU             *string     `json:"cpu"`
	RAM             *string     `json:"ram"`
	Storage         *string     `json:"storage"`
	OS              *string     `json:"os"`
	ScreenSize      *string     `json:"screen_size"`
	ManufactureYear *int        `json:"manufacture_year"`
	PurchaseDate    models.Date `json:"purchase_date"`
	PurchasePrice   *int64      `json:"purchase_price"`
	WarrantyUntil   models.Date `json:"warranty_until"`
	Vendor          *string     `json:"vendor"`
	Location        *string     `json:"location"`
	Quantity        int         `json:"quantity"`
	Notes           *string     `json:"notes"`
	Status          string      `json:"status"` // create only
}

func (in *AssetInput) Normalize() {
	for _, p := range []**string{&in.Brand, &in.Model, &in.SerialNumber, &in.CPU, &in.RAM, &in.Storage,
		&in.OS, &in.ScreenSize, &in.Vendor, &in.Location, &in.Notes} {
		*p = clean(*p)
	}
	if in.Quantity == 0 {
		in.Quantity = 1
	}
	in.Status = strings.TrimSpace(in.Status)
}

func (in *AssetInput) Validate(forCreate bool) map[string]string {
	e := map[string]string{}
	if in.CategoryID == 0 {
		e["category_id"] = "Kategori wajib dipilih"
	}
	max := func(field, label string, p *string, n int) {
		if p != nil && len([]rune(*p)) > n {
			e[field] = label + " terlalu panjang"
		}
	}
	max("brand", "Merk", in.Brand, 255)
	max("model", "Tipe", in.Model, 255)
	max("serial_number", "Serial number", in.SerialNumber, 255)
	max("cpu", "CPU", in.CPU, 255)
	max("ram", "RAM", in.RAM, 100)
	max("storage", "Storage", in.Storage, 255)
	max("os", "OS", in.OS, 100)
	max("screen_size", "Ukuran layar", in.ScreenSize, 50)
	max("vendor", "Vendor", in.Vendor, 255)
	max("location", "Lokasi", in.Location, 255)
	if in.ManufactureYear != nil && (*in.ManufactureYear < 1980 || *in.ManufactureYear > time.Now().Year()+1) {
		e["manufacture_year"] = "Tahun pembuatan tidak valid"
	}
	if in.PurchasePrice != nil && *in.PurchasePrice < 0 {
		e["purchase_price"] = "Harga beli tidak boleh negatif"
	}
	if in.Quantity < 1 || in.Quantity > 100000 {
		e["quantity"] = "Jumlah minimal 1"
	}
	if forCreate {
		if in.Status == "" {
			in.Status = models.StatusAvailable
		}
		if !models.ValidStatus(in.Status) {
			e["status"] = "Status tidak dikenal"
		} else if in.Status == models.StatusInUse {
			e["status"] = "Untuk status Dipakai gunakan Serah Terima"
		}
	}
	return e
}

func (in *AssetInput) apply(a *models.Asset) {
	a.CategoryID = in.CategoryID
	a.Brand, a.Model, a.SerialNumber = in.Brand, in.Model, in.SerialNumber
	a.CPU, a.RAM, a.Storage, a.OS, a.ScreenSize = in.CPU, in.RAM, in.Storage, in.OS, in.ScreenSize
	a.ManufactureYear, a.PurchaseDate, a.PurchasePrice = in.ManufactureYear, in.PurchaseDate, in.PurchasePrice
	a.WarrantyUntil, a.Vendor, a.Location, a.Quantity, a.Notes = in.WarrantyUntil, in.Vendor, in.Location, in.Quantity, in.Notes
}

func categoryCode(tx *gorm.DB, id uint) (string, error) {
	var c models.Category
	if err := tx.Select("id", "code").First(&c, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", invalid("category_id", "Kategori tidak ditemukan")
		}
		return "", err
	}
	return c.Code, nil
}

// CreateAsset stores a new asset with the next tag of its category.
func CreateAsset(db *gorm.DB, in AssetInput, userID *uint) (*models.Asset, error) {
	in.Normalize()
	if errs := in.Validate(true); len(errs) > 0 {
		return nil, &ValidationError{errs}
	}
	var asset *models.Asset
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		asset, err = createAssetOnce(db, in, userID)
		if err == nil || !IsDuplicate(err) {
			break
		}
	}
	return asset, err
}

func createAssetOnce(db *gorm.DB, in AssetInput, userID *uint) (*models.Asset, error) {
	var out *models.Asset
	err := db.Transaction(func(tx *gorm.DB) error {
		code, err := categoryCode(tx, in.CategoryID)
		if err != nil {
			return err
		}
		tag, err := NextTag(tx, code)
		if err != nil {
			return err
		}
		a := &models.Asset{Tag: tag, Status: in.Status}
		in.apply(a)
		if err := tx.Omit(clause.Associations).Create(a).Error; err != nil {
			return err
		}
		to := a.Status
		if err := addLog(tx, a.ID, userID, models.LogCreated, nil, &to, "Aset didaftarkan", nil); err != nil {
			return err
		}
		out = a
		return nil
	})
	return out, err
}

// UpdateAsset edits descriptive fields and records which ones changed.
func UpdateAsset(db *gorm.DB, id uint, in AssetInput, userID *uint) (*models.Asset, error) {
	in.Normalize()
	if errs := in.Validate(false); len(errs) > 0 {
		return nil, &ValidationError{errs}
	}
	var out *models.Asset
	err := db.Transaction(func(tx *gorm.DB) error {
		var a models.Asset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if _, err := categoryCode(tx, in.CategoryID); err != nil {
			return err
		}
		before := a
		in.apply(&a)
		if err := tx.Omit(clause.Associations).Save(&a).Error; err != nil {
			return err
		}
		if changed := changedFields(before, a); len(changed) > 0 {
			if err := addLog(tx, a.ID, userID, models.LogUpdated, nil, nil, "Diubah: "+strings.Join(changed, ", "), nil); err != nil {
				return err
			}
		}
		out = &a
		return nil
	})
	return out, err
}

// dstr compares calendar dates independent of time zone (MySQL returns them in the local zone).
func dstr(d models.Date) string {
	if d.IsZero() {
		return ""
	}
	return d.Format("2006-01-02")
}

func changedFields(a, b models.Asset) []string {
	var c []string
	add := func(name string, diff bool) {
		if diff {
			c = append(c, name)
		}
	}
	ptr := func(x, y *string) bool { return val(x) != val(y) }
	add("kategori", a.CategoryID != b.CategoryID)
	add("merk", ptr(a.Brand, b.Brand))
	add("tipe", ptr(a.Model, b.Model))
	add("serial number", ptr(a.SerialNumber, b.SerialNumber))
	add("CPU", ptr(a.CPU, b.CPU))
	add("RAM", ptr(a.RAM, b.RAM))
	add("storage", ptr(a.Storage, b.Storage))
	add("OS", ptr(a.OS, b.OS))
	add("ukuran layar", ptr(a.ScreenSize, b.ScreenSize))
	add("tahun pembuatan", (a.ManufactureYear == nil) != (b.ManufactureYear == nil) ||
		(a.ManufactureYear != nil && *a.ManufactureYear != *b.ManufactureYear))
	add("tanggal pembelian", dstr(a.PurchaseDate) != dstr(b.PurchaseDate))
	add("harga beli", (a.PurchasePrice == nil) != (b.PurchasePrice == nil) ||
		(a.PurchasePrice != nil && *a.PurchasePrice != *b.PurchasePrice))
	add("garansi", dstr(a.WarrantyUntil) != dstr(b.WarrantyUntil))
	add("vendor", ptr(a.Vendor, b.Vendor))
	add("lokasi", ptr(a.Location, b.Location))
	add("jumlah", a.Quantity != b.Quantity)
	add("keterangan", ptr(a.Notes, b.Notes))
	return c
}

// ChangeStatus moves an asset between the states that do not involve a person.
func ChangeStatus(db *gorm.DB, id uint, to, note string, userID *uint) (*models.Asset, error) {
	note = strings.TrimSpace(note)
	if !models.ValidStatus(to) {
		return nil, invalid("status", "Status tidak dikenal")
	}
	if to == models.StatusInUse {
		return nil, invalid("status", "Untuk status Dipakai gunakan Serah Terima")
	}
	var out *models.Asset
	err := db.Transaction(func(tx *gorm.DB) error {
		var a models.Asset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if a.Status == models.StatusInUse {
			return &ConflictError{"Aset sedang dipakai. Kembalikan dulu lewat Pengembalian, lalu ubah statusnya"}
		}
		if a.Status == to {
			return invalid("status", "Aset sudah berstatus tersebut")
		}
		from := a.Status
		a.Status = to
		if err := tx.Model(&models.Asset{}).Where("id = ?", id).Update("status", to).Error; err != nil {
			return err
		}
		if err := addLog(tx, id, userID, models.LogStatus, &from, &to, note, nil); err != nil {
			return err
		}
		out = &a
		return nil
	})
	return out, err
}

// AddNote appends a free-text entry (e.g. a service record) to the asset history.
func AddNote(db *gorm.DB, id uint, note string, userID *uint) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return invalid("message", "Catatan wajib diisi")
	}
	if len([]rune(note)) > 2000 {
		return invalid("message", "Catatan terlalu panjang")
	}
	var n int64
	if err := db.Model(&models.Asset{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return addLog(db, id, userID, models.LogNote, nil, nil, note, nil)
}

// LogPhoto records a photo being added or removed.
func LogPhoto(db *gorm.DB, id uint, userID *uint, msg string) error {
	return addLog(db, id, userID, models.LogPhoto, nil, nil, msg, nil)
}
