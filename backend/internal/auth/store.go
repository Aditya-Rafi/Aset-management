package auth

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/contracts"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/idgen"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/sandi"
)

// polaNamaPengguna membatasi nama pengguna pada huruf kecil ASCII, angka, dan
// tiga tanda baca.
//
// Batasan ini menutup dua hal sekaligus: nama yang hanya berbeda besar-kecil
// huruf, dan nama yang terlihat sama tetapi hurufnya berbeda — "admın" dengan
// i tanpa titik akan tampil nyaris identik dengan "admin" di jejak audit.
// Untuk sistem yang keluarannya laporan audit, identitas yang ambigu di
// audit_log adalah cacat, bukan sekadar kejanggalan.
var polaNamaPengguna = regexp.MustCompile(`^[a-z0-9._-]{3,64}$`)

// NormalkanNamaPengguna memangkas spasi dan menurunkan ke huruf kecil.
//
// Dipakai di SETIAP tempat nama pengguna masuk atau dicari — pembuatan,
// perubahan, pencarian saat masuk, dan kunci gembok — supaya satu orang selalu
// berarti satu nama.
func NormalkanNamaPengguna(nama string) string {
	return strings.ToLower(strings.TrimSpace(nama))
}

// NamaPenggunaSah memeriksa bentuk nama pengguna yang SUDAH dinormalkan.
func NamaPenggunaSah(nama string) bool {
	return polaNamaPengguna.MatchString(nama)
}

// PeranSah adalah daftar peran yang dikenali, sama persis dengan CHECK pada
// 0001_init.sql. Dua daftar ini harus selalu sejalan.
var PeranSah = []string{"admin", "operator", "auditor", "viewer"}

var (
	// ErrTidakDitemukan dikembalikan bila pengguna atau token tidak ada.
	ErrTidakDitemukan = errors.New("tidak ditemukan")
	// ErrNamaTerpakai dikembalikan bila nama pengguna sudah ada.
	ErrNamaTerpakai = errors.New("nama pengguna sudah terpakai")
	// ErrEmailTerpakai dikembalikan bila email sudah dipakai akun lain.
	ErrEmailTerpakai = errors.New("email sudah terpakai")
)

// Store adalah akses basis data domain auth.
type Store struct{ DB *gorm.DB }

func sekarangRFC() string { return time.Now().UTC().Format(time.RFC3339) }

// KeKontrak mengubah baris users menjadi bentuk yang keluar lewat API.
// Sandi dan turunannya tidak pernah ikut.
func KeKontrak(u User) contracts.Pengguna {
	return contracts.Pengguna{
		Ref:           u.Ref,
		NamaPengguna:  u.Username,
		NamaLengkap:   u.FullName,
		Peran:         u.Role,
		Aktif:         u.Active,
		Email:         teks(u.Email),
		Status:        u.Status(),
		DisetujuiPada: u.ApprovedAt,
		MasukTerakhir: u.LastLoginAt,
		DibuatPada:    u.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// AmbilPenggunaAktif mengembalikan pengguna aktif berdasarkan nama pengguna.
// Pengguna nonaktif diperlakukan seolah tidak ada.
func (s *Store) AmbilPenggunaAktif(namaPengguna string) (*User, error) {
	var u User
	err := s.DB.Where("username = ? AND active = 1", NormalkanNamaPengguna(namaPengguna)).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTidakDitemukan
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// AmbilPenggunaRef mengembalikan pengguna berdasarkan ref, aktif maupun tidak.
func (s *Store) AmbilPenggunaRef(ref string) (*User, error) {
	var u User
	err := s.DB.Where("ref = ?", ref).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTidakDitemukan
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// AmbilPenggunaID mengembalikan pengguna berdasarkan id internal.
func (s *Store) AmbilPenggunaID(id uint) (*User, error) {
	var u User
	err := s.DB.Where("id = ?", id).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTidakDitemukan
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// DaftarPengguna mengembalikan seluruh pengguna, urut nama.
func (s *Store) DaftarPengguna() ([]User, error) {
	daftar := []User{}
	err := s.DB.Order("username").Find(&daftar).Error
	return daftar, err
}

// JumlahPengguna dipakai bootstrap untuk memutuskan perlu tidaknya admin pertama.
func (s *Store) JumlahPengguna() (int64, error) {
	var n int64
	err := s.DB.Model(&User{}).Count(&n).Error
	return n, err
}

// BuatPengguna menyimpan pengguna baru beserta sandi yang sudah disandikan.
func (s *Store) BuatPengguna(in contracts.PenggunaBaru) (*User, error) {
	hash, err := sandi.Sandikan(*in.Sandi)
	if err != nil {
		return nil, err
	}

	waktu := sekarangRFC()
	u := &User{
		Ref:          idgen.New(RefPrefix),
		Username:     NormalkanNamaPengguna(*in.NamaPengguna),
		PasswordHash: hash,
		FullName:     strings.TrimSpace(nilaiAtau(in.NamaLengkap, *in.NamaPengguna)),
		Role:         nilaiAtau(in.Peran, "viewer"),
		Active:       true,
		// Akun yang dibuat admin sudah disetujui sejak lahir (SPEK-14 §A14-1).
		ApprovedAt: &waktu,
		Email:      emailAtauNil(in.Email),
	}
	if in.Aktif != nil {
		u.Active = *in.Aktif
	}

	if err := s.DB.Create(u).Error; err != nil {
		return nil, galatKembar(err)
	}
	return u, nil
}

// UbahPengguna menerapkan perubahan sebagian. Medan yang nil tidak disentuh.
//
// Mengaktifkan akun yang masih menunggu adalah PERSETUJUAN: approved_at dan
// approved_by_ref diisi sekali dan tidak pernah ditimpa (SPEK-14 §A14-2).
func (s *Store) UbahPengguna(ref string, in contracts.PenggunaBaru, olehRef string) (*User, error) {
	u, err := s.AmbilPenggunaRef(ref)
	if err != nil {
		return nil, err
	}

	if in.NamaPengguna != nil {
		u.Username = NormalkanNamaPengguna(*in.NamaPengguna)
	}
	if in.NamaLengkap != nil {
		u.FullName = strings.TrimSpace(*in.NamaLengkap)
	}
	if in.Peran != nil {
		u.Role = *in.Peran
	}
	if in.Aktif != nil {
		u.Active = *in.Aktif
		if u.Active && u.ApprovedAt == nil {
			kini := sekarangRFC()
			u.ApprovedAt = &kini
			if olehRef != "" {
				u.ApprovedByRef = &olehRef
			}
		}
	}
	if in.Email != nil {
		u.Email = emailAtauNil(in.Email)
	}
	if in.Sandi != nil {
		hash, err := sandi.Sandikan(*in.Sandi)
		if err != nil {
			return nil, err
		}
		u.PasswordHash = hash
	}
	u.UpdatedAt = time.Now()

	if err := s.DB.Save(u).Error; err != nil {
		return nil, galatKembar(err)
	}
	return u, nil
}

// HapusPengguna menghapus baris pengguna. Token penyegarnya ikut terhapus
// lewat ON DELETE CASCADE.
func (s *Store) HapusPengguna(ref string) error {
	hasil := s.DB.Where("ref = ?", ref).Delete(&User{})
	if hasil.Error != nil {
		return hasil.Error
	}
	if hasil.RowsAffected == 0 {
		return ErrTidakDitemukan
	}
	return nil
}

// PerbaruiSandi dipakai saat sandi lama perlu disandikan ulang dengan putaran
// yang lebih tinggi (SPEK-07 §A7-4).
func (s *Store) PerbaruiSandi(id uint, hash string) error {
	return s.DB.Model(&User{}).Where("id = ?", id).
		Updates(map[string]any{"password": hash, "updated_at": time.Now()}).Error
}

// CatatMasuk memperbarui waktu masuk terakhir.
func (s *Store) CatatMasuk(id uint) error {
	return s.DB.Model(&User{}).Where("id = ?", id).
		Update("last_login_at", sekarangRFC()).Error
}

// SimpanPenyegar menyimpan SHA-256 token penyegar, bukan tokennya.
func (s *Store) SimpanPenyegar(userID uint, hash, sampai string) error {
	return s.DB.Create(&RefreshToken{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: sampai,
		CreatedAt: sekarangRFC(),
	}).Error
}

// AmbilPenyegar mencari token penyegar berdasarkan turunannya.
func (s *Store) AmbilPenyegar(hash string) (*RefreshToken, error) {
	var t RefreshToken
	err := s.DB.Where("token_hash = ?", hash).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTidakDitemukan
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// SandiMinimum adalah panjang sandi terpendek yang diterima di mana pun:
// bootstrap, pemulihan admin, pendaftaran, dan setel ulang lewat OTP.
const SandiMinimum = 12

// ErrSandiPendek dikembalikan bila sandi di bawah SandiMinimum.
var ErrSandiPendek = errors.New("sandi minimal 12 karakter")

// SetelUlangSandi mengganti sandi satu pengguna dan MENCABUT seluruh sesinya,
// dalam satu transaksi.
//
// Mencabut sesi bukan tambahan opsional. Sandi biasanya disetel ulang karena
// ada yang salah — sandi bocor, perangkat hilang, orang keluar dari kantor —
// dan token penyegar yang sudah beredar tetap berlaku 7 hari bila tidak
// dicabut. Menyetel ulang sandi tanpa mencabutnya hanya mengunci pintu depan
// sementara jendela terbuka.
//
// Pengguna yang tidak ditemukan adalah GALAT, bukan keberhasilan diam-diam.
// Versi sebelumnya melapor "berhasil" walau tidak satu baris pun berubah,
// sehingga salah ketik nama pengguna tampak seperti pemulihan yang berhasil.
func (s *Store) SetelUlangSandi(namaPengguna, sandiBaru string) (*User, error) {
	if len(sandiBaru) < SandiMinimum {
		return nil, ErrSandiPendek
	}

	hash, err := sandi.Sandikan(sandiBaru)
	if err != nil {
		return nil, err
	}

	nama := NormalkanNamaPengguna(namaPengguna)
	var u User

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("username = ?", nama).First(&u).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTidakDitemukan
			}
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]any{
			"password":   hash,
			"updated_at": time.Now(),
		}).Error; err != nil {
			return err
		}

		return tx.Model(&RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", u.ID).
			Update("revoked_at", sekarangRFC()).Error
	})
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// CabutPenyegar menandai satu token sebagai dicabut dan mengembalikan apakah
// pemanggil inilah yang mencabutnya.
//
// Syarat `revoked_at IS NULL` bukan hiasan: ia yang membuat pencabutan menjadi
// operasi yang hanya bisa menang sekali. Tanpa itu, dua permintaan yang tiba
// bersamaan sama-sama melihat token "belum dicabut" pada SELECT masing-masing,
// lalu sama-sama menerbitkan sesi baru — dan pemakaian ulang tidak pernah
// terdeteksi. Itu persis skenario yang membuat rotasi ada (§A7-3).
//
// Token tidak dihapus supaya pemakaian ulangnya tetap bisa dikenali.
func (s *Store) CabutPenyegar(id uint) (menang bool, err error) {
	hasil := s.DB.Model(&RefreshToken{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", sekarangRFC())
	return hasil.RowsAffected == 1, hasil.Error
}

// CabutSeluruhPenyegar mencabut seluruh token penyegar milik satu pengguna.
func (s *Store) CabutSeluruhPenyegar(userID uint) (int64, error) {
	hasil := s.DB.Model(&RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", sekarangRFC())
	return hasil.RowsAffected, hasil.Error
}

// Audit menulis satu baris jejak audit.
//
// Kegagalan menulis audit tidak boleh menggagalkan operasi yang sedang berjalan;
// pemanggil mencatatnya ke log dan melanjutkan.
func (s *Store) Audit(userRef, namaPengguna *string, aksi, entitas string, entitasRef, detail, ip *string) error {
	e := entitas
	return s.DB.Create(&AuditLog{
		UserRef:    userRef,
		Username:   namaPengguna,
		Action:     aksi,
		Entity:     &e,
		EntityRef:  entitasRef,
		DetailJSON: detail,
		IP:         ip,
		CreatedAt:  sekarangRFC(),
	}).Error
}

func nilaiAtau(p *string, def string) string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return def
	}
	return *p
}

// galatKembar menerjemahkan pelanggaran indeks unik menjadi galat domain.
func galatKembar(err error) error {
	if !namaKembar(err) {
		return err
	}
	if strings.Contains(strings.ToLower(err.Error()), "email") {
		return ErrEmailTerpakai
	}
	return ErrNamaTerpakai
}

// emailAtauNil menormalkan email; string kosong berarti "tidak ada email".
func emailAtauNil(p *string) *string {
	if p == nil {
		return nil
	}
	e := NormalkanEmail(*p)
	if e == "" {
		return nil
	}
	return &e
}

func teks(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// namaKembar mengenali pelanggaran indeks unik SQLite maupun MySQL.
func namaKembar(err error) bool { return models.IsDuplicateKey(err) }
