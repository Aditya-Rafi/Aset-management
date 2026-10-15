package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/akses"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/auth"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/config"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/httpx"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/pdf"
)

func crud[T any](r chi.Router, path string, rs *Resource[T]) {
	r.Route(path, func(r chi.Router) {
		r.Get("/", rs.List)
		r.Post("/", rs.Create)
		r.Get("/{id}", rs.Show)
		r.Put("/{id}", rs.Update)
		r.Delete("/{id}", rs.Delete)
	})
}

// NewRouter wires every route. The auth server carries the token signer and
// the account store; the access middleware guards the whole /api tree.
func NewRouter(cfg *config.Config, db *gorm.DB, authSrv *auth.Server) (http.Handler, error) {
	proksi, err := akses.NewProksi(cfg.TrustedProxies)
	if err != nil {
		return nil, err
	}
	guard := &akses.Penjaga{DB: db, Penanda: authSrv.Penanda, Proksi: proksi}
	pw := &PasswordHandler{DB: db, Store: authSrv.Store}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer, middleware.Compress(5))
	r.Use(securityHeaders, cors(cfg.AllowedOrigins))

	assets := &AssetHandler{DB: db, Images: ImageStore{Dir: cfg.UploadDir}, PublicBaseURL: cfg.PublicBaseURL}
	txs := &TransactionHandler{DB: db, Org: pdf.Org{Approver: cfg.ApproverName},
		BastOrg: pdf.BastOrg{Name: cfg.CompanyName, Address: cfg.CompanyAddress, Place: cfg.CompanyPlace}}
	look := &LookupHandler{DB: db, PublicBaseURL: cfg.PublicBaseURL, CompanyName: cfg.CompanyName}
	mon := &MonitorHandler{DB: db}
	dash := &DashboardHandler{DB: db}

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.Ping() != nil {
			httpx.Fail(w, http.StatusServiceUnavailable, "database tidak tersedia")
			return
		}
		httpx.OK(w, "ok", nil)
	})

	r.Group(func(g chi.Router) {
		g.Use(guard.Middleware)
		authSrv.Tambahan = func(a chi.Router) { a.Put("/password", pw.ChangePassword) }
		authSrv.Rute(g)
		g.Route("/api", func(p chi.Router) {
			p.Get("/config", look.Config)
			p.Get("/dashboard", dash.Stats)
			p.Get("/logs", mon.Logs)
			p.Get("/inspection-templates", func(w http.ResponseWriter, r *http.Request) {
				httpx.OK(w, "OK", models.InspectionTemplates)
			})

			emp := &EmployeeHandler{DB: db}
			p.Post("/employees/{id}/offboard", emp.Offboard)
			p.Post("/employees/{id}/reactivate", emp.Reactivate)
			crud(p, "/employees", NewEmployeeResource(db))
			crud(p, "/categories", NewCategoryResource(db))

			p.Route("/assets", func(a chi.Router) {
				a.Get("/", assets.List)
				a.Post("/", assets.Create)
				a.Get("/export", assets.Export)
				a.Get("/import/template", assets.ImportTemplate)
				a.Post("/import", assets.Import)
				a.Get("/tag/{tag}", assets.ByTag)
				a.Get("/{id}", assets.Show)
				a.Put("/{id}", assets.Update)
				a.Delete("/{id}", assets.Delete)
				a.Put("/{id}/status", assets.SetStatus)
				a.Post("/{id}/notes", assets.AddNote)
				a.Get("/{id}/logs", assets.Logs)
				a.Get("/{id}/condition-report", assets.ConditionReport)
				a.Post("/{id}/images", assets.UploadImages)
				a.Delete("/{id}/images/{imageId}", assets.DeleteImage)
				a.Get("/{id}/qr", assets.QR)
				a.Get("/{id}/barcode", assets.Barcode)
			})
			p.Get("/uploads/assets/{name}", assets.Image)

			p.Route("/transactions", func(t chi.Router) {
				t.Get("/", txs.List)
				t.Post("/", txs.Create)
				t.Get("/{id}", txs.Show)
				t.Delete("/{id}", txs.Delete)
				t.Get("/{id}/pdf", txs.PDF)
				t.Get("/{id}/bast", txs.BAST)
			})

			p.Route("/monitoring", func(m chi.Router) {
				m.Get("/employees", mon.Employees)
				m.Get("/employees/{id}/assets", mon.EmployeeAssets)
				m.Get("/employees/{id}/history", mon.EmployeeHistory)
			})

			p.Get("/select/employees", look.Employees)
			p.Get("/select/categories", look.Categories)
			p.Get("/select/assets", look.Assets)
			p.Get("/suggest", look.Suggest)
			p.NotFound(func(w http.ResponseWriter, r *http.Request) {
				httpx.Fail(w, http.StatusNotFound, "Endpoint tidak ditemukan")
			})
		})
	})

	mountSPA(r, cfg.FrontendDir)
	return r, nil
}

// mountSPA serves the built Vue app (if present) with history-mode fallback.
func mountSPA(r chi.Router, dir string) {
	index := filepath.Join(dir, "index.html")
	if _, err := os.Stat(index); err != nil {
		return
	}
	fs := http.FileServer(http.Dir(dir))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		clean := filepath.Join(dir, filepath.Clean("/"+req.URL.Path))
		if st, err := os.Stat(clean); err == nil && !st.IsDir() {
			if strings.HasPrefix(req.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fs.ServeHTTP(w, req)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, req, index)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// cors allows the configured origins (used when the SPA runs on another port, e.g. Vite dev).
func cors(origins []string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if o := r.Header.Get("Origin"); o != "" && allowed[o] {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", o)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
				h.Set("Access-Control-Max-Age", strconvSeconds(10*time.Minute))
				h.Add("Vary", "Origin")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func strconvSeconds(d time.Duration) string { return strconv.Itoa(int(d.Seconds())) }
