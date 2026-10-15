// Package token menerbitkan dan memverifikasi token akses JWT HS256.
//
// Ditulis di atas pustaka standar, bukan pustaka JWT pihak ketiga
// (SPEK-07 §A7-1). Itu pertukaran yang disadari: menulis sendiri kode yang
// menyangkut keamanan berarti memikul sendiri kesalahannya. Tiga kesalahan
// klasik ditutup secara eksplisit di sini dan diuji satu per satu di
// token_test.go.
//
// Bila pemilik proyek lebih memilih pustaka mapan, penggantiannya terbatas
// pada berkas ini saja.
package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrTokenTidakSah menandai bentuk, tanda tangan, atau header yang salah.
	ErrTokenTidakSah = errors.New("token tidak sah")
	// ErrTokenKedaluwarsa dibedakan supaya klien tahu ia harus menyegarkan,
	// bukan meminta pengguna masuk ulang.
	ErrTokenKedaluwarsa = errors.New("token kedaluwarsa")
)

// panjangRahasiaMinimum adalah 32 byte: kunci HMAC yang lebih pendek dari
// keluaran SHA-256 memperkecil ruang tebak tanpa alasan.
const panjangRahasiaMinimum = 32

// headerTetap adalah SATU-SATUNYA header yang diterima.
//
// Ia dibandingkan sebagai byte, bukan diurai lalu diperiksa medannya. Dengan
// begitu tidak ada percabangan algoritma sama sekali, sehingga serangan
// "alg: none" dan kebingungan RS256→HS256 tidak punya jalan masuk.
var headerTetap = []byte(`{"alg":"HS256","typ":"JWT"}`)

// pengkode memakai mode Strict: base64 tanpa padding menyisakan bit tak
// terpakai di karakter terakhir (tanda tangan 32 byte = 43 karakter, 2 bit
// sisa). Tanpa Strict, bit itu diabaikan saat mengurai, sehingga satu token
// bisa ditulis dalam beberapa ejaan dan semuanya lolos verifikasi. Token harus
// punya tepat satu bentuk.
var pengkode = base64.RawURLEncoding.Strict()

// Klaim adalah muatan token akses. Nama medan JSON adalah bagian kontrak —
// jangan diubah tanpa SPEK-02 baru.
type Klaim struct {
	Sub   string `json:"sub"`   // ref pengguna, usr_<ULID>
	Peran string `json:"peran"` // admin|operator|auditor|viewer
	Nama  string `json:"nama"`  // nama pengguna, untuk log dan tampilan
	Iat   int64  `json:"iat"`   // detik Unix
	Exp   int64  `json:"exp"`   // detik Unix
	Jti   string `json:"jti"`   // pengenal token, untuk jejak audit
}

// Penanda menerbitkan dan memverifikasi token akses.
type Penanda struct {
	rahasia []byte
	umur    time.Duration

	// sekarang bisa diganti dalam uji supaya kedaluwarsa bisa diperiksa tanpa
	// menunggu waktu nyata.
	sekarang func() time.Time
}

// NewPenanda mengembalikan galat bila rahasia lebih pendek dari 32 byte atau
// umur bukan bilangan positif.
func NewPenanda(rahasia string, umur time.Duration) (*Penanda, error) {
	if len(rahasia) < panjangRahasiaMinimum {
		return nil, fmt.Errorf("rahasia penandatangan harus minimal %d karakter, dapat %d",
			panjangRahasiaMinimum, len(rahasia))
	}
	if umur <= 0 {
		return nil, errors.New("umur token harus lebih dari nol")
	}
	return &Penanda{rahasia: []byte(rahasia), umur: umur, sekarang: time.Now}, nil
}

// Umur mengembalikan masa berlaku token yang diterbitkan Penanda ini.
func (p *Penanda) Umur() time.Duration { return p.umur }

// Terbitkan mengembalikan token akses beserta waktu kedaluwarsanya.
func (p *Penanda) Terbitkan(sub, peran, nama, jti string) (string, time.Time, error) {
	sekarang := p.sekarang().UTC()
	kedaluwarsa := sekarang.Add(p.umur)

	muatan, err := json.Marshal(Klaim{
		Sub:   sub,
		Peran: peran,
		Nama:  nama,
		Iat:   sekarang.Unix(),
		Exp:   kedaluwarsa.Unix(),
		Jti:   jti,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("menyusun muatan token: %w", err)
	}

	badan := pengkode.EncodeToString(headerTetap) + "." + pengkode.EncodeToString(muatan)
	return badan + "." + pengkode.EncodeToString(p.tandaTangan(badan)), kedaluwarsa, nil
}

// Verifikasi mengurai dan memeriksa token.
//
// Urutannya disengaja: bentuk dan header diperiksa lebih dulu, tanda tangan
// berikutnya, dan muatan baru diurai setelah tanda tangannya terbukti. Muatan
// yang belum terbukti tidak boleh menyentuh pengurai JSON.
func (p *Penanda) Verifikasi(tokenMentah string) (*Klaim, error) {
	segmen := strings.Split(tokenMentah, ".")
	if len(segmen) != 3 {
		return nil, ErrTokenTidakSah
	}

	// Header dibandingkan byte per byte terhadap satu-satunya header yang sah.
	header, err := pengkode.DecodeString(segmen[0])
	if err != nil || !hmac.Equal(header, headerTetap) {
		return nil, ErrTokenTidakSah
	}

	tandaTangan, err := pengkode.DecodeString(segmen[2])
	if err != nil {
		return nil, ErrTokenTidakSah
	}

	// hmac.Equal, bukan ==: perbandingan tidak boleh bocor lewat waktu.
	if !hmac.Equal(tandaTangan, p.tandaTangan(segmen[0]+"."+segmen[1])) {
		return nil, ErrTokenTidakSah
	}

	muatan, err := pengkode.DecodeString(segmen[1])
	if err != nil {
		return nil, ErrTokenTidakSah
	}

	var k Klaim
	if err := json.Unmarshal(muatan, &k); err != nil {
		return nil, ErrTokenTidakSah
	}

	if k.Sub == "" || k.Peran == "" || k.Exp == 0 {
		return nil, ErrTokenTidakSah
	}

	sekarang := p.sekarang().UTC().Unix()
	if sekarang >= k.Exp {
		return nil, ErrTokenKedaluwarsa
	}
	// Token yang mengaku diterbitkan di masa depan menandakan jam yang tidak
	// sinkron atau muatan yang dikarang. Satu menit toleransi.
	if k.Iat > sekarang+60 {
		return nil, ErrTokenTidakSah
	}

	return &k, nil
}

func (p *Penanda) tandaTangan(badan string) []byte {
	mac := hmac.New(sha256.New, p.rahasia)
	mac.Write([]byte(badan))
	return mac.Sum(nil)
}
