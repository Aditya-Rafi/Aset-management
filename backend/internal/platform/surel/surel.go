// Package surel mengirim surel lewat SMTP memakai net/smtp stdlib (SPEK-14 §A14-6).
//
// Tidak ada dependensi baru: net/smtp, crypto/tls, dan mime sudah cukup untuk
// surel teks polos berisi kode OTP.
package surel

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Mode keamanan sambungan.
const (
	ModeSTARTTLS = "starttls"
	ModeTLS      = "tls"
	ModePolos    = "tidak"
)

// batasWaktu membatasi satu pengiriman seluruhnya, dari sambung sampai QUIT.
const batasWaktu = 20 * time.Second

// Konfigurasi dibaca dari SMTP_*.
type Konfigurasi struct {
	Host  string
	Port  string
	User  string
	Sandi string
	Dari  string
	Mode  string
}

// Aktif melaporkan apakah pengiriman surel dikonfigurasi sama sekali.
func (k Konfigurasi) Aktif() bool { return k.Host != "" }

// Periksa menolak konfigurasi yang pasti gagal, supaya kesalahannya muncul
// saat service naik, bukan saat pengguna pertama lupa sandi.
func (k Konfigurasi) Periksa() error {
	if !k.Aktif() {
		return nil
	}
	switch k.Mode {
	case ModeSTARTTLS, ModeTLS, ModePolos:
	default:
		return fmt.Errorf("SMTP_TLS harus %q, %q, atau %q, dapat %q",
			ModeSTARTTLS, ModeTLS, ModePolos, k.Mode)
	}
	if k.Port == "" {
		return errors.New("SMTP_PORT kosong")
	}
	if _, err := mail.ParseAddress(k.Dari); err != nil {
		return fmt.Errorf("SMTP_DARI tidak sah: %w", err)
	}
	return nil
}

// Pengirim mengirim satu surel.
type Pengirim interface {
	Kirim(Surat) error
}

// SMTP adalah Pengirim sungguhan.
type SMTP struct{ K Konfigurasi }

// Kirim mengirim satu surel.
func (s SMTP) Kirim(srt Surat) error {
	ke := srt.Ke
	pesan, err := Susun(s.K.Dari, srt, time.Now())
	if err != nil {
		return err
	}
	dari, _ := mail.ParseAddress(s.K.Dari)

	alamat := net.JoinHostPort(s.K.Host, s.K.Port)
	konfTLS := &tls.Config{ServerName: s.K.Host, MinVersion: tls.VersionTLS12}

	var sambungan net.Conn
	if s.K.Mode == ModeTLS {
		sambungan, err = tls.DialWithDialer(&net.Dialer{Timeout: batasWaktu}, "tcp", alamat, konfTLS)
	} else {
		sambungan, err = net.DialTimeout("tcp", alamat, batasWaktu)
	}
	if err != nil {
		return fmt.Errorf("menyambung ke %s: %w", alamat, err)
	}
	_ = sambungan.SetDeadline(time.Now().Add(batasWaktu))

	c, err := smtp.NewClient(sambungan, s.K.Host)
	if err != nil {
		_ = sambungan.Close()
		return err
	}
	defer c.Close()

	if s.K.Mode == ModeSTARTTLS {
		// Wajib, bukan "bila tersedia": server yang tidak menawarkan STARTTLS
		// pada mode ini adalah server yang salah atau sambungan yang disadap.
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("server SMTP tidak menawarkan STARTTLS")
		}
		if err := c.StartTLS(konfTLS); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}

	if s.K.User != "" {
		// PlainAuth net/smtp menolak mengirim sandi lewat sambungan polos
		// kecuali ke localhost. Batas itu diwarisi dengan sadar (§A14-6).
		if err := c.Auth(smtp.PlainAuth("", s.K.User, s.K.Sandi, s.K.Host)); err != nil {
			return fmt.Errorf("autentikasi SMTP: %w", err)
		}
	}

	if err := c.Mail(dari.Address); err != nil {
		return err
	}
	if err := c.Rcpt(ke); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(pesan); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// Susun membangun pesan RFC 5322 lengkap.
//
// Penerima dan subjek yang memuat CR atau LF DITOLAK, bukan dibersihkan:
// satu-satunya cara nilai seperti itu sampai ke sini adalah percobaan
// menyisipkan header (Bcc: ke alamat lain) lewat medan email.
//
// Surat yang punya Html dikirim multipart/alternative: klien surel yang bisa
// menampilkan HTML memperlihatkan tombol, yang tidak tetap mendapat teks
// polos dengan tautan yang sama.
func Susun(dari string, srt Surat, waktu time.Time) ([]byte, error) {
	ke, subjek := srt.Ke, srt.Subjek
	for _, v := range []string{dari, ke, subjek} {
		if strings.ContainsAny(v, "\r\n") {
			return nil, errors.New("header surel memuat baris baru")
		}
	}
	if a, err := mail.ParseAddress(ke); err != nil || a.Address != ke {
		return nil, fmt.Errorf("alamat penerima tidak sah: %q", ke)
	}
	pengirim, err := mail.ParseAddress(dari)
	if err != nil {
		return nil, fmt.Errorf("alamat pengirim tidak sah: %w", err)
	}

	acak := make([]byte, 12)
	if _, err := rand.Read(acak); err != nil {
		return nil, err
	}
	domain := "aset-it.local"
	if i := strings.LastIndexByte(pengirim.Address, '@'); i >= 0 {
		domain = pengirim.Address[i+1:]
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", pengirim.String())
	fmt.Fprintf(&b, "To: %s\r\n", ke)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subjek))
	fmt.Fprintf(&b, "Date: %s\r\n", waktu.Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", hex.EncodeToString(acak), domain)
	b.WriteString("MIME-Version: 1.0\r\n")

	if srt.Html == "" {
		if err := tulisBagian(&b, "text/plain", srt.Teks); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}

	batas := "aset-it-" + hex.EncodeToString(acak)
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", batas)
	for _, bag := range []struct{ jenis, isi string }{{"text/plain", srt.Teks}, {"text/html", srt.Html}} {
		fmt.Fprintf(&b, "--%s\r\n", batas)
		if err := tulisBagian(&b, bag.jenis, bag.isi); err != nil {
			return nil, err
		}
	}
	fmt.Fprintf(&b, "--%s--\r\n", batas)
	return b.Bytes(), nil
}

// tulisBagian menulis header Content-Type beserta badannya dalam
// quoted-printable.
func tulisBagian(b *bytes.Buffer, jenis, isi string) error {
	fmt.Fprintf(b, "Content-Type: %s; charset=UTF-8\r\n", jenis)
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(b)
	if _, err := qp.Write([]byte(strings.ReplaceAll(isi, "\n", "\r\n"))); err != nil {
		return err
	}
	if err := qp.Close(); err != nil {
		return err
	}
	b.WriteString("\r\n")
	return nil
}

// Surat adalah satu surel yang menunggu dikirim.
type Surat struct {
	Ke, Subjek, Teks string
	Html             string // opsional; kosong berarti teks polos saja
}

// Antrean mengirim surel di luar jalur permintaan HTTP.
//
// Pengiriman serentak akan membuat permintaan untuk email terdaftar menjawab
// ratusan milidetik lebih lambat daripada untuk email yang tidak terdaftar —
// dan waktu jawaban adalah jawaban (SPEK-14 §A14-5).
type Antrean struct {
	p   Pengirim
	isi chan Surat
}

// NewAntrean menyalakan satu pengirim di latar. Kapasitasnya kecil dan tetap:
// surel yang tidak muat dibuang dengan catatan log, tidak pernah menahan
// permintaan HTTP.
func NewAntrean(p Pengirim, kapasitas int) *Antrean {
	a := &Antrean{p: p, isi: make(chan Surat, kapasitas)}
	go a.jalan()
	return a
}

// Masukkan menaruh surat di antrean tanpa menunggu.
func (a *Antrean) Masukkan(s Surat) bool {
	select {
	case a.isi <- s:
		return true
	default:
		log.Printf("surel: antrean penuh, surel ke %s dibuang", Samarkan(s.Ke))
		return false
	}
}

func (a *Antrean) jalan() {
	for s := range a.isi {
		if err := a.p.Kirim(s); err != nil {
			log.Printf("surel: mengirim ke %s: %v", Samarkan(s.Ke), err)
		}
	}
}

// Samarkan menyisakan huruf pertama dan domain: cukup untuk menyelidiki
// kegagalan kirim, tidak cukup untuk menyusun daftar alamat dari log.
func Samarkan(alamat string) string {
	i := strings.LastIndexByte(alamat, '@')
	if i <= 0 {
		return "***"
	}
	return alamat[:1] + "***" + alamat[i:]
}
