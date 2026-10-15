package handlers

import (
	"errors"
	"net/http"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/service"
)

// serviceError maps a service-layer error to the right HTTP response.
func serviceError(w http.ResponseWriter, err error, what string) {
	var ve *service.ValidationError
	var ce *service.ConflictError
	switch {
	case errors.As(err, &ve):
		httpx.JSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
			"success": false, "message": ve.Error(), "errors": ve.Fields,
		})
	case errors.As(err, &ce):
		httpx.Fail(w, http.StatusConflict, ce.Msg)
	case errors.Is(err, service.ErrNotFound):
		httpx.NotFound(w, what)
	default:
		httpx.ServerError(w, err)
	}
}
