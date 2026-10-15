package service

import (
	"fmt"
	"strings"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

// InspectionInput is what the client sends for one asset on a document.
type InspectionInput struct {
	Skipped    bool   `json:"skipped"`
	SkipReason string `json:"skip_reason"`
	models.InspectionData
}

// cleanInspection validates an inspection against the checklist of the asset's
// category and returns it complete and normalised (every template item present,
// in template order). The returned string is a human readable problem, or "".
func cleanInspection(profile string, in *InspectionInput) (data models.InspectionData, skipped bool, reason *string, problem string) {
	if in == nil {
		return data, false, nil, "pemeriksaan fisik belum diisi"
	}
	if in.Skipped {
		r := strings.TrimSpace(in.SkipReason)
		if len([]rune(r)) < 3 {
			return data, true, nil, "alasan melewati pemeriksaan wajib diisi"
		}
		if len([]rune(r)) > 255 {
			return data, true, nil, "alasan terlalu panjang"
		}
		return models.InspectionData{Profile: profile}, true, &r, ""
	}

	tpl := models.InspectionTemplates[profile]
	given := map[string]models.CheckResult{}
	for _, it := range in.Items {
		if _, dup := given[it.Code]; dup {
			return data, false, nil, "butir pemeriksaan ganda: " + it.Code
		}
		given[it.Code] = it
	}
	answered := 0
	out := models.InspectionData{Profile: profile}
	known := 0
	for _, sec := range tpl.Sections {
		for _, ti := range sec.Items {
			known++
			g := given[ti.Code]
			switch g.Result {
			case "", models.CheckOK, models.CheckBad:
			default:
				return data, false, nil, "hasil pemeriksaan harus Baik atau Tidak Baik"
			}
			note := strings.TrimSpace(g.Note)
			if len([]rune(note)) > 255 {
				return data, false, nil, "keterangan pemeriksaan terlalu panjang"
			}
			if g.Result != "" {
				answered++
			}
			if g.Result == models.CheckBad && note == "" {
				return data, false, nil, "butir Tidak Baik wajib diberi keterangan: " + ti.Label
			}
			out.Items = append(out.Items, models.CheckResult{Code: ti.Code, Result: g.Result, Note: note})
		}
	}
	for code := range given {
		if models.ItemLabel(profile, code) == code {
			return data, false, nil, "butir pemeriksaan tidak dikenal: " + code
		}
	}
	if answered == 0 {
		return data, false, nil, "isi minimal satu butir pemeriksaan (atau lewati dengan alasan)"
	}

	accGiven := map[string]models.AccessoryResult{}
	for _, a := range in.Accessories {
		accGiven[a.Code] = a
	}
	for _, ta := range tpl.Accessories {
		g := accGiven[ta.Code]
		cond, note := strings.TrimSpace(g.Condition), strings.TrimSpace(g.Note)
		if len([]rune(cond)) > 100 || len([]rune(note)) > 255 {
			return data, false, nil, "keterangan kelengkapan terlalu panjang"
		}
		out.Accessories = append(out.Accessories, models.AccessoryResult{Code: ta.Code, Present: g.Present, Condition: cond, Note: note})
	}
	for code := range accGiven {
		found := false
		for _, ta := range tpl.Accessories {
			found = found || ta.Code == code
		}
		if !found {
			return data, false, nil, "kelengkapan tidak dikenal: " + code
		}
	}

	if b := in.BatteryHealth; b != nil {
		if *b < 0 || *b > 100 {
			return data, false, nil, "kesehatan baterai harus 0-100%"
		}
		out.BatteryHealth = b
	}
	switch in.Conclusion {
	case models.ConclusionFit, models.ConclusionRepair, models.ConclusionUnfit:
		out.Conclusion = in.Conclusion
	default:
		return data, false, nil, "kesimpulan pemeriksaan wajib dipilih"
	}
	out.Notes = strings.TrimSpace(in.Notes)
	if len([]rune(out.Notes)) > 2000 {
		return data, false, nil, "catatan terlalu panjang"
	}
	_ = known
	return out, false, nil, ""
}

// statusFromConclusion is the default state of a returned asset after its check.
func statusFromConclusion(c string) string {
	switch c {
	case models.ConclusionRepair:
		return models.StatusRepair
	case models.ConclusionUnfit:
		return models.StatusBroken
	}
	return models.StatusAvailable
}

func problemList(tag string, msg string) string { return fmt.Sprintf("%s: %s", tag, msg) }
