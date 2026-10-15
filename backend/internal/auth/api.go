package auth

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/contracts"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/idgen"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/sandi"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/token"
)

// umurPenyegar adalah masa berlaku token penyegar (SPEK-07 §A7-2).
const umurPenyegar = 30 * 24 * time.Hour

// pesanGagalMasuk selalu sama, apa pun sebabnya.
//
// Menyebut medan mana yang salah berarti memberi tahu penyerang bahwa satu nama
// pengguna ada — daftar pengguna bocor satu per satu (SPEK-07 §A7-5).
const pesanGagalMasuk = "nama pengguna atau kata sandi salah"

// Header identitas yang disuntikkan gateway setelah token terverifikasi.
const (
	HeaderPengguna = "X-Aset-Pengguna"
	HeaderPeran    = "X-Aset-Peran"
)

// Server memegang dependensi handler auth.
type Server struct {
	Store   *Store
	Penanda *token.Penanda
	Gembok  *Gembok

	// Pembatas laju rute publik SPEK-14. Setiap permintaan dihitung, bukan
	// hanya yang gagal.
	GembokDaftar    *Gembok
	GembokLupaEmail *Gembok
	GembokLupaIP    *Gembok

	Mandiri Mandiri

	// Tambahan memasang rute lain di bawah /api/auth (mis. ganti sandi), karena
	// chi tidak mengizinkan dua mount pada awalan yang sama.
	Tambahan func(chi.Router)
}

// NewServer membangun Server dengan gembok baku: lima kegagalan dalam 15 menit
// mengunci selama 15 menit.
func NewServer(db *gorm.DB, penanda *token.Penanda) *Server {
	return &Server{
		Store:   &Store{DB: db},
		Penanda: penanda,
		Gembok:  NewGembok(5, 15*time.Minute, 15*time.Minute),

		// SPEK-14 §A14-2 dan §A14-5.
		GembokDaftar:    NewGembok(5, time.Hour, time.Hour),
		GembokLupaEmail: NewGembok(5, time.Hour, time.Hour),
		GembokLupaIP:    NewGembok(20, time.Hour, time.Hour),
	}
}

// Rute memasang seluruh rute domain auth.
func (s *Server) Rute(r chi.Router) {
	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/login", s.HandleMasuk)
		r.Post("/refresh", s.HandleSegarkan)
		r.Post("/logout", s.HandleKeluar)
		r.Get("/saya", s.HandleSaya)

		// SPEK-14.
		r.Post("/daftar", s.HandleDaftar)
		r.Post("/lupa-sandi", s.HandleLupaSandi)
		r.Post("/setel-ulang-sandi", s.HandleSetelUlangSandi)
		r.Post("/tamu", s.HandleTamu)
		r.Post("/persetujuan/lihat", s.HandleLihatPersetujuan)
		r.Post("/persetujuan", s.HandlePutuskanPersetujuan)
		if s.Tambahan != nil {
			s.Tambahan(r)
		}
	})

	r.Route("/api/pengguna", func(r chi.Router) {
		r.Get("/", s.HandleDaftarPengguna)
		r.Post("/", s.HandleBuatPengguna)
		r.Get("/{ref}", s.HandleAmbilPengguna)
		r.Patch("/{ref}", s.HandleUbahPengguna)
		r.Delete("/{ref}", s.HandleHapusPengguna)
	})
}

// HandleMasuk menjawab POST /api/auth/login.
func (s *Server) HandleMasuk(w http.ResponseWriter, r *http.Request) {
	var in contracts.PermintaanMasuk
	if err := baca(w, r, &in); err != nil {
		tulisGalat(w, http.StatusBadRequest, "badan permintaan bukan JSON yang sah", nil)
		return
	}

	masukan := strings.TrimSpace(in.NamaPengguna)
	if masukan == "" || in.Sandi == "" {
		tulisGalat(w, http.StatusBadRequest, "email atau nama pengguna dan sandi wajib diisi", nil)
		return
	}
	// Batas panjang sebelum apa pun: masukan ini menjadi kunci gembok di
	// memori, dan kunci sepanjang 64 KiB adalah cara murah menghabiskannya.
	if len(masukan) > panjangEmailMaks {
		tulisGalat(w, http.StatusUnauthorized, pesanGagalMasuk, nil)
		return
	}

	u, errCari := s.Store.AmbilUntukMasuk(masukan)

	// Gembok dikunci pada akun yang ditemukan, supaya bergantian antara email
	// dan nama pengguna tidak menggandakan jatah tebakan (SPEK-14 §A14-9).
	nama := NormalkanNamaPengguna(masukan)
	if strings.Contains(masukan, "@") {
		nama = NormalkanEmail(masukan)
	}
	if errCari == nil {
		nama = u.Username
	}

	if terkunci, sisa := s.Gembok.Terkunci(nama); terkunci {
		w.Header().Set("Retry-After", strconv.Itoa(int(sisa.Seconds())+1))
		tulisGalat(w, http.StatusTooManyRequests,
			"terlalu banyak percobaan masuk, coba lagi nanti", nil)
		return
	}

	if errCari != nil {
		// Nama yang tidak ada tetap melewati satu perhitungan sebanding, supaya
		// waktu jawaban tidak membocorkan keberadaan nama itu (§A7-5).
		sandi.Rata()
		s.gagalMasuk(r, nama, "nama tidak ditemukan")
		tulisGalat(w, http.StatusUnauthorized, pesanGagalMasuk, nil)
		return
	}

	cocok, perluUlang, err := periksaSandi(in.Sandi, u.PasswordHash)
	if err != nil || !cocok {
		s.gagalMasuk(r, nama, "sandi salah")
		tulisGalat(w, http.StatusUnauthorized, pesanGagalMasuk, nil)
		return
	}

	// Status diperiksa SETELAH sandi terbukti: hanya pemilik akun yang boleh
	// tahu akunnya menunggu persetujuan (SPEK-14 §A14-1).
	switch u.Status() {
	case contracts.StatusMenunggu:
		tulisGalat(w, http.StatusForbidden, pesanMenunggu, nil)
		return
	case contracts.StatusNonaktif:
		s.gagalMasuk(r, nama, "akun nonaktif")
		tulisGalat(w, http.StatusUnauthorized, pesanGagalMasuk, nil)
		return
	}

	// Sandi lama berputaran rendah diperbarui diam-diam, mumpung sandi mentahnya
	// ada di tangan (§A7-4).
	if perluUlang {
		if hash, err := sandi.Sandikan(in.Sandi); err == nil {
			if err := s.Store.PerbaruiSandi(u.ID, hash); err != nil {
				log.Printf("auth: memperbarui sandi %s: %v", u.Username, err)
			}
		}
	}

	s.Gembok.Bersihkan(nama)
	if err := s.Store.CatatMasuk(u.ID); err != nil {
		log.Printf("auth: mencatat masuk %s: %v", u.Username, err)
	}

	sesi, err := s.terbitkanSesi(u)
	if err != nil {
		httpx.GalatInternal(w, "auth", "permintaan gagal", err)
		return
	}

	s.audit(r, u, "masuk", nil)
	httpx.WriteJSON(w, http.StatusOK, sesi)
}

// HandleSegarkan menjawab POST /api/auth/refresh.
func (s *Server) HandleSegarkan(w http.ResponseWriter, r *http.Request) {
	var in contracts.PermintaanPenyegar
	if err := baca(w, r, &in); err != nil || strings.TrimSpace(in.Penyegar) == "" {
		tulisGalat(w, http.StatusBadRequest, "penyegar wajib diisi", nil)
		return
	}

	t, err := s.Store.AmbilPenyegar(sandi.SHA256Hex(in.Penyegar))
	if err != nil {
		tulisGalat(w, http.StatusUnauthorized, "penyegar tidak sah", nil)
		return
	}

	u, err := s.Store.AmbilPenggunaID(t.UserID)
	if err != nil {
		tulisGalat(w, http.StatusUnauthorized, "penyegar tidak sah", nil)
		return
	}

	// Akun nonaktif diperiksa LEBIH DULU. Menonaktifkan akun mencabut seluruh
	// penyegarnya, jadi tab yang masih terbuka akan menyegarkan dengan token
	// yang sudah dicabut — itu bukan pencurian, dan mencatatnya sebagai
	// "penyegar_dipakai_ulang" memberi admin alarm palsu (audit Hari 13).
	if !u.Active {
		tulisGalat(w, http.StatusUnauthorized, "penyegar tidak sah", nil)
		return
	}

	// Token yang sudah dicabut dipakai lagi berarti ia bocor: yang asli dan yang
	// dicuri sama-sama dipakai. Seluruh sesi pengguna itu dicabut supaya pemilik
	// sah terpaksa masuk ulang dan penyerang ikut terputus (§A7-3).
	if t.RevokedAt != nil {
		s.tanganiPemakaianUlang(r, u)
		tulisGalat(w, http.StatusUnauthorized, "penyegar tidak sah", nil)
		return
	}

	if sudahLewat(t.ExpiresAt) {
		tulisGalat(w, http.StatusUnauthorized, "penyegar kedaluwarsa", nil)
		return
	}

	if !u.Active {
		tulisGalat(w, http.StatusUnauthorized, "penyegar tidak sah", nil)
		return
	}

	// Pencabutan inilah penentunya, bukan pembacaan di atas. Permintaan yang
	// kalah balapan berarti ada permintaan lain memakai token yang sama pada
	// saat yang sama — perlakuannya sama dengan pemakaian ulang.
	menang, err := s.Store.CabutPenyegar(t.ID)
	if err != nil {
		httpx.GalatInternal(w, "auth", "permintaan gagal", err)
		return
	}
	if !menang {
		s.tanganiPemakaianUlang(r, u)
		tulisGalat(w, http.StatusUnauthorized, "penyegar tidak sah", nil)
		return
	}

	sesi, err := s.terbitkanSesi(u)
	if err != nil {
		httpx.GalatInternal(w, "auth", "permintaan gagal", err)
		return
	}

	s.audit(r, u, "penyegar", nil)
	httpx.WriteJSON(w, http.StatusOK, sesi)
}

// tanganiPemakaianUlang mencabut seluruh sesi pengguna dan mencatatnya.
//
// Token yang bocor akan dipakai bersamaan dengan yang asli. Mencabut semuanya
// memaksa pemilik sah masuk ulang — satu-satunya cara memastikan penyerang ikut
// terputus (§A7-3).
func (s *Server) tanganiPemakaianUlang(r *http.Request, u *User) {
	n, err := s.Store.CabutSeluruhPenyegar(u.ID)
	if err != nil {
		log.Printf("auth: mencabut seluruh penyegar %s: %v", u.Username, err)
	}

	detail := `{"dicabut":` + strconv.FormatInt(n, 10) + `}`
	s.audit(r, u, "penyegar_dipakai_ulang", &detail)
}

// HandleKeluar menjawab POST /api/auth/logout.
//
// Selalu 204, bahkan bila penyegarnya tidak ditemukan: balasan yang berbeda
// akan memberi tahu penyerang token mana yang sah.
func (s *Server) HandleKeluar(w http.ResponseWriter, r *http.Request) {
	var in contracts.PermintaanPenyegar
	if err := baca(w, r, &in); err == nil && strings.TrimSpace(in.Penyegar) != "" {
		if t, err := s.Store.AmbilPenyegar(sandi.SHA256Hex(in.Penyegar)); err == nil {
			if _, err := s.Store.CabutPenyegar(t.ID); err != nil {
				log.Printf("auth: mencabut penyegar: %v", err)
			}
			if u, err := s.Store.AmbilPenggunaID(t.UserID); err == nil {
				s.audit(r, u, "keluar", nil)
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleSaya menjawab GET /api/auth/saya, memakai identitas yang disuntikkan gateway.
func (s *Server) HandleSaya(w http.ResponseWriter, r *http.Request) {
	ref := r.Header.Get(HeaderPengguna)
	if ref == "" {
		tulisGalat(w, http.StatusUnauthorized, "butuh autentikasi", nil)
		return
	}

	u, err := s.Store.AmbilPenggunaRef(ref)
	if err != nil {
		tulisGalat(w, http.StatusUnauthorized, "pengguna tidak dikenal", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, KeKontrak(*u))
}

// HandleDaftarPengguna menjawab GET /api/pengguna.
func (s *Server) HandleDaftarPengguna(w http.ResponseWriter, r *http.Request) {
	daftar, err := s.Store.DaftarPengguna()
	if err != nil {
		httpx.GalatInternal(w, "auth", "permintaan gagal", err)
		return
	}

	keluar := make([]contracts.Pengguna, 0, len(daftar))
	for _, u := range daftar {
		keluar = append(keluar, KeKontrak(u))
	}
	httpx.WriteJSON(w, http.StatusOK, keluar)
}

// HandleAmbilPengguna menjawab GET /api/pengguna/{ref}.
func (s *Server) HandleAmbilPengguna(w http.ResponseWriter, r *http.Request) {
	u, err := s.Store.AmbilPenggunaRef(chi.URLParam(r, "ref"))
	if err != nil {
		galatStore(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, KeKontrak(*u))
}

// HandleBuatPengguna menjawab POST /api/pengguna.
func (s *Server) HandleBuatPengguna(w http.ResponseWriter, r *http.Request) {
	var in contracts.PenggunaBaru
	if err := baca(w, r, &in); err != nil {
		tulisGalat(w, http.StatusBadRequest, "badan permintaan bukan JSON yang sah", nil)
		return
	}

	if medan := ValidasiPenggunaBaru(in); len(medan) > 0 {
		tulisGalat(w, http.StatusBadRequest, "pengguna tidak sah", medan)
		return
	}

	u, err := s.Store.BuatPengguna(in)
	if errors.Is(err, ErrNamaTerpakai) {
		tulisGalat(w, http.StatusConflict, ErrNamaTerpakai.Error(),
			map[string]string{"nama_pengguna": "sudah dipakai pengguna lain"})
		return
	}
	if errors.Is(err, ErrEmailTerpakai) {
		tulisGalat(w, http.StatusConflict, ErrEmailTerpakai.Error(),
			map[string]string{"email": "sudah dipakai pengguna lain"})
		return
	}
	if err != nil {
		httpx.GalatInternal(w, "auth", "permintaan gagal", err)
		return
	}

	s.audit(r, u, "pengguna_dibuat", nil)
	httpx.WriteJSON(w, http.StatusCreated, KeKontrak(*u))
}

// HandleUbahPengguna menjawab PATCH /api/pengguna/{ref}.
func (s *Server) HandleUbahPengguna(w http.ResponseWriter, r *http.Request) {
	var in contracts.PenggunaBaru
	if err := baca(w, r, &in); err != nil {
		tulisGalat(w, http.StatusBadRequest, "badan permintaan bukan JSON yang sah", nil)
		return
	}

	if medan := ValidasiPenggunaUbah(in); len(medan) > 0 {
		tulisGalat(w, http.StatusBadRequest, "perubahan tidak sah", medan)
		return
	}

	ref := chi.URLParam(r, "ref")

	// Pemasangan tanpa admin tidak bisa dipulihkan lewat API, jadi admin tidak
	// boleh menonaktifkan atau menurunkan peran dirinya sendiri.
	if ref == r.Header.Get(HeaderPengguna) {
		if in.Aktif != nil && !*in.Aktif {
			tulisGalat(w, http.StatusBadRequest,
				"tidak bisa menonaktifkan akun sendiri", nil)
			return
		}
		if in.Peran != nil && *in.Peran != r.Header.Get(HeaderPeran) {
			tulisGalat(w, http.StatusBadRequest,
				"tidak bisa mengubah peran akun sendiri", nil)
			return
		}
	}

	sebelum, err := s.Store.AmbilPenggunaRef(ref)
	if err != nil {
		galatStore(w, err)
		return
	}

	u, err := s.Store.UbahPengguna(ref, in, r.Header.Get(HeaderPengguna))
	if errors.Is(err, ErrNamaTerpakai) || errors.Is(err, ErrEmailTerpakai) {
		tulisGalat(w, http.StatusConflict, err.Error(), nil)
		return
	}
	if err != nil {
		galatStore(w, err)
		return
	}

	// Menonaktifkan pengguna atau mengganti sandinya harus memutus sesinya:
	// tanpa ini, token penyegarnya masih bisa dipakai sampai 30 hari.
	if (in.Aktif != nil && !*in.Aktif) || in.Sandi != nil {
		if _, err := s.Store.CabutSeluruhPenyegar(u.ID); err != nil {
			log.Printf("auth: mencabut penyegar %s: %v", u.Username, err)
		}
	}

	aksi := "pengguna_diubah"
	if sebelum.ApprovedAt == nil && u.ApprovedAt != nil {
		aksi = "pengguna_disetujui"
		s.kabariDisetujui(u)
	}
	s.audit(r, u, aksi, nil)
	httpx.WriteJSON(w, http.StatusOK, KeKontrak(*u))
}

// HandleHapusPengguna menjawab DELETE /api/pengguna/{ref}.
func (s *Server) HandleHapusPengguna(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "ref")

	if ref == r.Header.Get(HeaderPengguna) {
		tulisGalat(w, http.StatusBadRequest, "tidak bisa menghapus akun sendiri", nil)
		return
	}

	u, err := s.Store.AmbilPenggunaRef(ref)
	if err != nil {
		galatStore(w, err)
		return
	}
	if err := s.Store.HapusPengguna(ref); err != nil {
		galatStore(w, err)
		return
	}

	// Menghapus akun yang masih menunggu adalah MENOLAK pengajuannya.
	if u.Status() == contracts.StatusMenunggu {
		s.kabariDitolak(u)
		s.audit(r, u, "pengguna_ditolak", nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.audit(r, u, "pengguna_dihapus", nil)
	w.WriteHeader(http.StatusNoContent)
}

// terbitkanSesi menerbitkan token akses dan token penyegar sekaligus.
func (s *Server) terbitkanSesi(u *User) (*contracts.Sesi, error) {
	jti := idgen.New("jti")

	akses, kedaluwarsa, err := s.Penanda.Terbitkan(u.Ref, u.Role, u.Username, jti)
	if err != nil {
		return nil, err
	}

	penyegar, err := sandi.Acak(32)
	if err != nil {
		return nil, err
	}

	sampai := time.Now().UTC().Add(umurPenyegar)
	if err := s.Store.SimpanPenyegar(u.ID, sandi.SHA256Hex(penyegar),
		sampai.Format(time.RFC3339)); err != nil {
		return nil, err
	}

	return &contracts.Sesi{
		Token:          akses,
		Kedaluwarsa:    kedaluwarsa.Format(time.RFC3339),
		Penyegar:       penyegar,
		PenyegarSampai: sampai.Format(time.RFC3339),
		Pengguna:       KeKontrak(*u),
	}, nil
}

// gagalMasuk mencatat satu kegagalan pada gembok dan jejak audit.
func (s *Server) gagalMasuk(r *http.Request, nama, alasan string) {
	s.Gembok.Gagal(nama)

	ip := alamat(r)
	detail := `{"alasan":"` + alasan + `"}`
	if err := s.Store.Audit(nil, &nama, "masuk_gagal", "users", nil, &detail, &ip); err != nil {
		log.Printf("auth: menulis audit masuk_gagal: %v", err)
	}
}

// audit menulis satu baris jejak audit untuk pengguna yang sudah dikenali.
func (s *Server) audit(r *http.Request, u *User, aksi string, detail *string) {
	ip := alamat(r)
	if err := s.Store.Audit(&u.Ref, &u.Username, aksi, "users", &u.Ref, detail, &ip); err != nil {
		log.Printf("auth: menulis audit %s: %v", aksi, err)
	}
}

// ValidasiPenggunaBaru memeriksa medan untuk pembuatan pengguna.
func ValidasiPenggunaBaru(in contracts.PenggunaBaru) map[string]string {
	medan := map[string]string{}

	nama := ""
	if in.NamaPengguna != nil {
		nama = NormalkanNamaPengguna(*in.NamaPengguna)
	}
	switch {
	case nama == "":
		medan["nama_pengguna"] = "nama pengguna wajib diisi"
	case !NamaPenggunaSah(nama):
		medan["nama_pengguna"] = "nama pengguna harus 3 sampai 64 karakter, " +
			"hanya huruf kecil a-z, angka, titik, garis bawah, atau tanda hubung"
	}

	if in.Sandi == nil || len(*in.Sandi) < 12 {
		medan["sandi"] = "sandi minimal 12 karakter"
	}

	if in.Peran != nil && !termasuk(PeranSah, *in.Peran) {
		medan["peran"] = "peran harus salah satu dari: " + strings.Join(PeranSah, ", ")
	}
	if in.Email != nil && strings.TrimSpace(*in.Email) != "" && !EmailSah(NormalkanEmail(*in.Email)) {
		medan["email"] = pesanEmailTidakSah
	}

	return medan
}

// ValidasiPenggunaUbah memeriksa hanya medan yang benar-benar dikirim.
func ValidasiPenggunaUbah(in contracts.PenggunaBaru) map[string]string {
	medan := map[string]string{}

	if in.NamaPengguna != nil {
		if !NamaPenggunaSah(NormalkanNamaPengguna(*in.NamaPengguna)) {
			medan["nama_pengguna"] = "nama pengguna harus 3 sampai 64 karakter, " +
				"hanya huruf kecil a-z, angka, titik, garis bawah, atau tanda hubung"
		}
	}
	if in.Sandi != nil && len(*in.Sandi) < 12 {
		medan["sandi"] = "sandi minimal 12 karakter"
	}
	if in.Peran != nil && !termasuk(PeranSah, *in.Peran) {
		medan["peran"] = "peran harus salah satu dari: " + strings.Join(PeranSah, ", ")
	}
	if in.Email != nil && strings.TrimSpace(*in.Email) != "" && !EmailSah(NormalkanEmail(*in.Email)) {
		medan["email"] = pesanEmailTidakSah
	}

	return medan
}

func baca(w http.ResponseWriter, r *http.Request, dst any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(dst)
}

func tulisGalat(w http.ResponseWriter, kode int, pesan string, medan map[string]string) {
	httpx.WriteJSON(w, kode, contracts.Galat{Galat: pesan, Medan: medan})
}

func galatStore(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrTidakDitemukan) {
		tulisGalat(w, http.StatusNotFound, "pengguna tidak ditemukan", nil)
		return
	}
	httpx.GalatInternal(w, "auth", "permintaan gagal", err)
}

func alamat(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

func sudahLewat(rfc string) bool {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		// Waktu yang tidak terbaca diperlakukan sebagai kedaluwarsa: menerima
		// token yang masa berlakunya tidak diketahui lebih berbahaya daripada
		// memaksa satu kali masuk ulang.
		return true
	}
	return time.Now().UTC().After(t)
}

func termasuk(daftar []string, v string) bool {
	for _, d := range daftar {
		if d == v {
			return true
		}
	}
	return false
}
