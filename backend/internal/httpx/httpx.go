// Package httpx holds small helpers shared by all HTTP handlers.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Meta struct {
	Total   int64 `json:"total"`
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
}

func JSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func OK(w http.ResponseWriter, message string, data interface{}) {
	JSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": message, "data": data})
}

func Created(w http.ResponseWriter, message string, data interface{}) {
	JSON(w, http.StatusCreated, map[string]interface{}{"success": true, "message": message, "data": data})
}

func List(w http.ResponseWriter, data interface{}, meta Meta) {
	JSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": data, "meta": meta})
}

func Fail(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]interface{}{"success": false, "message": message})
}

func Invalid(w http.ResponseWriter, errs map[string]string) {
	JSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
		"success": false, "message": "Data yang dikirim tidak valid", "errors": errs,
	})
}

func NotFound(w http.ResponseWriter, what string) {
	Fail(w, http.StatusNotFound, what+" tidak ditemukan")
}

func ServerError(w http.ResponseWriter, err error) {
	// Never leak internal error text to clients.
	Fail(w, http.StatusInternalServerError, "Terjadi kesalahan pada server")
	logErr(err)
}

// DecodeJSON reads a JSON body (max 1 MiB) into dst.
func DecodeJSON(r *http.Request, dst interface{}) error {
	body := http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(body)
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("body kosong")
		}
		return errors.New("format JSON tidak valid")
	}
	return nil
}

func IDParam(r *http.Request) (uint, bool) {
	n, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	return uint(n), err == nil && n > 0
}

// ListQuery holds server-side table parameters.
type ListQuery struct {
	Page, PerPage int
	Search        string
	Sort          string
	Desc          bool
}

func ParseListQuery(r *http.Request) ListQuery {
	q := r.URL.Query()
	lq := ListQuery{
		Page:    atoiDefault(q.Get("page"), 1),
		PerPage: atoiDefault(q.Get("per_page"), 10),
		Search:  strings.TrimSpace(q.Get("search")),
		Sort:    q.Get("sort"),
		Desc:    strings.EqualFold(q.Get("order"), "desc"),
	}
	if lq.Page < 1 {
		lq.Page = 1
	}
	if lq.PerPage < 1 {
		lq.PerPage = 10
	}
	if lq.PerPage > 100 {
		lq.PerPage = 100
	}
	return lq
}

func (l ListQuery) Offset() int { return (l.Page - 1) * l.PerPage }

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

// EscapeLike escapes LIKE wildcards in user input. It uses "!" as the escape
// character (queries say ESCAPE '!') because a backslash means different things
// in MySQL and SQLite string literals.
func EscapeLike(s string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(s)
}

// Validator accumulates field errors.
type Validator struct{ Errors map[string]string }

func NewValidator() *Validator { return &Validator{Errors: map[string]string{}} }

func (v *Validator) Add(field, msg string) {
	if _, ok := v.Errors[field]; !ok {
		v.Errors[field] = msg
	}
}

func (v *Validator) OK() bool { return len(v.Errors) == 0 }

func (v *Validator) Required(field, val, label string) {
	if strings.TrimSpace(val) == "" {
		v.Add(field, label+" wajib diisi")
	}
}

func (v *Validator) MaxLen(field, val, label string, n int) {
	if len([]rune(val)) > n {
		v.Add(field, label+" maksimal "+strconv.Itoa(n)+" karakter")
	}
}

func (v *Validator) Email(field string, val *string, label string) {
	if val == nil || *val == "" {
		return
	}
	if a, err := mail.ParseAddress(*val); err != nil || a.Address != *val {
		v.Add(field, label+" harus berupa email yang valid")
	}
}

func (v *Validator) URL(field string, val *string, label string) {
	if val == nil || *val == "" {
		return
	}
	u, err := url.ParseRequestURI(*val)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		v.Add(field, label+" harus berupa URL http(s) yang valid")
	}
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) { JSON(w, status, v) }

// GalatInternal logs the real cause and answers with a generic message in the
// {"galat": "..."} shape used by the authentication endpoints.
func GalatInternal(w http.ResponseWriter, svc, konteks string, err error) {
	log.Printf("%s: %s: %v", svc, konteks, err)
	JSON(w, http.StatusInternalServerError, map[string]string{"galat": "terjadi galat internal"})
}
