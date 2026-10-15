package contracts

// PermintaanMasuk adalah badan POST /api/auth/login.
type PermintaanMasuk struct {
	NamaPengguna string `json:"nama_pengguna"`
	Sandi        string `json:"sandi"`
}

// PermintaanPenyegar adalah badan POST /api/auth/refresh dan /api/auth/logout.
type PermintaanPenyegar struct {
	Penyegar string `json:"penyegar"`
}

// Sesi adalah balasan login dan refresh.
type Sesi struct {
	Token          string   `json:"token"`
	Kedaluwarsa    string   `json:"kedaluwarsa"` // RFC3339 UTC
	Penyegar       string   `json:"penyegar"`
	PenyegarSampai string   `json:"penyegar_sampai"` // RFC3339 UTC
	Pengguna       Pengguna `json:"pengguna"`
}

// Pengguna adalah bentuk pengguna yang keluar lewat API.
// Sandi dan turunannya tidak pernah termasuk.
type Pengguna struct {
	Ref           string  `json:"ref"`
	NamaPengguna  string  `json:"nama_pengguna"`
	NamaLengkap   string  `json:"nama_lengkap"`
	Peran         string  `json:"peran"`
	Aktif         bool    `json:"aktif"`
	Email         string  `json:"email,omitempty"`
	Status        string  `json:"status"` // menunggu | aktif | nonaktif (SPEK-14 §A14-1)
	DisetujuiPada *string `json:"disetujui_pada,omitempty"`
	MasukTerakhir *string `json:"masuk_terakhir,omitempty"`
	DibuatPada    string  `json:"dibuat_pada"`
}

// PenggunaBaru adalah badan POST dan PATCH /api/pengguna.
// Pointer dipakai supaya PATCH bisa membedakan "tidak dikirim" dari "dikosongkan".
type PenggunaBaru struct {
	NamaPengguna *string `json:"nama_pengguna"`
	Sandi        *string `json:"sandi"`
	NamaLengkap  *string `json:"nama_lengkap"`
	Peran        *string `json:"peran"`
	Aktif        *bool   `json:"aktif"`
	Email        *string `json:"email"` // SPEK-14 §A14-3; string kosong menghapusnya
}

// Galat adalah bentuk galat endpoint autentikasi: satu pesan dan, bila ada,
// pesan per medan.
type Galat struct {
	Galat string            `json:"galat"`
	Medan map[string]string `json:"medan,omitempty"`
}
