package contracts

// Status akun (SPEK-14 §A14-1). Diturunkan dari active + approved_at.
const (
	StatusMenunggu = "menunggu"
	StatusAktif    = "aktif"
	StatusNonaktif = "nonaktif"
)

// PeranTamu adalah peran di token sesi tamu. Ia tidak pernah tersimpan di
// auth.db (SPEK-14 §A14-7).
const PeranTamu = "tamu"

// PermintaanDaftar adalah badan POST /api/auth/daftar.
//
// Tidak ada medan nama pengguna (diturunkan dari email) dan tidak ada medan
// aktif: akun lahir menunggu persetujuan. Peran adalah PERMINTAAN — teknisi,
// manajemen, atau auditor — dan tidak memberi apa pun sebelum admin
// menyetujuinya (SPEK-14 §A14-2).
type PermintaanDaftar struct {
	NamaLengkap string `json:"nama_lengkap"`
	Email       string `json:"email"`
	Sandi       string `json:"sandi"`
	Peran       string `json:"peran"`
}

// PeranDiminta memetakan pilihan di formulir pendaftaran ke peran sistem.
// admin sengaja tidak ada di sini.
var PeranDiminta = map[string]string{
	"teknisi":   "operator",
	"manajemen": "viewer",
	"auditor":   "auditor",
}

// PermintaanLupaSandi adalah badan POST /api/auth/lupa-sandi.
type PermintaanLupaSandi struct {
	Email string `json:"email"`
}

// PermintaanSetelUlang adalah badan POST /api/auth/setel-ulang-sandi.
type PermintaanSetelUlang struct {
	Email     string `json:"email"`
	Kode      string `json:"kode"`
	SandiBaru string `json:"sandi_baru"`
}

// SesiTamu adalah balasan POST /api/auth/tamu. Tanpa penyegar (§A14-7).
type SesiTamu struct {
	Token       string `json:"token"`
	Kedaluwarsa string `json:"kedaluwarsa"`
	Peran       string `json:"peran"`
}

// Pesan adalah balasan singkat tanpa data.
type Pesan struct {
	Pesan string `json:"pesan"`
}

// Keputusan persetujuan lewat tautan surel (SPEK-14 §A14-12).
const (
	KeputusanTerima = "terima"
	KeputusanTolak  = "tolak"
)

// PermintaanTautan adalah badan POST /api/auth/persetujuan/lihat.
type PermintaanTautan struct {
	Token string `json:"token"`
}

// PermintaanPersetujuan adalah badan POST /api/auth/persetujuan.
// Peran kosong berarti peran yang diminta pendaftar.
type PermintaanPersetujuan struct {
	Token     string `json:"token"`
	Keputusan string `json:"keputusan"`
	Peran     string `json:"peran,omitempty"`
}

// RincianPengajuan adalah yang ditampilkan halaman persetujuan: cukup untuk
// memutuskan, tidak lebih.
type RincianPengajuan struct {
	NamaLengkap  string `json:"nama_lengkap"`
	Email        string `json:"email"`
	PeranDiminta string `json:"peran_diminta"`
	DibuatPada   string `json:"dibuat_pada"`
	Kedaluwarsa  string `json:"kedaluwarsa"`
}

// HasilPersetujuan adalah balasan POST /api/auth/persetujuan.
type HasilPersetujuan struct {
	Pesan  string `json:"pesan"`
	Status string `json:"status"` // aktif | ditolak
	Peran  string `json:"peran,omitempty"`
}
