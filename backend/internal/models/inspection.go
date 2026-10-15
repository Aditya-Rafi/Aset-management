package models

import "time"

// Inspection is the physical check IT Support does when an asset changes hands.
// Comparing consecutive inspections shows when a component went from good to
// bad, and therefore who had the asset at that time.
type Inspection struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	TransactionID  uint           `gorm:"not null;index" json:"transaction_id"`
	AssetID        uint           `gorm:"not null;index" json:"asset_id"`
	Kind           string         `gorm:"size:8;not null" json:"kind"` // out | in | transfer
	EmployeeID     uint           `gorm:"not null" json:"employee_id"` // out/transfer: receiver, in: returner
	FromEmployeeID *uint          `json:"from_employee_id"`            // transfer: previous holder
	Skipped        bool           `gorm:"not null;default:false" json:"skipped"`
	SkipReason     *string        `gorm:"size:255" json:"skip_reason"`
	Data           InspectionData `gorm:"serializer:json;type:text" json:"data"`
	Transaction    *Transaction   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"transaction,omitempty"`
	Asset          *Asset         `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"asset,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// Check results.
const (
	CheckOK  = "ok"  // Baik
	CheckBad = "bad" // Tidak Baik
)

// Overall conclusions.
const (
	ConclusionFit    = "layak"
	ConclusionRepair = "perlu_perbaikan"
	ConclusionUnfit  = "tidak_layak"
)

type CheckResult struct {
	Code   string `json:"code"`
	Result string `json:"result"` // ok | bad | "" (not checked)
	Note   string `json:"note"`
}

type AccessoryResult struct {
	Code      string `json:"code"`
	Present   *bool  `json:"present"` // Ada / Tidak
	Condition string `json:"condition"`
	Note      string `json:"note"`
}

type InspectionData struct {
	Profile       string            `json:"profile"`
	Items         []CheckResult     `json:"items"`
	Accessories   []AccessoryResult `json:"accessories"`
	BatteryHealth *int              `json:"battery_health"`
	Conclusion    string            `json:"conclusion"`
	Notes         string            `json:"notes"`
}

// ---- checklist templates (the wording follows the company BAST form) ----

const (
	ProfileNone    = "none"
	ProfileGeneral = "general"
	ProfileLaptop  = "laptop"
)

type CheckItem struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	Battery bool   `json:"battery,omitempty"` // this row also records the battery health %
}

type CheckSection struct {
	Key   string      `json:"key"`
	Title string      `json:"title"`
	Items []CheckItem `json:"items"`
}

type AccessoryItem struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type InspectionTemplate struct {
	Profile     string          `json:"profile"`
	Title       string          `json:"title"`
	Sections    []CheckSection  `json:"sections"`
	Accessories []AccessoryItem `json:"accessories"`
}

var InspectionTemplates = map[string]InspectionTemplate{
	ProfileLaptop: {
		Profile: ProfileLaptop,
		Title:   "Pemeriksaan Kondisi Laptop (Hardware & Software)",
		Sections: []CheckSection{
			{Key: "hardware", Title: "A. HARDWARE", Items: []CheckItem{
				{Code: "hw_body", Label: "Bodi / casing (goresan, retak, penyok)"},
				{Code: "hw_screen", Label: "Layar / LCD (dead pixel, garis, flicker)"},
				{Code: "hw_keyboard", Label: "Keyboard (seluruh tombol berfungsi)"},
				{Code: "hw_touchpad", Label: "Touchpad / trackpad"},
				{Code: "hw_hinge", Label: "Engsel layar tidak longgar / patah"},
				{Code: "hw_battery", Label: "Baterai - kesehatan (health)", Battery: true},
				{Code: "hw_charger", Label: "Adaptor & pengisian daya berfungsi"},
				{Code: "hw_ports", Label: "Port (USB / HDMI / LAN / audio / card reader)"},
				{Code: "hw_audio", Label: "Speaker & mikrofon"},
				{Code: "hw_camera", Label: "Kamera / webcam"},
				{Code: "hw_wireless", Label: "Konektivitas WiFi & Bluetooth"},
				{Code: "hw_fan", Label: "Kipas / suhu (tidak overheat, tidak berisik)"},
			}},
			{Key: "software", Title: "B. SOFTWARE / SISTEM", Items: []CheckItem{
				{Code: "sw_os", Label: "Sistem Operasi - versi & status aktivasi/lisensi"},
				{Code: "sw_license", Label: "Lisensi software terpasang (Office, dsb.)"},
				{Code: "sw_av", Label: "Antivirus / endpoint security aktif & ter-update"},
				{Code: "sw_update", Label: "Update OS & patch keamanan terkini"},
				{Code: "sw_prev_account", Label: "Akun pengguna sebelumnya sudah dihapus / logout"},
				{Code: "sw_clean", Label: "Data pribadi pemilik lama sudah dibersihkan"},
				{Code: "sw_apps", Label: "Aplikasi kerja standar perusahaan terpasang"},
				{Code: "sw_vpn", Label: "Akses VPN / email / join domain berfungsi"},
				{Code: "sw_encrypt", Label: "Enkripsi disk (BitLocker / FileVault): aktif / tidak"},
				{Code: "sw_reset", Label: "Password & kredensial sudah direset"},
			}},
		},
		Accessories: []AccessoryItem{
			{"acc_charger", "Charger / Adaptor daya"}, {"acc_bag", "Tas / Sleeve / Dus"}, {"acc_mouse", "Mouse"},
			{"acc_cable", "Kabel (LAN/HDMI/USB)"}, {"acc_dongle", "Dongle / Adapter"},
			{"acc_manual", "Buku manual / Dokumen"}, {"acc_other", "Lainnya"},
		},
	},
	// Not part of the supplied form: a short check for other devices (monitor, phone, printer...).
	ProfileGeneral: {
		Profile: ProfileGeneral,
		Title:   "Pemeriksaan Kondisi Barang",
		Sections: []CheckSection{
			{Key: "physical", Title: "FISIK & FUNGSI", Items: []CheckItem{
				{Code: "gen_body", Label: "Fisik / casing (goresan, retak, penyok)"},
				{Code: "gen_function", Label: "Berfungsi normal (menyala / dapat digunakan)"},
				{Code: "gen_parts", Label: "Bagian & komponen utuh (tombol, port, kabel)"},
			}},
		},
		Accessories: []AccessoryItem{
			{"acc_charger", "Charger / Adaptor daya"}, {"acc_cable", "Kabel (power/data)"}, {"acc_bag", "Tas / Dus"},
			{"acc_manual", "Buku manual / Dokumen"}, {"acc_other", "Lainnya"},
		},
	},
}

// ValidInspectionProfile accepts none, general and laptop.
func ValidInspectionProfile(p string) bool {
	_, ok := InspectionTemplates[p]
	return ok || p == ProfileNone
}

// ItemLabel finds the checklist wording for a code in a profile.
func ItemLabel(profile, code string) string {
	t := InspectionTemplates[profile]
	for _, sec := range t.Sections {
		for _, it := range sec.Items {
			if it.Code == code {
				return it.Label
			}
		}
	}
	for _, a := range t.Accessories {
		if a.Code == code {
			return a.Label
		}
	}
	return code
}
