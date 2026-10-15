package auth

import (
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/sandi"
)

// periksaSandi memeriksa sandi terhadap bentuk simpan. Selain format PBKDF2
// NetAudit ia mengenali bcrypt dari versi pertama aplikasi ini (dan dari
// Laravel), sehingga admin yang sudah ada tidak perlu mengatur ulang sandi:
// sandi bcrypt yang benar selalu dilaporkan "perlu disandikan ulang" dan
// diganti PBKDF2 saat pemiliknya berhasil masuk.
func periksaSandi(mentah, tersimpan string) (cocok bool, perluUlang bool, err error) {
	if strings.HasPrefix(tersimpan, "$2") {
		ok := bcrypt.CompareHashAndPassword([]byte(tersimpan), []byte(mentah)) == nil
		return ok, ok, nil
	}
	return sandi.Periksa(mentah, tersimpan)
}
