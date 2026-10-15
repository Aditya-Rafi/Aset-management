package handlers

import (
	"net/http"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/akses"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/auth"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/sandi"
)

// CurrentUser returns the signed-in account row, or nil for guest tokens.
func CurrentUser(r *http.Request) *models.User {
	return akses.Dari(r).Pengguna()
}

// PasswordHandler lets a signed-in user change their own password.
type PasswordHandler struct {
	DB    *gorm.DB
	Store *auth.Store
}

// ChangePassword verifies the current password, stores the new one and
// revokes every refresh token, so other devices must sign in again.
func (h *PasswordHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	u := CurrentUser(r)
	if u == nil {
		httpx.Fail(w, http.StatusUnauthorized, "butuh autentikasi")
		return
	}
	errs := map[string]string{}
	if ok, _, _ := sandi.Periksa(in.Current, u.PasswordHash); !ok {
		errs["current_password"] = "Kata sandi saat ini salah"
	}
	switch {
	case len(in.New) < 12:
		errs["new_password"] = "Kata sandi baru minimal 12 karakter"
	case len(in.New) > 128:
		errs["new_password"] = "Kata sandi baru maksimal 128 karakter"
	case in.New == in.Current:
		errs["new_password"] = "Kata sandi baru harus berbeda"
	}
	if len(errs) > 0 {
		httpx.Invalid(w, errs)
		return
	}
	hash, err := sandi.Sandikan(in.New)
	if err != nil {
		httpx.ServerError(w, err)
		return
	}
	if err := h.Store.PerbaruiSandi(u.ID, hash); err != nil {
		httpx.ServerError(w, err)
		return
	}
	if _, err := h.Store.CabutSeluruhPenyegar(u.ID); err != nil {
		httpx.ServerError(w, err)
		return
	}
	httpx.OK(w, "Kata sandi berhasil diubah", nil)
}
