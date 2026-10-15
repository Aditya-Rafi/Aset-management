// Package akses menegakkan autentikasi dan matriks peran × rute untuk seluruh
// API. Ia padanan gateway NetAudit (SPEK-07 §A7-6/§A7-7): aplikasi ini satu
// proses, jadi matriksnya dipasang sebagai middleware, bukan service terpisah.
//
// Handler di belakangnya tidak memeriksa peran; mereka mempercayai identitas
// yang disuntikkan middleware ini.
package akses

import (
	"net/http"
	"strings"
)

// Peran yang dikenali. Sama persis dengan auth.PeranSah.
const (
	PeranAdmin    = "admin"
	PeranOperator = "operator"
	PeranAuditor  = "auditor"
	PeranViewer   = "viewer"

	// PeranTamu hanya ada di token sesi tamu, tidak pernah di tabel users. Ia
	// sengaja TIDAK masuk semuaPeran: tidak ada rute data yang menyebutnya,
	// jadi setiap token tamu ke rute data dijawab 403 (SPEK-14 §A14-7).
	PeranTamu = "tamu"
)

var (
	// semuaPeran: seluruh peran berakun boleh. Dipakai untuk rute baca.
	semuaPeran = []string{PeranAdmin, PeranOperator, PeranAuditor, PeranViewer}
	// penulis: peran yang mengubah keadaan sehari-hari (IT Support).
	penulis = []string{PeranAdmin, PeranOperator}
	// pembaca bukti: yang boleh mengunduh data mentah (ekspor CSV).
	pembacaBukti = []string{PeranAdmin, PeranOperator, PeranAuditor}
	// hanyaAdmin: penghapusan data induk dan pengelolaan akun.
	hanyaAdmin = []string{PeranAdmin}
)

// Aturan adalah satu baris matriks.
type Aturan struct {
	Metode string   // "GET", "POST", ... atau "*" untuk semua metode
	Pola   string   // segmen "{...}" cocok dengan apa saja
	Peran  []string // wajib diisi untuk rute non-publik (gagal-tertutup)
	Publik bool     // tanpa token sama sekali
}

// matriks adalah daftar IZIN, bukan daftar larangan. Yang lebih khusus
// didahulukan, karena pencocokan berhenti pada yang pertama cocok.
var matriks = []Aturan{
	// ---------------------------------------------------------------- publik
	{Metode: "GET", Pola: "/healthz", Publik: true},
	{Metode: "POST", Pola: "/api/auth/login", Publik: true},
	{Metode: "POST", Pola: "/api/auth/refresh", Publik: true},
	// SPEK-14. Semuanya menulis tanpa token, jadi masing-masing punya pembatas
	// laju sendiri di auth.
	{Metode: "POST", Pola: "/api/auth/daftar", Publik: true},
	{Metode: "POST", Pola: "/api/auth/lupa-sandi", Publik: true},
	{Metode: "POST", Pola: "/api/auth/setel-ulang-sandi", Publik: true},
	{Metode: "POST", Pola: "/api/auth/tamu", Publik: true},
	// Tautan Terima/Tolak di surel admin (SPEK-14 §A14-12): token 256 bit sekali
	// pakai di badan permintaan yang menjadi kewenangan, bukan sesi admin.
	{Metode: "POST", Pola: "/api/auth/persetujuan/lihat", Publik: true},
	{Metode: "POST", Pola: "/api/auth/persetujuan", Publik: true},

	// ------------------------------------------ terautentikasi, semua peran
	// Peran ditulis eksplisit, bukan dikosongkan: "kosong = cukup terautentikasi"
	// akan ikut meloloskan token tamu.
	{Metode: "POST", Pola: "/api/auth/logout", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/auth/saya", Peran: semuaPeran},
	{Metode: "PUT", Pola: "/api/auth/password", Peran: semuaPeran},

	// ----------------------------------------------------------------- umum
	{Metode: "GET", Pola: "/api/config", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/dashboard", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/logs", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/inspection-templates", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/suggest", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/select/employees", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/select/categories", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/select/assets", Peran: semuaPeran},

	// ------------------------------------------------------------- karyawan
	{Metode: "GET", Pola: "/api/employees", Peran: semuaPeran},
	{Metode: "POST", Pola: "/api/employees", Peran: penulis},
	{Metode: "GET", Pola: "/api/employees/{id}", Peran: semuaPeran},
	{Metode: "PUT", Pola: "/api/employees/{id}", Peran: penulis},
	// Alur masuk-keluar karyawan: menonaktifkan dan mengaktifkan kembali.
	{Metode: "POST", Pola: "/api/employees/{id}/offboard", Peran: penulis},
	{Metode: "POST", Pola: "/api/employees/{id}/reactivate", Peran: penulis},
	// Menghapus karyawan hanya mungkin bila tanpa riwayat; selebihnya dinonaktifkan.
	{Metode: "DELETE", Pola: "/api/employees/{id}", Peran: hanyaAdmin},

	// ------------------------------------------------------------- kategori
	{Metode: "GET", Pola: "/api/categories", Peran: semuaPeran},
	{Metode: "POST", Pola: "/api/categories", Peran: penulis},
	{Metode: "GET", Pola: "/api/categories/{id}", Peran: semuaPeran},
	{Metode: "PUT", Pola: "/api/categories/{id}", Peran: penulis},
	{Metode: "DELETE", Pola: "/api/categories/{id}", Peran: hanyaAdmin},

	// ----------------------------------------------------------------- aset
	// Yang khusus lebih dulu: /export dan /import/template bukan {id}.
	{Metode: "GET", Pola: "/api/assets/export", Peran: pembacaBukti},
	{Metode: "GET", Pola: "/api/assets/import/template", Peran: penulis},
	{Metode: "POST", Pola: "/api/assets/import", Peran: penulis},
	{Metode: "GET", Pola: "/api/assets/tag/{tag}", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/assets", Peran: semuaPeran},
	{Metode: "POST", Pola: "/api/assets", Peran: penulis},
	{Metode: "GET", Pola: "/api/assets/{id}", Peran: semuaPeran},
	{Metode: "PUT", Pola: "/api/assets/{id}", Peran: penulis},
	// Aset yang sudah punya riwayat tidak bisa dihapus sama sekali; yang bisa
	// dihapus hanyalah kesalahan input, dan itu keputusan admin.
	{Metode: "DELETE", Pola: "/api/assets/{id}", Peran: hanyaAdmin},
	{Metode: "PUT", Pola: "/api/assets/{id}/status", Peran: penulis},
	{Metode: "POST", Pola: "/api/assets/{id}/notes", Peran: penulis},
	{Metode: "GET", Pola: "/api/assets/{id}/logs", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/assets/{id}/condition-report", Peran: semuaPeran},
	{Metode: "POST", Pola: "/api/assets/{id}/images", Peran: penulis},
	{Metode: "DELETE", Pola: "/api/assets/{id}/images/{imageId}", Peran: penulis},
	{Metode: "GET", Pola: "/api/assets/{id}/qr", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/assets/{id}/barcode", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/uploads/assets/{name}", Peran: semuaPeran},

	// ------------------------------------------------------------ dokumen
	{Metode: "GET", Pola: "/api/transactions", Peran: semuaPeran},
	{Metode: "POST", Pola: "/api/transactions", Peran: penulis},
	{Metode: "GET", Pola: "/api/transactions/{id}", Peran: semuaPeran},
	// Membatalkan dokumen adalah koreksi harian IT Support yang baru membuatnya
	// salah; ia hanya berhasil bila aset belum berubah lagi.
	{Metode: "DELETE", Pola: "/api/transactions/{id}", Peran: penulis},
	{Metode: "GET", Pola: "/api/transactions/{id}/pdf", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/transactions/{id}/bast", Peran: semuaPeran},

	// ------------------------------------------------------------- laporan
	{Metode: "GET", Pola: "/api/monitoring/employees", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/monitoring/employees/{id}/assets", Peran: semuaPeran},
	{Metode: "GET", Pola: "/api/monitoring/employees/{id}/history", Peran: semuaPeran},

	// ------------------------------------------------------------ pengguna
	{Metode: "*", Pola: "/api/pengguna", Peran: hanyaAdmin},
	{Metode: "*", Pola: "/api/pengguna/{ref}", Peran: hanyaAdmin},
}

// Matriks mengembalikan salinan aturan, dipakai uji dan dokumentasi.
func Matriks() []Aturan {
	keluar := make([]Aturan, len(matriks))
	copy(keluar, matriks)
	return keluar
}

// Putusan adalah hasil pencocokan satu permintaan terhadap matriks.
type Putusan struct {
	Dikenal bool // rute ada di matriks; bila false pemanggil menjawab 404
	Publik  bool // tidak perlu token
	Boleh   bool // peran yang diberikan berhak
}

// Izinkan memutuskan apakah peran boleh mengakses metode+path. Dengan peran
// kosong, Boleh selalu false kecuali rutenya publik.
func Izinkan(metode, path, peran string) Putusan {
	// RFC 9110 §9.3.2: HEAD hanya membaca, kewenangannya identik dengan GET.
	if strings.EqualFold(metode, http.MethodHead) {
		metode = http.MethodGet
	}

	for _, a := range matriks {
		if a.Metode != "*" && !strings.EqualFold(a.Metode, metode) {
			continue
		}
		if !cocokPola(a.Pola, path) {
			continue
		}
		switch {
		case a.Publik:
			return Putusan{Dikenal: true, Publik: true, Boleh: true}
		case peran == "", len(a.Peran) == 0:
			// Gagal-tertutup: baris tanpa daftar peran tidak meloloskan siapa pun.
			return Putusan{Dikenal: true}
		}
		for _, p := range a.Peran {
			if p == peran {
				return Putusan{Dikenal: true, Boleh: true}
			}
		}
		return Putusan{Dikenal: true}
	}
	return Putusan{}
}

// cocokPola mencocokkan path terhadap pola bergaya chi: segmen "{...}" cocok
// dengan satu segmen apa pun, dan jumlah segmen harus sama persis. Tidak ada
// pola cocok-semua.
func cocokPola(pola, path string) bool {
	if pola == path {
		return true
	}
	pp := strings.Split(strings.Trim(pola, "/"), "/")
	ps := strings.Split(strings.Trim(path, "/"), "/")
	if len(pp) != len(ps) {
		return false
	}
	for i := range pp {
		if strings.HasPrefix(pp[i], "{") && strings.HasSuffix(pp[i], "}") {
			if ps[i] == "" {
				return false
			}
			continue
		}
		if pp[i] != ps[i] {
			return false
		}
	}
	return true
}
