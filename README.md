# Aset IT

Aplikasi inventaris aset untuk **IT Support**: laptop, PC, monitor, handphone, sparepart, aksesoris — lengkap dengan
**stiker QR** yang bila di-scan dari HP langsung membuka data aset.

- **API**: Go (chi, GORM, JWT di cookie httpOnly) · **Database**: MySQL 8 atau SQLite
- **Frontend**: Vue 3 + TypeScript + Vite + PrimeVue — di produksi dilayani oleh satu binary Go

## Fitur

| | |
|---|---|
| **Inventaris** | Satu daftar untuk semua jenis barang. Form menyesuaikan kategori: laptop/PC menampilkan CPU, RAM, Storage, OS; monitor menampilkan ukuran layar; sparepart cukup merk/tipe/jumlah. Serial number, tahun pembuatan, tanggal & harga beli, vendor, **garansi**, lokasi, keterangan, hingga 5 foto. |
| **Status** | Tersedia · Dipakai · Diservis · Rusak · Dimusnahkan. Aset yang dipakai hanya berubah lewat Serah Terima, sehingga pemegangnya selalu jelas. |
| **Kode & stiker** | Kode otomatis per kategori: `LPT-0001`, `MON-0004`, `HP-0002`. Tiap aset punya **QR** (membuka halaman aset) dan barcode Code 128 (untuk scanner). Cetak label 1 per halaman (50×30 / 70×40 mm) atau lembar A4 3×8. |
| **Scan** | Kamera bawaan HP / Google Lens → halaman aset. Dalam aplikasi: menu *Scan Kode* (kamera, butuh HTTPS) atau ketik/scan USB di kolom pencarian atas dan tekan Enter dari halaman mana pun. |
| **Serah terima** | Tiga jenis dokumen untuk beberapa aset sekaligus: *Serah Terima* (IT → karyawan), *Pengembalian* (karyawan → IT), dan *Pindah Tangan* (karyawan → karyawan, mis. laptop bekas Adrian ke Mumu). Saat pengembalian tiap aset bisa menjadi Tersedia / Diservis / Rusak. Dokumen bisa dibatalkan dan status serta pemegang aset kembali persis seperti semula. |
| **Pemeriksaan fisik (preventif)** | Setiap kali aset berpindah tangan, IT Support mengisi checklist langsung di aplikasi: untuk laptop hardware (12 butir, termasuk kesehatan baterai) + software (10 butir) + kelengkapan, mengikuti form BAST kantor; untuk perangkat lain checklist singkat. Butir Tidak Baik wajib diberi keterangan. Jenis checklist diatur per kategori (menu Kategori). |
| **Tanggung jawab kerusakan** | Di halaman aset, kartu *Pemeriksaan & Tanggung Jawab* membandingkan pemeriksaan berurutan dan menunjukkan **kapan sebuah komponen berubah dari Baik ke Tidak Baik dan siapa yang memegang aset pada rentang itu** (pemegang tertentu, saat disimpan di IT, atau tidak dapat dipastikan). Kerusakan yang sudah ada saat diterima tidak dibebankan ke penerima. |
| **Dokumen cetak** | **BAST + pemeriksaan** (Berita Acara Serah Terima Barang: para pihak, rincian, tabel centang hardware/software, kelengkapan, ketentuan, 4 tanda tangan) sebagai dokumen untuk ditandatangani, dan **form ringkas** ("FORM SERAH TERIMA ASET KANTOR", ukuran dari template) sebagai dokumentasi tertulis. Keduanya tersedia untuk ketiga jenis dokumen. |
| **Riwayat & catatan servis** | Setiap perubahan tercatat otomatis (siapa, kapan, dari status apa ke apa). Tambah catatan manual untuk servis atau klaim garansi. Log global di menu *Log Aktivitas*. |
| **Laporan** | Dashboard (status, per kategori, perlu perhatian, garansi segera habis), *Inventaris Karyawan* (siapa memegang apa), ekspor CSV (mengikuti filter). |
| **Impor CSV** | Dari Excel/Google Sheets (pemisah `,` atau `;`). Kategori dan karyawan baru dibuat otomatis, tanggal boleh `10 October 2025`, `10 Oktober 2025`, `10/10/2025`. Ada mode *Periksa dulu*; bila satu baris bermasalah, tidak ada yang disimpan. |

## Menjalankan

```bash
make dev-api    # terminal 1 — API :8090 (SQLite, data awal otomatis)
make dev-web    # terminal 2 — http://localhost:5173
```
Login pertama: lihat `ADMIN_EMAIL` dan `ADMIN_PASSWORD` di [backend/.env](backend/.env) (contoh: [backend/.env.example](backend/.env.example)).
Ganti password lewat klik nama di pojok kanan atas. Database fresh dibuat otomatis di `backend/data/app.db`; hapus folder itu untuk mulai dari nol.

Produksi / server kantor (Go + MySQL):
```bash
PUBLIC_BASE_URL=http://192.168.1.10:8090 \
JWT_SECRET=$(openssl rand -hex 32) ADMIN_PASSWORD='ganti-ini-min-8' docker compose up --build
```

## Stiker QR dan scan dari HP — hal yang perlu diatur

QR berisi tautan `PUBLIC_BASE_URL/a/KODE`. Karena stiker ditempel di barang dan dipakai bertahun-tahun:

1. **Isi `PUBLIC_BASE_URL` dengan alamat tetap yang bisa dibuka dari HP**, mis. IP statis server di jaringan kantor
   (`http://192.168.1.10:8090`) atau domain (`https://aset.perusahaan.id`). Jangan `localhost`. Halaman *Cetak Label* memberi peringatan bila alamatnya `localhost`.
2. **Jangan ganti alamat itu setelah stiker dicetak** — QR lama akan mati. Pakai domain bila memungkinkan.
3. **Login diperlukan.** Data aset tidak terbuka untuk umum; saat scan pertama kali HP diminta login lalu langsung diarahkan ke aset tersebut (sesi 2 jam).
4. Scan lewat kamera di dalam aplikasi hanya jalan di **HTTPS** (aturan browser). Memindai dengan aplikasi kamera bawaan HP/Google Lens tidak butuh HTTPS.

## Migrasi dari Excel

Menu *Daftar Aset → Impor*, unduh template, salin data dari sheet Anda (satu baris = satu barang; barang yang dipegang karyawan diisi di kolom *Pengguna*). Contoh kolom: Kategori, Merk, Tipe, Serial Number, CPU, RAM, Storage, OS, Ukuran Layar, Tahun Pembuatan, Tanggal Pembelian, Harga Beli, Garansi Sampai, Vendor, Status, Lokasi, Pengguna, Jumlah, Keterangan.

> **Jangan menyimpan password atau nomor rahasia di kolom Keterangan.** Aplikasi ini bukan password manager dan catatan terlihat oleh semua yang login.

## Mengganti template formulir
Form ringkas: `backend/internal/pdf/receipt.go`; BAST: `backend/internal/pdf/bast.go`; butir checklist: `backend/internal/models/inspection.go`. Layout form ringkas ada di `receipt.go` (semua ukuran dalam poin, diukur dari template), logo di `backend/internal/pdf/logo.png`. Font Carlito (`backend/internal/pdf/fonts/`) berlisensi SIL OFL; berkas lisensinya disertakan.

## Struktur
```
backend/   cmd/server · internal/{config,database,models,auth,httpx,handlers,service,pdf} · seed/
frontend/  src/{api,stores,router,layouts,components,composables,views,utils}
```

## Test
```bash
make test                                                                  # Go (SQLite) + typecheck + vitest
AMS_TEST_MYSQL=127.0.0.1:3306 AMS_TEST_MYSQL_PASSWORD=... go test ./...    # suite yang sama di MySQL asli
```
Mencakup auth & ganti password, kategori/karyawan, siklus aset (kode, validasi, log perubahan, foto, QR/barcode), alur serah terima/pengembalian/pindah tangan/pembatalan, aturan pemeriksaan fisik, laporan tanggung jawab, PDF form dan BAST, impor-ekspor CSV, dashboard, dan konkurensi.
