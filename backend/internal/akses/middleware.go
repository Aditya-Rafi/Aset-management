package akses

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/auth"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/contracts"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/token"
)

// Header identitas yang hanya boleh diisi middleware ini setelah token
// terverifikasi (SPEK-07 §A7-6). Dihapus lebih dulu dari permintaan masuk:
// tanpa itu siapa pun bisa mengaku admin hanya dengan mengirim headernya.
const (
	HeaderPengguna = auth.HeaderPengguna
	HeaderPeran    = auth.HeaderPeran
	HeaderNama     = "X-Aset-Nama"
	HeaderIPKlien  = "X-Real-IP"
)

// Identitas adalah siapa yang memanggil, dibaca dari token yang sudah terverifikasi.
type Identitas struct {
	Ref         string
	Peran       string
	Nama        string
	Kedaluwarsa time.Time

	db    *gorm.DB
	once  sync.Once
	baris *models.User
}

// Pengguna mengembalikan baris pengguna untuk identitas ini, dicari sekali dan
// hanya bila handler memintanya. Nil untuk token tamu atau akun yang sudah
// dihapus setelah token terbit.
func (i *Identitas) Pengguna() *models.User {
	if i == nil || i.db == nil {
		return nil
	}
	i.once.Do(func() {
		var u models.User
		if err := i.db.Where("ref = ?", i.Ref).First(&u).Error; err == nil {
			i.baris = &u
		}
	})
	return i.baris
}

type kunciIdentitas struct{}

// Dari mengembalikan identitas permintaan ini; nil bila belum terautentikasi.
func Dari(r *http.Request) *Identitas {
	i, _ := r.Context().Value(kunciIdentitas{}).(*Identitas)
	return i
}

// Penjaga memegang yang dibutuhkan middleware.
type Penjaga struct {
	DB      *gorm.DB
	Penanda *token.Penanda
	Proksi  *Proksi
}

// Middleware memverifikasi token akses, menegakkan matriks, lalu menyuntikkan
// identitas untuk handler di belakangnya.
func (p *Penjaga) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del(HeaderPengguna)
		r.Header.Del(HeaderPeran)
		r.Header.Del(HeaderNama)
		// Header yang dinamai di Connection dianggap hop-by-hop dan dibuang
		// penerus; klien tidak boleh bisa membuang identitas yang baru
		// disuntikkan (SPEK-07 §A7-13).
		bersihkanConnection(r)

		// Alamat klien ditetapkan di sini, bukan dikirim klien (SPEK-14 §A14-10).
		p.Proksi.TetapkanIPKlien(r)

		putusan := Izinkan(r.Method, r.URL.Path, "")
		if !putusan.Dikenal {
			tulisGalat(w, http.StatusNotFound, "rute tidak dikenal")
			return
		}
		if putusan.Publik {
			next.ServeHTTP(w, r)
			return
		}

		mentah, ok := ambilBearer(r)
		if !ok {
			tulisGalat(w, http.StatusUnauthorized, "butuh autentikasi")
			return
		}
		klaim, err := p.Penanda.Verifikasi(mentah)
		if err != nil {
			// Kedaluwarsa dibedakan supaya klien tahu harus menyegarkan, bukan
			// meminta pengguna masuk ulang.
			if errors.Is(err, token.ErrTokenKedaluwarsa) {
				tulisGalat(w, http.StatusUnauthorized, "token kedaluwarsa")
				return
			}
			tulisGalat(w, http.StatusUnauthorized, "token tidak sah")
			return
		}
		if !Izinkan(r.Method, r.URL.Path, klaim.Peran).Boleh {
			// 403, bukan 401: ia sudah dikenali, hanya tidak berhak.
			tulisGalat(w, http.StatusForbidden, "peran "+klaim.Peran+" tidak berhak atas tindakan ini")
			return
		}

		r.Header.Set(HeaderPengguna, klaim.Sub)
		r.Header.Set(HeaderPeran, klaim.Peran)
		r.Header.Set(HeaderNama, klaim.Nama)
		id := &Identitas{Ref: klaim.Sub, Peran: klaim.Peran, Nama: klaim.Nama, Kedaluwarsa: time.Unix(klaim.Exp, 0), db: p.DB}
		// Resolved here, not lazily in handlers: a handler inside a transaction
		// would otherwise ask for a second connection and deadlock on SQLite.
		if klaim.Peran != PeranTamu {
			id.Pengguna()
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), kunciIdentitas{}, id)))
	})
}

var headerIdentitas = []string{HeaderPengguna, HeaderPeran, HeaderNama}

// bersihkanConnection membuang penyebutan header identitas dari Connection.
func bersihkanConnection(r *http.Request) {
	nilai := r.Header.Get("Connection")
	if nilai == "" {
		return
	}
	sisa := make([]string, 0, 4)
	for _, t := range strings.Split(nilai, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		milikKami := false
		for _, h := range headerIdentitas {
			if strings.EqualFold(t, h) {
				milikKami = true
				break
			}
		}
		if !milikKami {
			sisa = append(sisa, t)
		}
	}
	if len(sisa) == 0 {
		r.Header.Del("Connection")
		return
	}
	r.Header.Set("Connection", strings.Join(sisa, ", "))
}

// ambilBearer mengambil token dari header Authorization. Hanya bentuk
// "Bearer <token>" yang dikenali; bentuk lain tidak ditebak-tebak.
func ambilBearer(r *http.Request) (string, bool) {
	nilai := r.Header.Get("Authorization")
	if nilai == "" {
		return "", false
	}
	bagian := strings.SplitN(nilai, " ", 2)
	if len(bagian) != 2 || !strings.EqualFold(bagian[0], "Bearer") {
		return "", false
	}
	tok := strings.TrimSpace(bagian[1])
	return tok, tok != ""
}

func tulisGalat(w http.ResponseWriter, kode int, pesan string) {
	httpx.WriteJSON(w, kode, contracts.Galat{Galat: pesan})
}

// Proksi mengenali proksi terpercaya di depan aplikasi (mis. nginx).
//
// Header X-Real-IP / X-Forwarded-For dari klien tidak boleh dipercaya begitu
// saja: pembatas laju per IP dan jejak audit bisa dikarang dengan mengganti
// header setiap permintaan. Header itu hanya dibaca bila rekan TCP-nya ada di
// daftar TRUSTED_PROXIES (alamat atau CIDR); selain itu yang dipakai adalah
// alamat TCP itu sendiri (SPEK-14 §A14-10).
type Proksi struct {
	jaringan []*net.IPNet
	alamat   []net.IP
}

// NewProksi membaca daftar dipisah koma, mis. "127.0.0.1,172.18.0.0/16".
func NewProksi(daftar string) (*Proksi, error) {
	p := &Proksi{}
	for _, e := range strings.Split(daftar, ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(e); err == nil {
			p.jaringan = append(p.jaringan, n)
			continue
		}
		ip := net.ParseIP(e)
		if ip == nil {
			return nil, errors.New("TRUSTED_PROXIES memuat alamat tidak sah: " + e)
		}
		p.alamat = append(p.alamat, ip)
	}
	return p, nil
}

func (p *Proksi) terpercaya(ip net.IP) bool {
	if p == nil || ip == nil {
		return false
	}
	for _, a := range p.alamat {
		if a.Equal(ip) {
			return true
		}
	}
	for _, n := range p.jaringan {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// TetapkanIPKlien menimpa X-Real-IP dengan alamat yang bisa dipertanggungjawabkan.
func (p *Proksi) TetapkanIPKlien(r *http.Request) {
	rekan, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		rekan = r.RemoteAddr
	}
	klien := rekan
	if p.terpercaya(net.ParseIP(rekan)) {
		cand := r.Header.Get(HeaderIPKlien)
		if cand == "" {
			cand = strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
		}
		if ip := net.ParseIP(cand); ip != nil {
			klien = ip.String()
		}
	}
	r.Header.Set(HeaderIPKlien, klien)
	r.Header.Del("X-Forwarded-For")
}
