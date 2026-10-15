package pdf

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
)

func ptr(s string) *string { return &s }

func sample(typ string, n int) *models.Transaction {
	t := &models.Transaction{
		OrderNumber: "OUT-20261014-001", Type: typ, Note: ptr("Pinjam untuk WFH"),
		CreatedAt: time.Date(2026, 10, 14, 9, 0, 0, 0, time.Local),
		Employee:  &models.Employee{Name: "Rizal", Department: ptr("Marketing")},
	}
	for i := 0; i < n; i++ {
		a := &models.Asset{
			Tag: fmt.Sprintf("LPT-%04d", i+1), Brand: ptr("Lenovo"), Model: ptr("IdeaPad Slim 3 dengan nama model yang cukup panjang"),
			SerialNumber: ptr("PF3ABC12"), CPU: ptr("Intel Core i5-12450H"), RAM: ptr("16 GB"), Storage: ptr("512 GB SSD"), OS: ptr("Windows 11"),
			Quantity: 1, Category: &models.Category{Name: "Laptop"},
		}
		d := models.TransactionDetail{Asset: a, NewStatus: models.StatusInUse}
		if typ == models.TxIn {
			d.NewStatus = models.StatusRepair
			d.Note = ptr("layar bergaris")
		}
		t.Details = append(t.Details, d)
	}
	return t
}

func TestReceiptFollowsTemplate(t *testing.T) {
	out, outText, err := render(sample(models.TxOut, 2), "Admin IT", Org{Approver: "Siti Nurchayati"})
	if err != nil || !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("pdf: %v", err)
	}
	has := func(all []string, want string) bool {
		for _, s := range all {
			if strings.Contains(s, want) {
				return true
			}
		}
		return false
	}
	for _, want := range []string{
		"FORM SERAH TERIMA ASET KANTOR", "Rabu, 14 Oktober 2026", "Rizal", "Marketing", "Tanggal", "Nama Karyawan", "Jabatan/Divisi",
		"Daftar Aset", "Nama Aset", "Jumlah", "Kondisi", "Keterangan", "LPT-0001", "Baik", "S/N: PF3ABC12",
		"Dengan ini, aset di atas telah diterima dalam kondisi sebagaimana tercantum.",
		"Diserahkan Oleh", "Diterima Oleh", "Nama: Admin IT", "Nama: Rizal", "Tanda Tangan:",
		"Diketahui Oleh", "Nama : Siti Nurchayati", "Catatan: Pinjam untuk WFH",
	} {
		if !has(outText, want) {
			t.Errorf("OUT form is missing %q", want)
		}
	}

	_, inText, err := render(sample(models.TxIn, 1), "Admin IT", Org{Approver: "Siti Nurchayati"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"FORM PENGEMBALIAN ASET KANTOR", "Perlu diservis", "layar bergaris", "Nama: Rizal", "Nama: Admin IT"} {
		if !has(inText, want) {
			t.Errorf("IN form is missing %q", want)
		}
	}
	// on a return the employee hands over, so they sign on the left (drawn first)
	idx := func(want string) int {
		for i, s := range inText {
			if s == want {
				return i
			}
		}
		return -1
	}
	if idx("Nama: Rizal") > idx("Nama: Admin IT") {
		t.Errorf("return: employee must be on the Diserahkan Oleh side")
	}
}

// PDF_OUT=/some/dir go test -run Render writes sample forms for visual review.
func TestRenderSamples(t *testing.T) {
	dir := os.Getenv("PDF_OUT")
	if dir == "" {
		t.Skip("set PDF_OUT to render samples")
	}
	for name, tx := range map[string]*models.Transaction{
		"out-2": sample(models.TxOut, 2), "in-1": sample(models.TxIn, 1), "out-9": sample(models.TxOut, 9),
	} {
		b, err := Receipt(tx, "Admin IT", Org{Approver: "Siti Nurchayati"})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".pdf"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
