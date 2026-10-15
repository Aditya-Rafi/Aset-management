package auth

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// PulihkanAdmin menyetel ulang sandi satu pengguna dari baris pertama masukan.
//
// Inilah jalan yang benar untuk keadaan "sandi admin hilang dan tidak ada admin
// lain yang bisa menyetelnya ulang". Ia dijalankan lewat perintah
//
//	docker compose exec -T auth netaudit-svc pulihkan-admin <nama_pengguna>
//
// sehingga wewenangnya adalah wewenang atas container — wewenang operator
// server, yang memang seharusnya menjadi pemulih terakhir. Bootstrap tidak
// boleh dipakai untuk ini: ia berjalan setiap kali service naik, dan sandi yang
// ditimpa setiap kali naik berarti siapa pun yang bisa menyunting deploy/.env
// menjadi admin aplikasi tanpa pernah melewati aplikasi (SPEK-07 §A7-8).
//
// Sandi dibaca dari masukan, TIDAK PERNAH dari argumen baris perintah: argumen
// tampil di `ps`, tercatat di riwayat shell, dan ikut di log docker.
//
// Seluruh sesi pengguna itu dicabut, dan kejadiannya dicatat di audit_log.
func PulihkanAdmin(s *Store, namaPengguna string, masukan io.Reader) error {
	namaPengguna = NormalkanNamaPengguna(namaPengguna)
	if namaPengguna == "" {
		return errors.New("nama pengguna wajib diisi")
	}

	baris, err := bufio.NewReader(masukan).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("membaca sandi: %w", err)
	}
	sandiBaru := strings.TrimRight(baris, "\r\n")
	if sandiBaru == "" {
		return errors.New("sandi kosong; kirim sandi baru lewat masukan standar")
	}

	u, err := s.SetelUlangSandi(namaPengguna, sandiBaru)
	if err != nil {
		if errors.Is(err, ErrTidakDitemukan) {
			return fmt.Errorf("pengguna %q tidak ada", namaPengguna)
		}
		return err
	}

	aksi := "sandi_dipulihkan_cli"
	if err := s.Audit(&u.Ref, &u.Username, aksi, "pengguna", &u.Ref, nil, nil); err != nil {
		// Sandinya sudah terganti. Gagal mencatat tidak membatalkannya, tetapi
		// harus terlihat oleh orang yang menjalankan perintah.
		return fmt.Errorf("sandi diganti, tetapi gagal mencatat ke audit_log: %w", err)
	}
	return nil
}
