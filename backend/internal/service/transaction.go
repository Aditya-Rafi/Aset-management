package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

type TxItem struct {
	AssetID      uint             `json:"asset_id"`
	ResultStatus string           `json:"result_status"` // returns only: available (default), repair or broken
	Note         string           `json:"note"`
	Inspection   *InspectionInput `json:"inspection"` // physical check; required for categories that have a checklist
}

type CreateTransactionInput struct {
	Type       string `json:"type"`
	EmployeeID uint   `json:"employee_id"` // out: receiver, in: returner, transfer: new holder
	// FromEmployeeID is the current holder on a transfer.
	FromEmployeeID uint     `json:"from_employee_id"`
	Note           *string  `json:"note"`
	Location       *string  `json:"location"` // returns only: where the assets are stored afterwards
	Items          []TxItem `json:"items"`
}

// CreateTransaction hands assets to an employee (out), takes them back (in) or
// passes them from one employee to another (transfer).
// Assets are row-locked so two simultaneous requests cannot both hand out the
// same asset, and every change is logged.
func CreateTransaction(db *gorm.DB, in CreateTransactionInput, userID *uint) (*models.Transaction, error) {
	fields := map[string]string{}
	if in.Type != models.TxOut && in.Type != models.TxIn && in.Type != models.TxTransfer {
		fields["type"] = "Jenis harus serah terima, pengembalian, atau pindah tangan"
	}
	if in.EmployeeID == 0 {
		fields["employee_id"] = "Karyawan wajib dipilih"
	}
	if in.Type == models.TxTransfer {
		if in.FromEmployeeID == 0 {
			fields["from_employee_id"] = "Karyawan yang menyerahkan wajib dipilih"
		} else if in.FromEmployeeID == in.EmployeeID {
			fields["employee_id"] = "Penerima harus berbeda dari yang menyerahkan"
		}
	}
	in.Note, in.Location = clean(in.Note), clean(in.Location)
	if in.Note != nil && len([]rune(*in.Note)) > 255 {
		fields["note"] = "Catatan maksimal 255 karakter"
	}
	if in.Location != nil && len([]rune(*in.Location)) > 255 {
		fields["location"] = "Lokasi terlalu panjang"
	}
	if len(in.Items) == 0 {
		fields["items"] = "Pilih minimal satu aset"
	}
	seen := map[uint]bool{}
	for i := range in.Items {
		it := &in.Items[i]
		if seen[it.AssetID] {
			fields["items"] = "Ada aset yang sama, silakan hapus salah satunya"
		}
		seen[it.AssetID] = true
		if in.Type == models.TxIn && it.ResultStatus != "" {
			switch it.ResultStatus {
			case models.StatusAvailable, models.StatusRepair, models.StatusBroken:
			default:
				fields["items"] = "Status setelah pengembalian harus Tersedia, Diservis, atau Rusak"
			}
		}
		if len([]rune(it.Note)) > 255 {
			fields["items"] = "Catatan per aset maksimal 255 karakter"
		}
	}
	if len(fields) > 0 {
		return nil, &ValidationError{fields}
	}

	var tx *models.Transaction
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		tx, err = createTxOnce(db, in, userID)
		if err == nil || !IsDuplicate(err) {
			break
		}
	}
	return tx, err
}

func createTxOnce(db *gorm.DB, in CreateTransactionInput, userID *uint) (*models.Transaction, error) {
	var result *models.Transaction
	err := db.Transaction(func(tx *gorm.DB) error {
		var emp models.Employee
		if err := tx.First(&emp, in.EmployeeID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return invalid("employee_id", "Karyawan tidak ditemukan")
			}
			return err
		}
		// Only people still at the company can be handed assets.
		if emp.Status == models.EmployeeInactive && in.Type != models.TxIn {
			return invalid("employee_id", "Karyawan sudah nonaktif, tidak bisa menerima aset")
		}
		var from models.Employee
		if in.Type == models.TxTransfer {
			if err := tx.First(&from, in.FromEmployeeID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return invalid("from_employee_id", "Karyawan yang menyerahkan tidak ditemukan")
				}
				return err
			}
		}

		ids := make([]uint, len(in.Items))
		for i, it := range in.Items {
			ids[i] = it.AssetID
		}
		var assets []models.Asset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Order("id").Find(&assets).Error; err != nil {
			return err
		}
		var cats []models.Category
		if err := tx.Find(&cats).Error; err != nil {
			return err
		}
		profileOf := map[uint]string{}
		for _, c := range cats {
			profileOf[c.ID] = c.Inspection
		}
		if len(assets) != len(ids) {
			return invalid("items", "Ada aset yang tidak ditemukan")
		}
		byID := map[uint]models.Asset{}
		for _, a := range assets {
			byID[a.ID] = a
		}

		var problems []string
		for _, a := range assets {
			switch {
			case in.Type == models.TxOut && a.Status != models.StatusAvailable:
				problems = append(problems, fmt.Sprintf("%s berstatus %s, hanya aset Tersedia yang bisa diserahkan", a.Tag, StatusLabel(a.Status)))
			case in.Type == models.TxIn && a.Status != models.StatusInUse:
				problems = append(problems, fmt.Sprintf("%s tidak sedang dipakai", a.Tag))
			case in.Type == models.TxIn && (a.HolderID == nil || *a.HolderID != in.EmployeeID):
				problems = append(problems, fmt.Sprintf("%s tidak dipegang oleh %s", a.Tag, emp.Name))
			case in.Type == models.TxTransfer && a.Status != models.StatusInUse:
				problems = append(problems, fmt.Sprintf("%s tidak sedang dipakai, hanya aset yang dipakai yang bisa dipindahtangankan", a.Tag))
			case in.Type == models.TxTransfer && (a.HolderID == nil || *a.HolderID != in.FromEmployeeID):
				problems = append(problems, fmt.Sprintf("%s tidak dipegang oleh %s", a.Tag, from.Name))
			}
		}
		if len(problems) > 0 {
			return invalid("items", strings.Join(problems, "; "))
		}

		// the physical check is part of the document for every category that has a checklist
		type checked struct {
			data    models.InspectionData
			skipped bool
			reason  *string
		}
		checks := map[uint]checked{}
		var missing []string
		for _, it := range in.Items {
			a := byID[it.AssetID]
			profile := profileOf[a.CategoryID]
			if profile == "" || profile == models.ProfileNone {
				continue
			}
			d, sk, rs, prob := cleanInspection(profile, it.Inspection)
			if prob != "" {
				missing = append(missing, problemList(a.Tag, prob))
				continue
			}
			checks[a.ID] = checked{d, sk, rs}
		}
		if len(missing) > 0 {
			return invalid("inspections", strings.Join(missing, "; "))
		}

		prefix := map[string]string{models.TxOut: "OUT", models.TxIn: "IN", models.TxTransfer: "TRF"}[in.Type]
		now := time.Now()
		order, err := NextDocNumber(tx, prefix, now)
		if err != nil {
			return err
		}
		t := &models.Transaction{OrderNumber: order, Type: in.Type, EmployeeID: in.EmployeeID, Note: in.Note, CreatedByID: userID}
		if in.Type == models.TxTransfer {
			t.FromEmployeeID = &in.FromEmployeeID
		}
		if err := tx.Omit(clause.Associations).Create(t).Error; err != nil {
			return err
		}

		for _, it := range in.Items {
			a := byID[it.AssetID]
			d := models.TransactionDetail{
				TransactionID: t.ID, AssetID: a.ID, PrevStatus: a.Status,
				PrevHolderID: a.HolderID, PrevLocation: a.Location, Note: clean(&it.Note),
			}
			var newStatus string
			var newHolder *uint
			var newLoc *string
			var action, msg string
			switch in.Type {
			case models.TxOut:
				newStatus, newHolder, newLoc = models.StatusInUse, &in.EmployeeID, nil
				action, msg = models.LogAssigned, fmt.Sprintf("Diserahkan kepada %s (%s)", emp.Name, order)
			case models.TxTransfer:
				newStatus, newHolder, newLoc = models.StatusInUse, &in.EmployeeID, nil
				action, msg = models.LogTransferred, fmt.Sprintf("Dipindahtangankan dari %s ke %s (%s)", from.Name, emp.Name, order)
			default:
				result := it.ResultStatus
				if result == "" {
					result = models.StatusAvailable
					if c, ok := checks[a.ID]; ok && !c.skipped {
						result = statusFromConclusion(c.data.Conclusion) // what the physical check concluded
					}
				}
				newStatus, newHolder, newLoc = result, nil, in.Location
				action, msg = models.LogReturned, fmt.Sprintf("Dikembalikan oleh %s (%s)", emp.Name, order)
			}
			d.NewStatus, d.NewLocation = newStatus, newLoc
			if d.Note != nil {
				msg += ": " + *d.Note
			}
			if err := tx.Omit(clause.Associations).Create(&d).Error; err != nil {
				return err
			}
			if c, ok := checks[a.ID]; ok {
				insp := models.Inspection{
					TransactionID: t.ID, AssetID: a.ID, Kind: in.Type, EmployeeID: in.EmployeeID,
					Skipped: c.skipped, SkipReason: c.reason, Data: c.data,
				}
				if in.Type == models.TxTransfer {
					insp.FromEmployeeID = &in.FromEmployeeID
				}
				if err := tx.Omit(clause.Associations).Create(&insp).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&models.Asset{}).Where("id = ?", a.ID).
				Select("status", "holder_id", "location").
				Updates(map[string]interface{}{"status": newStatus, "holder_id": newHolder, "location": newLoc}).Error; err != nil {
				return err
			}
			from, to, tid := a.Status, newStatus, t.ID
			if err := addLog(tx, a.ID, userID, action, &from, &to, msg, &tid); err != nil {
				return err
			}
		}
		result = t
		return nil
	})
	return result, err
}

// DeleteTransaction undoes a document and restores every asset exactly as it
// was before. It refuses when the asset has changed again since, because
// rolling back an older movement would corrupt its current state.
func DeleteTransaction(db *gorm.DB, id uint, userID *uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var t models.Transaction
		if err := tx.First(&t, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		var details []models.TransactionDetail
		if err := tx.Where("transaction_id = ?", id).Find(&details).Error; err != nil {
			return err
		}
		ids := make([]uint, 0, len(details))
		for _, d := range details {
			ids = append(ids, d.AssetID)
		}
		byID := map[uint]models.Asset{}
		if len(ids) > 0 {
			var assets []models.Asset
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Find(&assets).Error; err != nil {
				return err
			}
			for _, a := range assets {
				byID[a.ID] = a
			}
		}

		for _, d := range details {
			a := byID[d.AssetID]
			var later int64
			if err := tx.Model(&models.TransactionDetail{}).
				Where("asset_id = ? AND transaction_id > ?", a.ID, id).Count(&later).Error; err != nil {
				return err
			}
			stateChanged := a.Status != d.NewStatus
			if t.Type == models.TxOut || t.Type == models.TxTransfer {
				stateChanged = stateChanged || a.HolderID == nil || *a.HolderID != t.EmployeeID
			} else {
				stateChanged = stateChanged || a.HolderID != nil
			}
			if later > 0 || stateChanged {
				return &ConflictError{fmt.Sprintf(
					"Dokumen tidak dapat dibatalkan karena aset %s sudah berubah sejak dokumen ini dibuat", a.Tag)}
			}
		}

		for _, d := range details {
			if err := tx.Model(&models.Asset{}).Where("id = ?", d.AssetID).
				Select("status", "holder_id", "location").
				Updates(map[string]interface{}{"status": d.PrevStatus, "holder_id": d.PrevHolderID, "location": d.PrevLocation}).Error; err != nil {
				return err
			}
			from, to := d.NewStatus, d.PrevStatus
			if err := addLog(tx, d.AssetID, userID, models.LogReverted, &from, &to,
				fmt.Sprintf("Dokumen %s dibatalkan", t.OrderNumber), nil); err != nil {
				return err
			}
		}
		return tx.Delete(&models.Transaction{}, id).Error
	})
}

// StatusLabel is the Indonesian label of a status.
func StatusLabel(s string) string {
	switch s {
	case models.StatusAvailable:
		return "Tersedia"
	case models.StatusInUse:
		return "Dipakai"
	case models.StatusRepair:
		return "Diservis"
	case models.StatusBroken:
		return "Rusak"
	case models.StatusDisposed:
		return "Dimusnahkan"
	}
	return s
}
