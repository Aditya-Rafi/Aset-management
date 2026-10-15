package service

import (
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

type PersonRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type ReportItem struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

// ReportEvent is one physical check in the life of an asset.
type ReportEvent struct {
	InspectionID  uint         `json:"inspection_id"`
	TransactionID uint         `json:"transaction_id"`
	OrderNumber   string       `json:"order_number"`
	Kind          string       `json:"kind"`
	At            time.Time    `json:"at"`
	Employee      *PersonRef   `json:"employee"`
	FromEmployee  *PersonRef   `json:"from_employee"`
	Skipped       bool         `json:"skipped"`
	SkipReason    string       `json:"skip_reason,omitempty"`
	Conclusion    string       `json:"conclusion"`
	BatteryHealth *int         `json:"battery_health"`
	BadItems      []ReportItem `json:"bad_items"`
	Notes         string       `json:"notes"`

	profile string
	results map[string]models.CheckResult
	empID   uint
	fromID  uint
}

type Responsible struct {
	Kind string `json:"kind"` // employee | storage | unknown
	Name string `json:"name,omitempty"`
}

// Finding is a change between two consecutive checks.
type Finding struct {
	Type        string      `json:"type"` // new_damage | fixed | existing | battery
	Code        string      `json:"code"`
	Label       string      `json:"label"`
	Note        string      `json:"note"`
	FromEvent   uint        `json:"from_inspection_id"`
	ToEvent     uint        `json:"to_inspection_id"`
	Responsible Responsible `json:"responsible"`
	Explanation string      `json:"explanation"`
}

type ConditionReport struct {
	Events     []ReportEvent `json:"events"`
	Findings   []Finding     `json:"findings"`
	Disclaimer string        `json:"disclaimer"`
}

const disclaimer = "Kesimpulan ini hanya membandingkan hasil pemeriksaan fisik yang tercatat pada tiap serah terima. " +
	"Ia menunjukkan pada rentang kepemilikan siapa sebuah komponen berubah dari Baik menjadi Tidak Baik, bukan menetapkan penyebab atau kesalahan."

var kindLabel = map[string]string{models.TxOut: "serah terima", models.TxIn: "pengembalian", models.TxTransfer: "pindah tangan"}

func shortDate(t time.Time) string { return t.Format("02-01-2006") }

// BuildConditionReport compares every consecutive pair of physical checks of an asset
// and attributes each change to whoever had the asset between the two checks.
func BuildConditionReport(db *gorm.DB, assetID uint) (*ConditionReport, error) {
	var insps []models.Inspection
	if err := db.Preload("Transaction").Where("asset_id = ?", assetID).
		Order("transaction_id ASC, id ASC").Find(&insps).Error; err != nil {
		return nil, err
	}
	names := map[uint]string{}
	{
		var ids []uint
		for _, i := range insps {
			ids = append(ids, i.EmployeeID)
			if i.FromEmployeeID != nil {
				ids = append(ids, *i.FromEmployeeID)
			}
		}
		if len(ids) > 0 {
			var emps []models.Employee
			if err := db.Select("id", "name").Where("id IN ?", ids).Find(&emps).Error; err != nil {
				return nil, err
			}
			for _, e := range emps {
				names[e.ID] = e.Name
			}
		}
	}
	ref := func(id uint) *PersonRef { return &PersonRef{ID: id, Name: names[id]} }

	rep := &ConditionReport{Events: []ReportEvent{}, Findings: []Finding{}, Disclaimer: disclaimer}
	for _, i := range insps {
		ev := ReportEvent{
			InspectionID: i.ID, TransactionID: i.TransactionID, Kind: i.Kind, At: i.CreatedAt,
			Employee: ref(i.EmployeeID), Skipped: i.Skipped, Conclusion: i.Data.Conclusion,
			BatteryHealth: i.Data.BatteryHealth, BadItems: []ReportItem{}, Notes: i.Data.Notes,
			profile: i.Data.Profile, results: map[string]models.CheckResult{}, empID: i.EmployeeID,
		}
		if i.Transaction != nil {
			ev.OrderNumber, ev.At = i.Transaction.OrderNumber, i.Transaction.CreatedAt
		}
		if i.FromEmployeeID != nil {
			ev.FromEmployee, ev.fromID = ref(*i.FromEmployeeID), *i.FromEmployeeID
		}
		if i.SkipReason != nil {
			ev.SkipReason = *i.SkipReason
		}
		for _, c := range i.Data.Items {
			ev.results[c.Code] = c
			if c.Result == models.CheckBad {
				ev.BadItems = append(ev.BadItems, ReportItem{c.Code, models.ItemLabel(i.Data.Profile, c.Code), c.Note})
			}
		}
		rep.Events = append(rep.Events, ev)
	}

	// who had the asset between two adjacent checks
	custody := func(prev, cur ReportEvent) Responsible {
		switch prev.Kind {
		case models.TxOut, models.TxTransfer:
			leaving := cur.empID
			if cur.Kind == models.TxTransfer {
				leaving = cur.fromID
			}
			if (cur.Kind == models.TxIn || cur.Kind == models.TxTransfer) && leaving == prev.empID {
				return Responsible{Kind: "employee", Name: names[prev.empID]}
			}
		case models.TxIn:
			if cur.Kind == models.TxOut {
				return Responsible{Kind: "storage", Name: names[prev.empID]}
			}
		}
		return Responsible{Kind: "unknown"}
	}

	for idx := 0; idx < len(rep.Events); idx++ {
		cur := rep.Events[idx]
		if idx == 0 {
			if !cur.Skipped {
				for _, b := range cur.BadItems {
					rep.Findings = append(rep.Findings, Finding{
						Type: "existing", Code: b.Code, Label: b.Label, Note: b.Note, ToEvent: cur.InspectionID,
						Responsible: Responsible{Kind: "unknown"},
						Explanation: fmt.Sprintf("Sudah Tidak Baik pada pemeriksaan pertama yang tercatat (%s, %s), sebelum aset memiliki riwayat di sistem.", kindLabel[cur.Kind], shortDate(cur.At)),
					})
				}
			}
			continue
		}
		prev := rep.Events[idx-1]
		if prev.Skipped || cur.Skipped || prev.profile != cur.profile {
			continue // nothing to compare; the skipped check shows up in the timeline itself
		}
		who := custody(prev, cur)
		span := fmt.Sprintf("antara %s (%s) dan %s (%s)", kindLabel[prev.Kind], shortDate(prev.At), kindLabel[cur.Kind], shortDate(cur.At))
		whoText := func() string {
			switch who.Kind {
			case "employee":
				return "Dalam rentang itu aset dipegang " + who.Name + "."
			case "storage":
				return "Dalam rentang itu aset berada di IT / penyimpanan setelah dikembalikan " + who.Name + " dan sebelum diserahkan kepada " + names[cur.empID] + "."
			}
			return "Pemegang pada rentang itu tidak dapat dipastikan dari data."
		}()

		tpl := models.InspectionTemplates[cur.profile]
		for _, sec := range tpl.Sections {
			for _, it := range sec.Items {
				p, c := prev.results[it.Code], cur.results[it.Code]
				switch {
				case p.Result == models.CheckOK && c.Result == models.CheckBad:
					rep.Findings = append(rep.Findings, Finding{
						Type: "new_damage", Code: it.Code, Label: it.Label, Note: c.Note,
						FromEvent: prev.InspectionID, ToEvent: cur.InspectionID, Responsible: who,
						Explanation: fmt.Sprintf("Baik pada %s, Tidak Baik pada %s. %s", kindLabel[prev.Kind], kindLabel[cur.Kind], whoText),
					})
				case p.Result == models.CheckBad && c.Result == models.CheckBad:
					earlier := Responsible{Kind: "unknown"}
					expl := fmt.Sprintf("Sudah Tidak Baik pada %s (%s) dan masih Tidak Baik pada %s.", kindLabel[prev.Kind], shortDate(prev.At), kindLabel[cur.Kind])
					if prev.Kind == models.TxIn {
						earlier = Responsible{Kind: "employee", Name: names[prev.empID]}
						expl = fmt.Sprintf("Sudah Tidak Baik saat dikembalikan %s (%s); bukan akibat pemegang berikutnya.", names[prev.empID], shortDate(prev.At))
					}
					rep.Findings = append(rep.Findings, Finding{
						Type: "existing", Code: it.Code, Label: it.Label, Note: c.Note,
						FromEvent: prev.InspectionID, ToEvent: cur.InspectionID, Responsible: earlier, Explanation: expl,
					})
				case p.Result == models.CheckBad && c.Result == models.CheckOK:
					rep.Findings = append(rep.Findings, Finding{
						Type: "fixed", Code: it.Code, Label: it.Label, Note: c.Note,
						FromEvent: prev.InspectionID, ToEvent: cur.InspectionID, Responsible: Responsible{Kind: "unknown"},
						Explanation: fmt.Sprintf("Tidak Baik pada %s, sudah Baik pada %s (%s).", kindLabel[prev.Kind], kindLabel[cur.Kind], span),
					})
				}
			}
		}
		if prev.BatteryHealth != nil && cur.BatteryHealth != nil && *prev.BatteryHealth-*cur.BatteryHealth >= 10 {
			rep.Findings = append(rep.Findings, Finding{
				Type: "battery", Code: "hw_battery", Label: "Baterai - kesehatan (health)",
				FromEvent: prev.InspectionID, ToEvent: cur.InspectionID, Responsible: who,
				Explanation: fmt.Sprintf("Kesehatan baterai turun dari %d%% menjadi %d%% %s. %s", *prev.BatteryHealth, *cur.BatteryHealth, span, whoText),
			})
		}
	}
	sort.SliceStable(rep.Findings, func(i, j int) bool { return rep.Findings[i].ToEvent > rep.Findings[j].ToEvent })
	return rep, nil
}
