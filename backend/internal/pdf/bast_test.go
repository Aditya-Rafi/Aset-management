package pdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

func bptr(b bool) *bool { return &b }
func iptr(i int) *int   { return &i }

// laptopInspection answers every checklist item "ok" except the codes in bad.
func laptopInspection(conclusion string, bad map[string]string) models.InspectionData {
	d := models.InspectionData{Profile: models.ProfileLaptop, Conclusion: conclusion, BatteryHealth: iptr(87), Notes: "Unit bersih, tidak ada stiker pribadi."}
	for _, sec := range models.InspectionTemplates[models.ProfileLaptop].Sections {
		for _, it := range sec.Items {
			r := models.CheckResult{Code: it.Code, Result: models.CheckOK}
			if note, isBad := bad[it.Code]; isBad {
				r.Result, r.Note = models.CheckBad, note
			}
			d.Items = append(d.Items, r)
		}
	}
	for _, a := range models.InspectionTemplates[models.ProfileLaptop].Accessories {
		d.Accessories = append(d.Accessories, models.AccessoryResult{Code: a.Code, Present: bptr(a.Code == "acc_charger" || a.Code == "acc_bag"), Condition: "Baik"})
	}
	return d
}

func bastSample(typ string, laptops int) *models.Transaction {
	t := &models.Transaction{
		OrderNumber: "OUT-20261015-001", Type: typ,
		CreatedAt: time.Date(2026, 10, 15, 9, 0, 0, 0, time.Local),
		Employee:  &models.Employee{Name: "Rizal", NIK: ptr("EMP-0042"), Department: ptr("Marketing")},
	}
	if typ == models.TxTransfer {
		t.FromEmployee = &models.Employee{Name: "Mumu", NIK: ptr("EMP-0007"), Department: ptr("Finance")}
	}
	lap := models.Category{Name: "Laptop", Inspection: models.ProfileLaptop}
	for i := 0; i < laptops; i++ {
		a := &models.Asset{ID: uint(i + 1), Tag: "LPT-000" + string(rune('1'+i)), Brand: ptr("Lenovo"), Model: ptr("IdeaPad Slim 3"), SerialNumber: ptr("PF3ABC1" + string(rune('0'+i))), Quantity: 1, Category: &lap}
		t.Details = append(t.Details, models.TransactionDetail{AssetID: a.ID, Asset: a, NewStatus: models.StatusInUse})
		bad := map[string]string{}
		concl := models.ConclusionFit
		if i == 1 {
			bad["hw_screen"] = "Garis vertikal di sisi kanan"
			bad["hw_hinge"] = "Engsel kiri longgar"
			concl = models.ConclusionRepair
		}
		t.Inspections = append(t.Inspections, models.Inspection{AssetID: a.ID, Kind: typ, Data: laptopInspection(concl, bad)})
	}
	mon := models.Category{Name: "Monitor", Inspection: models.ProfileNone}
	m := &models.Asset{ID: 50, Tag: "MON-0001", Brand: ptr("LG"), Model: ptr("24MR400-B"), Quantity: 1, Category: &mon}
	t.Details = append(t.Details, models.TransactionDetail{AssetID: m.ID, Asset: m, NewStatus: models.StatusInUse})
	return t
}

func TestBASTFollowsTemplate(t *testing.T) {
	op := BastParty{Name: "Admin IT"}
	out, text, err := renderBAST(bastSample(models.TxOut, 1), op, BastOrg{Name: "PT Contoh Jaya", Address: "Jl. Contoh 1 - 021-000 - it@contoh.id"})
	if err != nil || !strings.HasPrefix(string(out), "%PDF") {
		t.Fatalf("pdf: %v", err)
	}
	all := strings.Join(text, "\n")
	for _, want := range []string{
		"PT Contoh Jaya", "BERITA ACARA SERAH TERIMA BARANG", "(Aset / Inventaris Kantor)", "Nomor: OUT-20261015-001",
		"Pada hari ini Kamis, tanggal 15 bulan Oktober tahun 2026", "I. PIHAK PERTAMA (Yang Menyerahkan)", "II. PIHAK KEDUA (Yang Menerima)",
		"NIK / ID Karyawan", ": EMP-0042", "Selanjutnya disebut sebagai PIHAK KEDUA.", "A. Rincian Barang", "Serial Number / Asset Tag",
		"PF3ABC10 / LPT-0001", "B. Pemeriksaan Kondisi Laptop (Hardware & Software)", "A. HARDWARE", "B. SOFTWARE / SISTEM",
		"Baterai - kesehatan (health): 87 %", "Kesimpulan pemeriksaan laptop:  [x] Layak digunakan     [ ] Perlu perbaikan     [ ] Tidak layak",
		"C. Kelengkapan / Aksesoris", "Charger / Adaptor daya", "Catatan kondisi tambahan:", "D. Ketentuan dan Pernyataan",
		"Berita acara ini dibuat dalam rangkap 2 (dua)", "Yang Menyerahkan,", "Menyaksikan,", "Mengetahui,", "Atasan Langsung / HRD", "(Admin IT)",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("BAST is missing %q", want)
		}
	}
	// a monitor has no checklist, so it appears in section A only
	if strings.Count(all, "MON-0001") != 1 {
		t.Errorf("monitor must only be listed in the item table")
	}

	_, in, _ := renderBAST(bastSample(models.TxIn, 1), op, BastOrg{})
	allIn := strings.Join(in, "\n")
	if !strings.Contains(allIn, "BERITA ACARA PENGEMBALIAN BARANG") || !strings.Contains(allIn, "dibuat dengan sebenar-benarnya") {
		t.Errorf("return BAST title/closing wrong")
	}
	// on a return the employee is the first party and IT receives
	if strings.Index(allIn, "Rizal") > strings.Index(allIn, "Admin IT") {
		t.Errorf("return: employee should be PIHAK PERTAMA")
	}

	_, tr, _ := renderBAST(bastSample(models.TxTransfer, 1), op, BastOrg{})
	allTr := strings.Join(tr, "\n")
	if !strings.Contains(allTr, "Mumu") || !strings.Contains(allTr, "EMP-0007") || !strings.Contains(allTr, "(Admin IT)") {
		t.Errorf("transfer: first party Mumu and IT as witness expected")
	}
}

// PDF_OUT=/some/dir go test -run RenderBAST writes sample forms for visual review.
func TestRenderBASTSamples(t *testing.T) {
	dir := os.Getenv("PDF_OUT")
	if dir == "" {
		t.Skip("set PDF_OUT to render samples")
	}
	for name, tx := range map[string]*models.Transaction{
		"bast-out": bastSample(models.TxOut, 1), "bast-two": bastSample(models.TxOut, 2), "bast-transfer": bastSample(models.TxTransfer, 1),
	} {
		b, err := BAST(tx, BastParty{Name: "Admin IT"}, BastOrg{Name: "PT Contoh Jaya", Address: "Jl. Contoh No. 1, Jakarta - 021-5550000 - it@contoh.id"})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".pdf"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
