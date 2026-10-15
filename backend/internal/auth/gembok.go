package auth

import (
	"sync"
	"time"
)

// Gembok menahan percobaan masuk beruntun untuk satu nama pengguna.
//
// Disimpan di memori proses dan hilang saat restart. Itu diterima: gembok ini
// menghambat serangan otomatis, bukan menjadi kebijakan yang harus awet
// (SPEK-07 §A7-5).
type Gembok struct {
	mu    sync.Mutex
	catat map[string]*jejak

	batas   int
	jendela time.Duration
	lama    time.Duration

	// sekarang bisa diganti dalam uji supaya jendela waktu bisa diperiksa
	// tanpa menunggu waktu nyata.
	sekarang func() time.Time
}

type jejak struct {
	gagal          int
	pertamaGagal   time.Time
	terkunciSampai time.Time
}

// NewGembok membuat gembok yang mengunci setelah batas kegagalan dalam jendela,
// selama lama.
func NewGembok(batas int, jendela, lama time.Duration) *Gembok {
	return &Gembok{
		catat:    map[string]*jejak{},
		batas:    batas,
		jendela:  jendela,
		lama:     lama,
		sekarang: time.Now,
	}
}

// Terkunci mengembalikan apakah kunci sedang terkunci beserta sisa waktunya.
func (g *Gembok) Terkunci(kunci string) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	j, ada := g.catat[kunci]
	if !ada {
		return false, 0
	}

	sekarang := g.sekarang()
	if sekarang.Before(j.terkunciSampai) {
		return true, j.terkunciSampai.Sub(sekarang)
	}

	// Kunci sudah lewat: catatan dibuang supaya hitungan mulai dari nol.
	if !j.terkunciSampai.IsZero() {
		delete(g.catat, kunci)
	}
	return false, 0
}

// Gagal mencatat satu kegagalan dan mengunci bila batas tercapai.
func (g *Gembok) Gagal(kunci string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	sekarang := g.sekarang()

	j, ada := g.catat[kunci]
	// Catatan yang jendelanya sudah lewat dimulai ulang: lima kegagalan
	// tersebar sepanjang hari bukan serangan.
	if !ada || sekarang.Sub(j.pertamaGagal) > g.jendela {
		if !ada && len(g.catat) >= batasCatatan {
			g.sapu(sekarang)
		}
		g.catat[kunci] = &jejak{gagal: 1, pertamaGagal: sekarang}
		return
	}

	j.gagal++
	if j.gagal >= g.batas {
		j.terkunciSampai = sekarang.Add(g.lama)
	}
}

// batasCatatan membatasi jumlah kunci di memori.
//
// Kuncinya berasal dari masukan pengguna — nama yang diketik, email yang
// diketik, alamat IP — dan versi pertama tidak pernah membuang catatan yang
// tidak sampai terkunci. Satu skrip yang mengirim nama acak bisa menumbuhkan
// peta ini sampai container auth (24m) kehabisan memori.
const batasCatatan = 10_000

// sapu membuang catatan yang jendelanya lewat dan tidak sedang terkunci. Bila
// peta masih penuh, catatan yang tidak terkunci dibuang sembarang: kehilangan
// hitungan satu kunci lebih baik daripada service yang mati.
// Pemanggil memegang g.mu.
func (g *Gembok) sapu(sekarang time.Time) {
	for k, j := range g.catat {
		if sekarang.Before(j.terkunciSampai) {
			continue
		}
		if sekarang.Sub(j.pertamaGagal) > g.jendela || !j.terkunciSampai.IsZero() {
			delete(g.catat, k)
		}
	}
	// Dibuang sampai 90%, bukan sampai tepat di bawah batas: membuang satu
	// per sisipan berarti menyapu seluruh peta pada SETIAP kunci baru, dan
	// biaya kuadrat itu sendiri menjadi jalan melumpuhkan service.
	sasaran := batasCatatan * 9 / 10
	for k, j := range g.catat {
		if len(g.catat) <= sasaran {
			return
		}
		if !sekarang.Before(j.terkunciSampai) {
			delete(g.catat, k)
		}
	}
}

// Bersihkan menghapus catatan setelah masuk berhasil.
func (g *Gembok) Bersihkan(kunci string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.catat, kunci)
}
