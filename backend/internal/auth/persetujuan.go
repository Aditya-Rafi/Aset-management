package auth

import (
	"encoding/base64"
	"errors"
	"html"
	"log"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/contracts"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/idgen"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/sandi"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/surel"
)

// umurTautan adalah masa berlaku tautan Terima/Tolak di surel admin.
// Lebih panjang dari OTP karena admin belum tentu membuka surel hari itu juga;
// yang membatasinya adalah ruang token 256 bit dan fakta bahwa ia hangus
// begitu pendaftar diputuskan (SPEK-14 §A14-12).
const umurTautan = 7 * 24 * time.Hour

// ErrTautanTidakSah mencakup semua sebab: tidak dikenal, kedaluwarsa, sudah
// dipakai, pendaftar sudah diputuskan, atau admin pemiliknya tak lagi admin.
var ErrTautanTidakSah = errors.New("tautan tidak sah, sudah kedaluwarsa, atau pengajuan sudah diputuskan")

// labelPeran menerjemahkan peran sistem ke pilihan di formulir pendaftaran.
var labelPeran = map[string]string{
	"operator": "Teknisi (operator)",
	"viewer":   "Manajemen (viewer)",
	"auditor":  "Auditor",
	"admin":    "Admin",
}

// ---------------------------------------------------------------- store

// BuatTautan membuat satu tautan persetujuan untuk pendaftar dan admin itu.
// Mengembalikan token mentahnya, satu-satunya saat token itu ada di luar surel.
func (s *Store) BuatTautan(userID uint, adminRef string, kini time.Time) (string, error) {
	mentah, err := sandi.Acak(32)
	if err != nil {
		return "", err
	}
	// Acak sudah heks; dikemas ulang base64url supaya tautan lebih pendek.
	token := base64.RawURLEncoding.EncodeToString([]byte(mentah))
	err = s.DB.Create(&ApprovalLink{
		Ref:       idgen.New("apv"),
		UserID:    userID,
		AdminRef:  adminRef,
		TokenHash: sandi.SHA256Hex(token),
		ExpiresAt: kini.UTC().Add(umurTautan).Format(time.RFC3339),
		CreatedAt: kini.UTC().Format(time.RFC3339),
	}).Error
	if err != nil {
		return "", err
	}
	return token, nil
}

// bacaTautan memeriksa token di dalam tx dan mengembalikan tautan, pendaftar,
// dan admin pemiliknya.
func bacaTautan(tx *gorm.DB, token string, kini time.Time) (*ApprovalLink, *User, *User, error) {
	if token == "" || len(token) > 128 {
		return nil, nil, nil, ErrTautanTidakSah
	}
	var t ApprovalLink
	if err := tx.Where("token_hash = ?", sandi.SHA256Hex(token)).Limit(1).Find(&t).Error; err != nil {
		return nil, nil, nil, err
	}
	if t.ID == 0 || t.UsedAt != nil || sudahLewatPada(t.ExpiresAt, kini) {
		return nil, nil, nil, ErrTautanTidakSah
	}

	var u User
	if err := tx.Where("id = ?", t.UserID).Limit(1).Find(&u).Error; err != nil {
		return nil, nil, nil, err
	}
	if u.ID == 0 || u.Status() != contracts.StatusMenunggu {
		return nil, nil, nil, ErrTautanTidakSah
	}

	// Tautan milik admin yang sudah diturunkan atau dinonaktifkan ikut mati:
	// kewenangannya berasal dari peran admin itu, bukan dari surelnya.
	var a User
	if err := tx.Where("ref = ? AND role = 'admin' AND active = 1", t.AdminRef).Limit(1).Find(&a).Error; err != nil {
		return nil, nil, nil, err
	}
	if a.ID == 0 {
		return nil, nil, nil, ErrTautanTidakSah
	}
	return &t, &u, &a, nil
}

// LihatTautan mengembalikan pendaftar yang ditunjuk token, tanpa mengubah apa pun.
func (s *Store) LihatTautan(token string, kini time.Time) (*User, *ApprovalLink, error) {
	t, u, _, err := bacaTautan(s.DB, token, kini)
	return u, t, err
}

// PutuskanTautan menerima atau menolak pendaftar dalam SATU transaksi, dan
// menghanguskan SEMUA tautan untuk pendaftar itu.
//
// Menerima mengaktifkan akun dengan peran yang dipilih. Menolak menghapus
// barisnya — sama dengan DELETE /api/pengguna/{ref}; jejaknya tetap di
// audit_log. Pendaftar yang ditolak tidak bisa masuk karena akunnya tidak ada.
func (s *Store) PutuskanTautan(token string, terima bool, peran string, kini time.Time) (*User, *User, error) {
	var u, a User
	sekarang := kini.UTC().Format(time.RFC3339)

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		_, pu, pa, err := bacaTautan(tx, token, kini)
		if err != nil {
			return err
		}
		u, a = *pu, *pa

		// used_at IS NULL: keputusan hanya bisa menang sekali (§A7-12).
		hasil := tx.Model(&ApprovalLink{}).
			Where("user_id = ? AND used_at IS NULL", u.ID).Update("used_at", sekarang)
		if hasil.Error != nil {
			return hasil.Error
		}
		if hasil.RowsAffected == 0 {
			return ErrTautanTidakSah
		}

		if !terima {
			return tx.Where("id = ?", u.ID).Delete(&User{}).Error
		}
		if peran == "" {
			peran = u.Role
		}
		u.Active = true
		u.Role = peran
		u.ApprovedAt = &sekarang
		u.ApprovedByRef = &a.Ref
		u.UpdatedAt = time.Now()
		return tx.Model(&User{}).Where("id = ?", u.ID).Updates(map[string]any{
			"active": true, "role": peran, "approved_at": sekarang,
			"approved_by_ref": a.Ref, "updated_at": time.Now(),
		}).Error
	})
	if err != nil {
		return nil, nil, err
	}
	return &u, &a, nil
}

// ---------------------------------------------------------------- handler

// HandleLihatPersetujuan menjawab POST /api/auth/persetujuan/lihat (publik;
// tokennya yang menjadi kewenangan).
//
// POST, bukan GET: token tidak boleh muncul di log akses, dan pemindai
// tautan di kotak surel tidak boleh bisa memicu apa pun.
func (s *Server) HandleLihatPersetujuan(w http.ResponseWriter, r *http.Request) {
	var in contracts.PermintaanTautan
	if err := baca(w, r, &in); err != nil {
		tulisGalat(w, http.StatusBadRequest, "badan permintaan bukan JSON yang sah", nil)
		return
	}
	if !s.lolosLaju(w, s.GembokLupaIP, "persetujuan-ip:"+alamat(r)) {
		return
	}
	u, t, err := s.Store.LihatTautan(in.Token, time.Now())
	switch {
	case errors.Is(err, ErrTautanTidakSah):
		tulisGalat(w, http.StatusNotFound, ErrTautanTidakSah.Error(), nil)
		return
	case err != nil:
		httpx.GalatInternal(w, "auth", "permintaan gagal", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, contracts.RincianPengajuan{
		NamaLengkap: u.FullName, Email: teks(u.Email), PeranDiminta: u.Role,
		DibuatPada: u.CreatedAt.UTC().Format(time.RFC3339), Kedaluwarsa: t.ExpiresAt,
	})
}

// HandlePutuskanPersetujuan menjawab POST /api/auth/persetujuan (publik).
func (s *Server) HandlePutuskanPersetujuan(w http.ResponseWriter, r *http.Request) {
	var in contracts.PermintaanPersetujuan
	if err := baca(w, r, &in); err != nil {
		tulisGalat(w, http.StatusBadRequest, "badan permintaan bukan JSON yang sah", nil)
		return
	}
	if in.Keputusan != contracts.KeputusanTerima && in.Keputusan != contracts.KeputusanTolak {
		tulisGalat(w, http.StatusBadRequest, "keputusan harus terima atau tolak", nil)
		return
	}
	if in.Peran != "" && !termasuk(PeranSah, in.Peran) {
		tulisGalat(w, http.StatusBadRequest, "peran tidak sah",
			map[string]string{"peran": "harus salah satu dari: " + strings.Join(PeranSah, ", ")})
		return
	}
	if !s.lolosLaju(w, s.GembokLupaIP, "persetujuan-ip:"+alamat(r)) {
		return
	}

	terima := in.Keputusan == contracts.KeputusanTerima
	u, a, err := s.Store.PutuskanTautan(in.Token, terima, in.Peran, time.Now())
	switch {
	case errors.Is(err, ErrTautanTidakSah):
		tulisGalat(w, http.StatusNotFound, ErrTautanTidakSah.Error(), nil)
		return
	case err != nil:
		httpx.GalatInternal(w, "auth", "permintaan gagal", err)
		return
	}

	ip := alamat(r)
	detail := `{"lewat":"tautan_surel","pendaftar":"` + u.Username + `"}`
	aksi := "pengguna_disetujui"
	if !terima {
		aksi = "pengguna_ditolak"
	}
	if err := s.Store.Audit(&a.Ref, &a.Username, aksi, "users", &u.Ref, &detail, &ip); err != nil {
		log.Printf("auth: menulis audit %s: %v", aksi, err)
	}

	if terima {
		s.kabariDisetujui(u)
		httpx.WriteJSON(w, http.StatusOK, contracts.HasilPersetujuan{
			Pesan: u.FullName + " disetujui dan sudah bisa masuk.", Status: contracts.StatusAktif, Peran: u.Role})
		return
	}
	s.kabariDitolak(u)
	httpx.WriteJSON(w, http.StatusOK, contracts.HasilPersetujuan{
		Pesan: "Pengajuan " + u.FullName + " ditolak; akun itu tidak bisa masuk.", Status: "ditolak"})
}

// ---------------------------------------------------------------- surel

// kabariPengajuan mengabari pendaftar bahwa pengajuannya diterima, dan setiap
// admin berEmail dengan tombol Terima/Tolak (SPEK-14 §A14-11, §A14-12).
// Upaya terbaik: pendaftaran tetap sah walau surelnya gagal.
func (s *Server) kabariPengajuan(u *User) {
	if s.Mandiri.Surel == nil || u.Email == nil {
		return
	}
	s.Mandiri.Surel.Masukkan(surel.Surat{
		Ke:     *u.Email,
		Subjek: subjekPengajuan,
		Teks: "Halo " + u.FullName + ",\n\n" +
			"Pengajuan akun Aset IT Anda sudah kami terima dan sedang menunggu\n" +
			"persetujuan admin. Anda akan menerima surel lagi begitu admin memutuskan.\n\n" +
			"Bila Anda tidak merasa mendaftar, abaikan surel ini; akun tidak aktif\n" +
			"sampai admin menyetujuinya.\n",
	})

	admin, err := s.Store.AdminBerEmail()
	if err != nil {
		log.Printf("auth: mencari admin untuk dikabari: %v", err)
		return
	}
	if len(admin) == 0 {
		log.Printf("auth: pengajuan %s tidak dikabarkan ke admin mana pun: tidak ada admin aktif yang punya email", u.Username)
		return
	}

	for _, a := range admin {
		token, err := s.Store.BuatTautan(u.ID, a.Ref, time.Now())
		if err != nil {
			log.Printf("auth: membuat tautan persetujuan: %v", err)
			continue
		}
		terima := s.Mandiri.URLPublik + "/persetujuan?t=" + token + "&aksi=" + contracts.KeputusanTerima
		tolak := s.Mandiri.URLPublik + "/persetujuan?t=" + token + "&aksi=" + contracts.KeputusanTolak
		peran := labelPeran[u.Role]

		s.Mandiri.Surel.Masukkan(surel.Surat{
			Ke:     *a.Email,
			Subjek: subjekPengajuanAdmin + ": " + u.FullName,
			Teks: "Ada pengajuan akun baru di Aset IT.\n\n" +
				"Nama         : " + u.FullName + "\n" +
				"Email        : " + *u.Email + "\n" +
				"Peran diminta: " + peran + "\n" +
				"Diajukan     : " + u.CreatedAt.UTC().Format(time.RFC3339) + " (UTC)\n\n" +
				"TERIMA: " + terima + "\n" +
				"TOLAK : " + tolak + "\n\n" +
				"Tautan berlaku 7 hari dan hangus begitu pengajuan diputuskan.\n",
			Html: htmlPengajuan(u.FullName, *u.Email, peran, u.CreatedAt.UTC().Format(time.RFC3339), terima, tolak),
		})
	}
}

// htmlPengajuan menyusun surel admin. Nama dan email berasal dari PENDAFTAR —
// orang yang belum dikenal — jadi keduanya di-escape: tanpa itu, siapa pun
// bisa menyisipkan tautan atau tombol palsu ke surel yang dibaca admin.
func htmlPengajuan(nama, email, peran, pada, terima, tolak string) string {
	e := html.EscapeString
	baris := func(label, nilai string) string {
		return `<tr><td style="padding:6px 16px 6px 0;color:#8a8f94;font:12px monospace;text-transform:uppercase;letter-spacing:.08em">` +
			label + `</td><td style="padding:6px 0;font:15px sans-serif;color:#1c1f22">` + e(nilai) + `</td></tr>`
	}
	tombol := func(href, teks, latar, warna, garis string) string {
		return `<a href="` + e(href) + `" style="display:inline-block;padding:12px 28px;margin-right:8px;background:` + latar +
			`;color:` + warna + `;border:1px solid ` + garis + `;font:600 14px sans-serif;text-decoration:none">` + teks + `</a>`
	}
	return `<!doctype html><html><body style="margin:0;background:#edeae3;padding:32px 16px">
<div style="max-width:520px;margin:0 auto;background:#faf9f6;border:1px solid #d6d1c7">
<div style="background:#1c1f22;color:#fff;padding:10px 20px;font:11px monospace;letter-spacing:.14em;text-transform:uppercase">Aset IT · pengajuan akun baru</div>
<div style="padding:24px 20px">
<p style="margin:0 0 16px;font:15px sans-serif;color:#4a4f55">Seseorang mengajukan akun dan menunggu keputusan Anda.</p>
<table style="border-collapse:collapse;margin-bottom:24px">` +
		baris("Nama", nama) + baris("Email", email) + baris("Peran diminta", peran) + baris("Diajukan (UTC)", pada) +
		`</table>` +
		tombol(terima, "Terima", "#1c1f22", "#ffffff", "#1c1f22") +
		tombol(tolak, "Tolak", "#ffffff", "#9c2b23", "#9c2b23") +
		`<p style="margin:24px 0 0;font:12px monospace;color:#8a8f94">Tombol membuka halaman konfirmasi; keputusan baru berlaku setelah Anda menekan tombol di sana. Tautan berlaku 7 hari dan hangus begitu pengajuan diputuskan.</p>
</div></div></body></html>`
}

// kabariDitolak mengabari pendaftar bahwa pengajuannya ditolak.
func (s *Server) kabariDitolak(u *User) {
	if s.Mandiri.Surel == nil || u.Email == nil {
		return
	}
	s.Mandiri.Surel.Masukkan(surel.Surat{
		Ke:     *u.Email,
		Subjek: subjekDitolak,
		Teks: "Halo " + u.FullName + ",\n\n" +
			"Pengajuan akun Aset IT Anda tidak disetujui admin, jadi akun ini tidak\n" +
			"bisa dipakai untuk masuk. Bila menurut Anda ini keliru, hubungi admin.\n",
	})
}
