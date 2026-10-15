// Package sandi menyandikan dan memeriksa kata sandi, serta membangkitkan
// rahasia acak.
//
// Seluruh bahannya berasal dari pustaka standar (SPEK-07 §A7-4): bcrypt akan
// menuntut golang.org/x/crypto, dan CLAUDE.md aturan 3 melarang dependensi baru.
package sandi

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// Putaran adalah jumlah putaran PBKDF2 untuk sandi baru.
	// 600.000 mengikuti anjuran OWASP 2023 untuk PBKDF2-HMAC-SHA256.
	Putaran = 600000

	// algoritma adalah penanda di awal bentuk simpan. Ia ikut tersimpan supaya
	// algoritma bisa diganti kelak tanpa membatalkan sandi lama.
	algoritma = "pbkdf2_sha256"

	panjangGaram   = 16
	panjangTurunan = 32
)

// ErrBentukSalah dikembalikan bila string tersimpan tidak berbentuk
// "pbkdf2_sha256$<putaran>$<garam>$<turunan>".
var ErrBentukSalah = errors.New("bentuk sandi tersimpan tidak dikenali")

var pengkode = base64.RawStdEncoding

// PutaranAktif adalah putaran yang benar-benar dipakai Sandikan.
//
// Ia variabel, bukan konstanta, SEMATA agar berkas uji bisa menurunkannya:
// 600.000 putaran membuat satu paket uji memakan puluhan detik tanpa menguji
// apa pun yang baru.
//
// Kode produksi TIDAK BOLEH mengubahnya. Larangan itu ditegakkan
// internal/arch/arch_test.go, bukan sekadar ditulis di sini.
var PutaranAktif = Putaran

// Sandikan mengembalikan bentuk simpan untuk sandi mentah.
func Sandikan(sandiMentah string) (string, error) {
	garam := make([]byte, panjangGaram)
	if _, err := rand.Read(garam); err != nil {
		return "", fmt.Errorf("membangkitkan garam: %w", err)
	}

	turunan, err := pbkdf2.Key(sha256.New, sandiMentah, garam, PutaranAktif, panjangTurunan)
	if err != nil {
		return "", fmt.Errorf("menurunkan kunci: %w", err)
	}

	return strings.Join([]string{
		algoritma,
		strconv.Itoa(PutaranAktif),
		pengkode.EncodeToString(garam),
		pengkode.EncodeToString(turunan),
	}, "$"), nil
}

// Periksa membandingkan sandi mentah dengan bentuk simpan.
//
// perluDisandikanUlang bernilai true bila putaran tersimpan lebih rendah dari
// Putaran saat ini, sehingga pemanggil bisa memperbarui diam-diam ketika
// pemiliknya berhasil masuk.
func Periksa(sandiMentah, tersimpan string) (cocok bool, perluDisandikanUlang bool, err error) {
	bagian := strings.Split(tersimpan, "$")
	if len(bagian) != 4 || bagian[0] != algoritma {
		return false, false, ErrBentukSalah
	}

	putaran, err := strconv.Atoi(bagian[1])
	if err != nil || putaran < 1 {
		return false, false, ErrBentukSalah
	}

	garam, err := pengkode.DecodeString(bagian[2])
	if err != nil {
		return false, false, ErrBentukSalah
	}
	mau, err := pengkode.DecodeString(bagian[3])
	if err != nil {
		return false, false, ErrBentukSalah
	}

	dapat, err := pbkdf2.Key(sha256.New, sandiMentah, garam, putaran, len(mau))
	if err != nil {
		return false, false, fmt.Errorf("menurunkan kunci: %w", err)
	}

	// ConstantTimeCompare, bukan bytes.Equal: waktu perbandingan tidak boleh
	// memberi tahu berapa banyak byte awal yang sudah benar.
	cocok = subtle.ConstantTimeCompare(dapat, mau) == 1

	return cocok, cocok && putaran < PutaranAktif, nil
}

// Rata menjalankan perhitungan sebanding tanpa membandingkan apa pun.
//
// Dipakai ketika nama pengguna tidak ditemukan. Tanpa ini, login yang gagal
// cepat berarti "nama ini tidak ada" dan yang gagal lambat berarti "nama ada,
// sandi salah" — daftar pengguna bocor hanya dari stopwatch (SPEK-07 §A7-5).
func Rata() {
	garam := make([]byte, panjangGaram)
	_, _ = pbkdf2.Key(sha256.New, "sandi-palsu-untuk-meratakan-waktu", garam, PutaranAktif, panjangTurunan)
}

// Acak mengembalikan n byte acak sebagai base64url tanpa padding.
func Acak(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("membangkitkan nilai acak: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SHA256Hex mengembalikan SHA-256 heksadesimal.
//
// Dipakai menyimpan token penyegar: yang tersimpan hanya turunannya, sehingga
// basis data yang bocor tidak berisi token yang bisa langsung dipakai.
func SHA256Hex(s string) string {
	jumlah := sha256.Sum256([]byte(s))
	return hex.EncodeToString(jumlah[:])
}
