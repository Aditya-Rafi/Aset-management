package auth

import (
	"errors"
	"fmt"
	"log"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/contracts"
)

// Bootstrap membuat admin pertama bila tabel users masih kosong.
//
// Bila tabel sudah berisi, ia tidak melakukan apa pun — environment tidak boleh
// bisa mengembalikan admin yang sengaja dinonaktifkan, dan tidak boleh
// mengatur ulang sandi admin yang sudah ada (SPEK-07 §A7-8).
//
// Mengembalikan true bila admin baru dibuat.
func Bootstrap(s *Store, namaPengguna, sandiMentah string) (bool, error) {
	n, err := s.JumlahPengguna()
	if err != nil {
		return false, fmt.Errorf("menghitung pengguna: %w", err)
	}
	if n > 0 {
		return false, nil
	}

	if sandiMentah == "" {
		return false, errors.New(
			"tabel pengguna kosong dan ADMIN_PASSWORD tidak diisi; " +
				"tidak ada sandi bawaan, isi environment itu lalu jalankan ulang")
	}
	if len(sandiMentah) < 12 {
		return false, errors.New("ADMIN_PASSWORD minimal 12 karakter")
	}
	namaPengguna = NormalkanNamaPengguna(namaPengguna)
	if namaPengguna == "" {
		namaPengguna = "admin"
	}
	if !NamaPenggunaSah(namaPengguna) {
		return false, errors.New(
			"ADMIN_USER hanya boleh huruf kecil a-z, angka, titik, " +
				"garis bawah, atau tanda hubung, 3 sampai 64 karakter")
	}

	peran := "admin"
	if _, err := s.BuatPengguna(contracts.PenggunaBaru{
		NamaPengguna: &namaPengguna,
		Sandi:        &sandiMentah,
		NamaLengkap:  &namaPengguna,
		Peran:        &peran,
	}); err != nil {
		return false, fmt.Errorf("membuat admin pertama: %w", err)
	}

	log.Printf("auth: admin pertama %q dibuat", namaPengguna)
	return true, nil
}

// PasangEmailAdminPertama mengisi email admin yang BARU SAJA dibuat bootstrap,
// supaya pengajuan akun (SPEK-14 §A14-12) punya penerima sejak pemasangan
// pertama. Dipanggil hanya bila Bootstrap mengembalikan dibuat = true.
func PasangEmailAdminPertama(s *Store, namaPengguna, email string) error {
	email = NormalkanEmail(email)
	if !EmailSah(email) {
		return fmt.Errorf("email %q tidak sah", email)
	}
	u, err := s.AmbilPenggunaAktif(namaPengguna)
	if err != nil {
		return err
	}
	_, err = s.UbahPengguna(u.Ref, contracts.PenggunaBaru{Email: &email}, "")
	return err
}
