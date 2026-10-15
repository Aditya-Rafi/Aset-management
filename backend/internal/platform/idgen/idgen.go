// Package idgen membangkitkan `ref` — identitas publik yang dipakai lintas
// basis data (SPEK-01 §A1-2).
//
// Bentuknya "<prefix>_<26 karakter Crockford base32>". Bagian setelah garis
// bawah adalah ULID: 48 bit milidetik Unix diikuti 80 bit acak, sehingga ref
// terurut kronologis ketika diurutkan sebagai teks biasa.
package idgen

import (
	"crypto/rand"
	"strings"
	"time"
)

// alfabet Crockford base32: tanpa I, L, O, dan U supaya tidak tertukar saat
// dibaca atau diketik ulang manusia dalam sengketa dengan provider.
const alfabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// panjangULID adalah jumlah karakter base32 untuk 128 bit.
const panjangULID = 26

// New mengembalikan ref baru dengan prefix yang diberikan.
func New(prefix string) string {
	var b [16]byte

	ms := uint64(time.Now().UTC().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)

	// rand.Read pada crypto/rand tidak pernah mengembalikan galat sejak Go 1.24;
	// ia panic bila sumber entropi sistem gagal, dan itu memang perilaku yang
	// diinginkan — ref yang bisa ditebak lebih buruk daripada proses berhenti.
	_, _ = rand.Read(b[6:])

	return prefix + "_" + encode(b)
}

// Valid mengembalikan true bila s berbentuk ref yang sah dengan prefix itu.
func Valid(prefix, s string) bool {
	awalan := prefix + "_"
	if !strings.HasPrefix(s, awalan) {
		return false
	}

	sisa := s[len(awalan):]
	if len(sisa) != panjangULID {
		return false
	}
	for _, r := range sisa {
		if !strings.ContainsRune(alfabet, r) {
			return false
		}
	}
	return true
}

// encode mengubah 128 bit menjadi 26 karakter Crockford base32.
// Karakter pertama hanya memuat 2 bit sisa (26*5 = 130 bit).
func encode(b [16]byte) string {
	out := make([]byte, panjangULID)

	// Diuraikan eksplisit alih-alih lewat big.Int supaya tidak ada alokasi
	// dan urutannya bisa dibaca langsung.
	var nilai uint64

	// 10 karakter pertama: 48 bit stempel waktu, digeser ke batas 50 bit.
	nilai = uint64(b[0])<<40 | uint64(b[1])<<32 | uint64(b[2])<<24 |
		uint64(b[3])<<16 | uint64(b[4])<<8 | uint64(b[5])
	for i := 9; i >= 0; i-- {
		out[i] = alfabet[nilai&31]
		nilai >>= 5
	}

	// 16 karakter berikutnya: 80 bit acak, dipecah dua kali 40 bit.
	nilai = uint64(b[6])<<32 | uint64(b[7])<<24 | uint64(b[8])<<16 |
		uint64(b[9])<<8 | uint64(b[10])
	for i := 17; i >= 10; i-- {
		out[i] = alfabet[nilai&31]
		nilai >>= 5
	}

	nilai = uint64(b[11])<<32 | uint64(b[12])<<24 | uint64(b[13])<<16 |
		uint64(b[14])<<8 | uint64(b[15])
	for i := 25; i >= 18; i-- {
		out[i] = alfabet[nilai&31]
		nilai >>= 5
	}

	return string(out)
}
