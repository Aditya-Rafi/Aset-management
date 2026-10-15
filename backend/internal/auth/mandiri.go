package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/contracts"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/idgen"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/sandi"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/surel"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/token"
)

// Batas SPEK-14. Diubah hanya lewat revisi spek.
const (
	umurOTP          = 10 * time.Minute // §A14-4
	maksPercobaanOTP = 5                // §A14-4
	batasMenunggu    = 50               // §A14-2
	panjangEmailMaks = 254              // RFC 5321 §4.5.3.1.3
	panjangNamaMaks  = 128
)

var (
	// ErrAntreanPenuh dikembalikan bila akun menunggu sudah mencapai batas.
	ErrAntreanPenuh = errors.New("antrean pendaftaran penuh")
	// ErrKodeTidakSah mencakup SEMUA sebab kegagalan setel ulang (§A14-5).
	ErrKodeTidakSah = errors.New("kode tidak sah atau sudah kedaluwarsa")
)

const (
	pesanEmailTidakSah   = "email tidak sah"
	pesanTerdaftar       = "pendaftaran diterima; akun menunggu persetujuan admin"
	pesanLupaDiterima    = "bila email itu milik akun aktif, kode setel ulang sudah dikirim dan berlaku 10 menit"
	pesanMenunggu        = "akun menunggu persetujuan admin"
	pesanKembarDaftar    = "email sudah terdaftar"
	pesanSurelMati       = "pemulihan sandi lewat email belum dikonfigurasi; hubungi admin"
	pesanTamuMati        = "mode tamu dinonaktifkan"
	pesanTerlaluSering   = "terlalu banyak permintaan, coba lagi nanti"
	pesanSandiDiganti    = "sandi berhasil diganti; silakan masuk dengan sandi baru"
	subjekOTP            = "Kode setel ulang sandi Aset IT"
	subjekSandiDiganti   = "Sandi Aset IT Anda baru saja diganti"
	subjekAkunDisetujui  = "Akun Aset IT Anda sudah disetujui"
	subjekPengajuan      = "Pengajuan akun Aset IT diterima"
	subjekPengajuanAdmin = "Pengajuan akun baru menunggu persetujuan"
	subjekDitolak        = "Pengajuan akun Aset IT tidak disetujui"
	subjekBelumAktif     = "Sandi Aset IT belum bisa diatur ulang"
	namaTampilTamu       = "Tamu"
	awalanSubjekTamu     = "tamu"
	kapasitasAntreanLupa = 64
)

// NormalkanEmail memangkas spasi dan menurunkan ke huruf kecil.
//
// Bagian lokal email secara teori peka huruf besar, tetapi tidak satu pun
// penyedia umum memperlakukannya begitu, dan dua akun untuk "Ani@x" dan "ani@x"
// adalah cacat identitas yang sama dengan §A7-15.
func NormalkanEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// EmailSah memeriksa email yang SUDAH dinormalkan: alamat polos, bukan
// "Nama <alamat>", tanpa baris baru, paling panjang 254 karakter.
func EmailSah(e string) bool {
	if e == "" || len(e) > panjangEmailMaks || strings.ContainsAny(e, "\r\n <>") {
		return false
	}
	a, err := mail.ParseAddress(e)
	if err != nil || a.Address != e {
		return false
	}
	at := strings.LastIndexByte(e, '@')
	return at > 0 && strings.Contains(e[at+1:], ".")
}

// NamaDariEmail menurunkan nama pengguna dari bagian lokal email, dinormalkan
// ke pola §A7-15: huruf di luar a-z0-9._- diganti tanda hubung, dipotong 64,
// dan diisi "pengguna" bila hasilnya kurang dari 3 karakter.
func NamaDariEmail(email string) string {
	lokal := NormalkanEmail(email)
	if i := strings.LastIndexByte(lokal, '@'); i >= 0 {
		lokal = lokal[:i]
	}
	if i := strings.IndexByte(lokal, '+'); i >= 0 {
		lokal = lokal[:i] // nama+tag@ adalah orang yang sama dengan nama@
	}

	var b strings.Builder
	for _, r := range lokal {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	nama := strings.Trim(b.String(), ".-_")
	if len(nama) > 64 {
		nama = nama[:64]
	}
	if len(nama) < 3 {
		nama = "pengguna"
	}
	return nama
}

// ---------------------------------------------------------------- store

// AmbilUntukMasuk mengembalikan pengguna APA PUN statusnya, dicari lewat email
// bila masukan memuat "@" dan lewat nama pengguna bila tidak (SPEK-14 §A14-9).
// Hanya dipakai saat masuk, yang perlu mengenali "menunggu" setelah sandinya
// terbukti.
func (s *Store) AmbilUntukMasuk(masukan string) (*User, error) {
	var u User
	q := s.DB.Where("username = ?", NormalkanNamaPengguna(masukan))
	if strings.Contains(masukan, "@") {
		q = s.DB.Where("email = ?", NormalkanEmail(masukan))
	}
	err := q.Limit(1).Find(&u).Error
	if err != nil {
		return nil, err
	}
	if u.ID == 0 {
		return nil, ErrTidakDitemukan
	}
	return &u, nil
}

// AmbilPenggunaEmailAktif mengembalikan pengguna AKTIF pemilik email itu.
func (s *Store) AmbilPenggunaEmailAktif(email string) (*User, error) {
	var u User
	err := s.DB.Where("email = ? AND active = 1", NormalkanEmail(email)).Limit(1).Find(&u).Error
	if err != nil {
		return nil, err
	}
	if u.ID == 0 {
		return nil, ErrTidakDitemukan
	}
	return &u, nil
}

// Daftarkan membuat akun yang MENUNGGU persetujuan: tidak aktif, belum
// disetujui, peran viewer (SPEK-14 §A14-2). Batas antrean diperiksa dalam
// transaksi yang sama dengan penyisipan, supaya dua pendaftaran serentak tidak
// sama-sama lolos pada hitungan 49.
func (s *Store) Daftarkan(in contracts.PermintaanDaftar) (*User, error) {
	hash, err := sandi.Sandikan(in.Sandi)
	if err != nil {
		return nil, err
	}

	peran, ok := contracts.PeranDiminta[in.Peran]
	if !ok {
		return nil, fmt.Errorf("peran %q tidak bisa diminta", in.Peran)
	}

	email := NormalkanEmail(in.Email)
	dasar := NamaDariEmail(email)
	lengkap := strings.TrimSpace(in.NamaLengkap)
	if lengkap == "" {
		lengkap = dasar
	}

	u := &User{
		Ref:          idgen.New(RefPrefix),
		PasswordHash: hash,
		FullName:     lengkap,
		Role:         peran,
		Active:       false,
		Email:        &email,
	}

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&User{}).
			Where("active = 0 AND approved_at IS NULL").Count(&n).Error; err != nil {
			return err
		}
		if n >= batasMenunggu {
			return ErrAntreanPenuh
		}

		// Nama pertama yang belum terpakai: ani, ani-2, ani-3, ... Dicari di
		// dalam transaksi yang sama, jadi dua pendaftar "ani@" serentak tidak
		// sama-sama mendapat "ani".
		for i := 1; ; i++ {
			calon := dasar
			if i > 1 {
				akhiran := "-" + strconv.Itoa(i)
				if len(calon)+len(akhiran) > 64 {
					calon = calon[:64-len(akhiran)]
				}
				calon += akhiran
			}
			var ada int64
			if err := tx.Model(&User{}).Where("username = ?", calon).Count(&ada).Error; err != nil {
				return err
			}
			if ada == 0 {
				u.Username = calon
				break
			}
		}

		if err := tx.Create(u).Error; err != nil {
			return galatKembar(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

// hashKode menurunkan hash yang disimpan dari ref permintaan dan kodenya.
func hashKode(ref, kode string) string { return sandi.SHA256Hex(ref + ":" + kode) }

// kodeOTP membangkitkan enam angka seragam dari crypto/rand.
func kodeOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// BuatReset membuat permintaan setel ulang baru dan menghanguskan semua
// permintaan lama pengguna itu yang belum terpakai. Mengembalikan kode
// mentahnya, satu-satunya saat kode itu ada di luar surel.
func (s *Store) BuatReset(userID uint, kini time.Time) (string, error) {
	kode, err := kodeOTP()
	if err != nil {
		return "", err
	}
	ref := idgen.New("rst")
	sekarang := kini.UTC().Format(time.RFC3339)

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		// Hangus = masa berlakunya dipotong sampai sekarang. used_at tidak
		// disentuh: "terpakai" dan "digantikan" adalah dua fakta berbeda.
		if err := tx.Model(&PasswordReset{}).
			Where("user_id = ? AND used_at IS NULL AND expires_at > ?", userID, sekarang).
			Update("expires_at", sekarang).Error; err != nil {
			return err
		}
		return tx.Create(&PasswordReset{
			Ref:       ref,
			UserID:    userID,
			CodeHash:  hashKode(ref, kode),
			ExpiresAt: kini.UTC().Add(umurOTP).Format(time.RFC3339),
			CreatedAt: sekarang,
		}).Error
	})
	if err != nil {
		return "", err
	}
	return kode, nil
}

// PakaiReset memeriksa kode dan, bila benar, mengganti sandi serta mencabut
// seluruh sesi — dalam SATU transaksi (SPEK-14 §A14-4).
//
// Setiap kegagalan mengembalikan ErrKodeTidakSah, apa pun sebabnya. Tebakan
// salah tetap DITULIS (attempts bertambah) walau fungsi ini mengembalikan
// galat; karena itu transaksinya selesai dengan sukses dan galatnya diputuskan
// sesudahnya.
func (s *Store) PakaiReset(email, kode, sandiBaru string, kini time.Time) (*User, error) {
	if len(sandiBaru) < SandiMinimum {
		return nil, ErrSandiPendek
	}
	// Disandikan di luar transaksi: PBKDF2 600 ribu putaran tidak boleh
	// menahan satu-satunya koneksi basis data.
	hashBaru, err := sandi.Sandikan(sandiBaru)
	if err != nil {
		return nil, err
	}

	sekarang := kini.UTC().Format(time.RFC3339)
	var u User
	cocok := false

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("email = ? AND active = 1", NormalkanEmail(email)).
			Limit(1).Find(&u).Error; err != nil {
			return err
		}
		if u.ID == 0 {
			return nil
		}

		var r PasswordReset
		if err := tx.Where("user_id = ? AND used_at IS NULL", u.ID).
			Order("id DESC").Limit(1).Find(&r).Error; err != nil {
			return err
		}
		if r.ID == 0 || sudahLewatPada(r.ExpiresAt, kini) || r.Attempts >= maksPercobaanOTP {
			return nil
		}

		if !hmac.Equal([]byte(hashKode(r.Ref, kode)), []byte(r.CodeHash)) {
			return tx.Model(&PasswordReset{}).Where("id = ?", r.ID).
				Update("attempts", gorm.Expr("attempts + 1")).Error
		}

		// Syarat used_at IS NULL membuat pemakaian hanya bisa menang sekali,
		// alasan yang sama dengan CabutPenyegar (§A7-12).
		hasil := tx.Model(&PasswordReset{}).
			Where("id = ? AND used_at IS NULL", r.ID).Update("used_at", sekarang)
		if hasil.Error != nil {
			return hasil.Error
		}
		if hasil.RowsAffected != 1 {
			return nil
		}

		if err := tx.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]any{
			"password":   hashBaru,
			"updated_at": time.Now(),
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", u.ID).
			Update("revoked_at", sekarang).Error; err != nil {
			return err
		}
		cocok = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !cocok {
		return nil, ErrKodeTidakSah
	}
	return &u, nil
}

func sudahLewatPada(rfc string, kini time.Time) bool {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return true
	}
	return !kini.UTC().Before(t)
}

// ---------------------------------------------------------------- handler

// Pengantar menerima surat untuk dikirim. *surel.Antrean memenuhinya; uji
// memakai pengantar yang mencatat.
type Pengantar interface {
	Masukkan(surel.Surat) bool
}

// Mandiri memegang dependensi fitur SPEK-14. Nilai nolnya aman: surel mati,
// tamu mati.
type Mandiri struct {
	// Surel nil berarti pemulihan lewat email tidak dikonfigurasi (§A14-6).
	Surel Pengantar
	// PenandaTamu nil berarti mode tamu dimatikan (§A14-7).
	PenandaTamu *token.Penanda

	// URLPublik adalah alamat SPA yang dibuka admin dari surel, tanpa garis
	// miring penutup, mis. http://localhost:8081 (PUBLIC_BASE_URL).
	URLPublik string

	// Sinkron menjalankan lupa sandi di jalur permintaan. HANYA untuk uji;
	// di produksi ia membuka kanal waktu yang §A14-5 tutup.
	Sinkron bool

	lupa chan string
}

// PasangMandiri menyalakan pekerja lupa sandi. Dipanggil sekali dari main.
func (s *Server) PasangMandiri(m Mandiri) {
	s.Mandiri = m
	if !s.Mandiri.Sinkron {
		s.Mandiri.lupa = make(chan string, kapasitasAntreanLupa)
		go func() {
			for email := range s.Mandiri.lupa {
				s.prosesLupa(email)
			}
		}()
	}
}

// HandleDaftar menjawab POST /api/auth/daftar (publik).
func (s *Server) HandleDaftar(w http.ResponseWriter, r *http.Request) {
	var in contracts.PermintaanDaftar
	if err := baca(w, r, &in); err != nil {
		tulisGalat(w, http.StatusBadRequest, "badan permintaan bukan JSON yang sah", nil)
		return
	}

	medan := map[string]string{}
	if _, ok := contracts.PeranDiminta[in.Peran]; !ok {
		medan["peran"] = "peran harus teknisi, manajemen, atau auditor"
	}
	if strings.TrimSpace(in.NamaLengkap) == "" {
		medan["nama_lengkap"] = "nama lengkap wajib diisi"
	}
	if !EmailSah(NormalkanEmail(in.Email)) {
		medan["email"] = pesanEmailTidakSah
	}
	if len(in.Sandi) < SandiMinimum {
		medan["sandi"] = "sandi minimal 12 karakter"
	}
	if len(strings.TrimSpace(in.NamaLengkap)) > panjangNamaMaks || strings.ContainsAny(in.NamaLengkap, "\r\n") {
		medan["nama_lengkap"] = "nama lengkap paling panjang 128 karakter, satu baris"
	}
	if len(medan) > 0 {
		tulisGalat(w, http.StatusBadRequest, "pendaftaran tidak sah", medan)
		return
	}

	kunci := "daftar-ip:" + alamat(r)
	if !s.lolosLaju(w, s.GembokDaftar, kunci) {
		return
	}

	u, err := s.Store.Daftarkan(in)
	switch {
	case errors.Is(err, ErrEmailTerpakai), errors.Is(err, ErrNamaTerpakai):
		tulisGalat(w, http.StatusConflict, pesanKembarDaftar,
			map[string]string{"email": "sudah terdaftar"})
		return
	case errors.Is(err, ErrAntreanPenuh):
		tulisGalat(w, http.StatusServiceUnavailable,
			"pendaftaran ditutup sementara: terlalu banyak akun menunggu persetujuan", nil)
		return
	case err != nil:
		httpx.GalatInternal(w, "auth", "permintaan gagal", err)
		return
	}

	s.audit(r, u, "pendaftaran", nil)
	s.kabariPengajuan(u)
	httpx.WriteJSON(w, http.StatusCreated, contracts.Pesan{Pesan: pesanTerdaftar})
}

// HandleLupaSandi menjawab POST /api/auth/lupa-sandi (publik).
//
// Jawabannya SAMA untuk email terdaftar dan tidak (§A14-5). Seluruh kerja yang
// bergantung pada ada-tidaknya akun terjadi di pekerja latar.
func (s *Server) HandleLupaSandi(w http.ResponseWriter, r *http.Request) {
	if s.Mandiri.Surel == nil {
		tulisGalat(w, http.StatusServiceUnavailable, pesanSurelMati, nil)
		return
	}

	var in contracts.PermintaanLupaSandi
	if err := baca(w, r, &in); err != nil {
		tulisGalat(w, http.StatusBadRequest, "badan permintaan bukan JSON yang sah", nil)
		return
	}
	email := NormalkanEmail(in.Email)
	if !EmailSah(email) {
		tulisGalat(w, http.StatusBadRequest, pesanEmailTidakSah,
			map[string]string{"email": pesanEmailTidakSah})
		return
	}

	// Dikunci pada email yang DIKETIK, bukan akun yang ditemukan, supaya batas
	// itu sendiri tidak membedakan keduanya.
	if !s.lolosLaju(w, s.GembokLupaIP, "lupa-ip:"+alamat(r)) ||
		!s.lolosLaju(w, s.GembokLupaEmail, "lupa-email:"+email) {
		return
	}

	if s.Mandiri.Sinkron {
		s.prosesLupa(email)
	} else {
		select {
		case s.Mandiri.lupa <- email:
		default:
			log.Printf("auth: antrean lupa sandi penuh, permintaan untuk %s dibuang", surel.Samarkan(email))
		}
	}
	httpx.WriteJSON(w, http.StatusAccepted, contracts.Pesan{Pesan: pesanLupaDiterima})
}

// prosesLupa membuat kode dan mengantrekan surelnya bila email itu milik
// akun aktif. Tidak mengembalikan apa pun: pemanggilnya sudah menjawab.
func (s *Server) prosesLupa(email string) {
	u, err := s.Store.AmbilUntukMasuk(email)
	if err != nil {
		if !errors.Is(err, ErrTidakDitemukan) {
			log.Printf("auth: mencari pemilik email: %v", err)
		}
		return
	}

	// Akun yang belum atau tidak lagi aktif tidak mendapat kode, tetapi
	// pemiliknya diberi tahu SEBABNYA. Diam membuat pemilik mengira surelnya
	// hilang lalu mencoba berulang kali. Ini tidak membocorkan apa pun:
	// surelnya pergi ke pemilik alamat itu, bukan ke yang mengetiknya, dan
	// jawaban HTTP-nya tetap sama (§A14-5).
	if st := u.Status(); st != contracts.StatusAktif {
		sebab := "masih menunggu persetujuan admin. Anda akan menerima surel begitu\n" +
			"admin memutuskan; setelah itu Anda bisa masuk atau mengatur ulang sandi."
		if st == contracts.StatusNonaktif {
			sebab = "sedang dinonaktifkan admin. Hubungi admin untuk mengaktifkannya kembali."
		}
		s.Mandiri.Surel.Masukkan(surel.Surat{
			Ke:     email,
			Subjek: subjekBelumAktif,
			Teks: "Halo " + u.FullName + ",\n\n" +
				"Ada permintaan atur ulang sandi untuk akun Aset IT \"" + u.Username + "\",\n" +
				"tetapi akun itu " + sebab + "\n\n" +
				"Tidak ada kode yang dikirim, dan sandi Anda tidak berubah.\n",
		})
		return
	}

	kode, err := s.Store.BuatReset(u.ID, time.Now())
	if err != nil {
		log.Printf("auth: membuat kode setel ulang %s: %v", u.Username, err)
		return
	}

	s.Mandiri.Surel.Masukkan(surel.Surat{
		Ke:     email,
		Subjek: subjekOTP,
		Teks: "Halo " + u.FullName + ",\n\n" +
			"Seseorang meminta setel ulang sandi untuk akun Aset IT \"" + u.Username + "\".\n\n" +
			"Kode Anda: " + kode + "\n\n" +
			"Kode berlaku 10 menit dan hanya bisa dipakai sekali. Bila Anda tidak\n" +
			"memintanya, abaikan surel ini; sandi Anda tidak berubah.\n",
	})

	detail := `{"kanal":"email"}`
	ip := ""
	if err := s.Store.Audit(&u.Ref, &u.Username, "setel_ulang_diminta", "users", &u.Ref, &detail, &ip); err != nil {
		log.Printf("auth: menulis audit setel_ulang_diminta: %v", err)
	}
}

// HandleSetelUlangSandi menjawab POST /api/auth/setel-ulang-sandi (publik).
func (s *Server) HandleSetelUlangSandi(w http.ResponseWriter, r *http.Request) {
	if s.Mandiri.Surel == nil {
		tulisGalat(w, http.StatusServiceUnavailable, pesanSurelMati, nil)
		return
	}

	var in contracts.PermintaanSetelUlang
	if err := baca(w, r, &in); err != nil {
		tulisGalat(w, http.StatusBadRequest, "badan permintaan bukan JSON yang sah", nil)
		return
	}
	if len(in.SandiBaru) < SandiMinimum {
		tulisGalat(w, http.StatusBadRequest, "sandi baru tidak sah",
			map[string]string{"sandi_baru": "sandi minimal 12 karakter"})
		return
	}

	if !s.lolosLaju(w, s.GembokLupaIP, "setel-ip:"+alamat(r)) {
		return
	}

	u, err := s.Store.PakaiReset(in.Email, strings.TrimSpace(in.Kode), in.SandiBaru, time.Now())
	switch {
	case errors.Is(err, ErrKodeTidakSah):
		tulisGalat(w, http.StatusBadRequest, ErrKodeTidakSah.Error(), nil)
		return
	case err != nil:
		httpx.GalatInternal(w, "auth", "permintaan gagal", err)
		return
	}

	// Pemilik yang terbukti memegang emailnya tidak perlu menunggu gembok
	// masuk yang mungkin dipicu penyerang.
	s.Gembok.Bersihkan(u.Username)
	s.audit(r, u, "sandi_disetel_ulang_otp", nil)

	if u.Email != nil {
		s.Mandiri.Surel.Masukkan(surel.Surat{
			Ke:     *u.Email,
			Subjek: subjekSandiDiganti,
			Teks: "Sandi akun Aset IT \"" + u.Username + "\" baru saja diganti lewat kode email,\n" +
				"dan seluruh sesi yang masih terbuka sudah diputus.\n\n" +
				"Bila bukan Anda yang melakukannya, segera hubungi admin.\n",
		})
	}
	httpx.WriteJSON(w, http.StatusOK, contracts.Pesan{Pesan: pesanSandiDiganti})
}

// HandleTamu menjawab POST /api/auth/tamu (publik).
func (s *Server) HandleTamu(w http.ResponseWriter, r *http.Request) {
	if s.Mandiri.PenandaTamu == nil {
		tulisGalat(w, http.StatusForbidden, pesanTamuMati, nil)
		return
	}

	acak, err := sandi.Acak(9)
	if err != nil {
		httpx.GalatInternal(w, "auth", "permintaan gagal", err)
		return
	}
	sub := awalanSubjekTamu + "_" + acak

	tok, sampai, err := s.Mandiri.PenandaTamu.Terbitkan(sub, contracts.PeranTamu, namaTampilTamu, idgen.New("jti"))
	if err != nil {
		httpx.GalatInternal(w, "auth", "permintaan gagal", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, contracts.SesiTamu{
		Token:       tok,
		Kedaluwarsa: sampai.Format(time.RFC3339),
		Peran:       contracts.PeranTamu,
	})
}

// AdminBerEmail mengembalikan admin aktif yang punya email.
func (s *Store) AdminBerEmail() ([]User, error) {
	var out []User
	err := s.DB.Where("role = 'admin' AND active = 1 AND email IS NOT NULL").Find(&out).Error
	return out, err
}

// kabariDisetujui mengabari pendaftar bahwa akunnya aktif. Upaya terbaik:
// persetujuan tetap sah walau surelnya gagal.
func (s *Server) kabariDisetujui(u *User) {
	if s.Mandiri.Surel == nil || u.Email == nil {
		return
	}
	s.Mandiri.Surel.Masukkan(surel.Surat{
		Ke:     *u.Email,
		Subjek: subjekAkunDisetujui,
		Teks: "Halo " + u.FullName + ",\n\n" +
			"Akun Aset IT \"" + u.Username + "\" sudah disetujui dengan peran " + u.Role + ".\n" +
			"Anda sudah bisa masuk.\n",
	})
}

// lolosLaju mencatat satu permintaan pada pembatas dan menjawab 429 bila
// batasnya sudah tercapai.
func (s *Server) lolosLaju(w http.ResponseWriter, g *Gembok, kunci string) bool {
	if terkunci, sisa := g.Terkunci(kunci); terkunci {
		w.Header().Set("Retry-After", strconv.Itoa(int(sisa.Seconds())+1))
		tulisGalat(w, http.StatusTooManyRequests, pesanTerlaluSering, nil)
		return false
	}
	g.Gagal(kunci)
	return true
}
