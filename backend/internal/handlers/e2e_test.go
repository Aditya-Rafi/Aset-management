package handlers_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"gorm.io/gorm"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/auth"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/config"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/database"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/handlers"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/models"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/sandi"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/token"
	"github.com/Aditya-Rafi/asset-management-system/backend/seed"
)

const testAdminPassword = "password-admin-123"

type env struct {
	t     *testing.T
	srv   *httptest.Server
	c     *http.Client
	db    *gorm.DB
	auth  *auth.Server
	token string // Bearer token added to every request; empty = anonymous
	cfg   *config.Config
}

// bearer adds the current access token to each request.
type bearer struct{ e *env }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	if b.e.token != "" && r.Header.Get("Authorization") == "" {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+b.e.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func newEnv(t *testing.T) *env { return buildEnv(t, true) }

// newEnvNoAdmin is an environment with an empty users table and nobody signed in.
func newEnvNoAdmin(t *testing.T) *env { return buildEnv(t, false) }

func buildEnv(t *testing.T, withAdmin bool) *env {
	t.Helper()
	// Hashing at full strength would make the suite crawl.
	old := sandi.PutaranAktif
	sandi.PutaranAktif = 1000
	t.Cleanup(func() { sandi.PutaranAktif = old })
	cfg := &config.Config{
		Env: "test", DBDriver: "sqlite", DBSQLitePath: t.TempDir() + "/t.db", JWTSecret: "test-secret-test-secret-0123456789",
		AccessTokenTTL: 15 * time.Minute, AdminUser: "admin", AdminPassword: testAdminPassword,
		UploadDir: t.TempDir(), FrontendDir: "/nonexistent",
		CompanyName: "Divisi IT Test", PublicBaseURL: "",
	}
	// AMS_TEST_MYSQL=host:port runs the same suite against a real MySQL
	// (root password in AMS_TEST_MYSQL_PASSWORD); each test gets its own database.
	if addr := os.Getenv("AMS_TEST_MYSQL"); addr != "" {
		host, port, _ := strings.Cut(addr, ":")
		name := fmt.Sprintf("ams_test_%d", time.Now().UnixNano())
		admin, err := sql.Open("mysql", fmt.Sprintf("root:%s@tcp(%s:%s)/", os.Getenv("AMS_TEST_MYSQL_PASSWORD"), host, port))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE " + name); admin.Close() })
		cfg.DBDriver, cfg.DBHost, cfg.DBPort, cfg.DBName = "mysql", host, port, name
		cfg.DBUser, cfg.DBPassword = "root", os.Getenv("AMS_TEST_MYSQL_PASSWORD")
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := seed.Run(db); err != nil {
		t.Fatal(err)
	}
	pen, err := token.NewPenanda(cfg.JWTSecret, cfg.AccessTokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	as := auth.NewServer(db, pen)
	if withAdmin {
		if _, err := auth.Bootstrap(as.Store, cfg.AdminUser, cfg.AdminPassword); err != nil {
			t.Fatal(err)
		}
	}
	router, err := handlers.NewRouter(cfg, db, as)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	e := &env{t: t, srv: srv, db: db, auth: as, cfg: cfg}
	e.c = &http.Client{Transport: bearer{e}}
	if withAdmin {
		e.login()
	}
	return e
}

func (e *env) do(method, path string, body interface{}) (int, map[string]interface{}) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set("Content-Type", "application/json")
	return e.send(req)
}

func (e *env) send(req *http.Request) (int, map[string]interface{}) {
	e.t.Helper()
	res, err := e.c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]interface{}
	b, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(b, &m)
	return res.StatusCode, m
}

func (e *env) get(path string) (*http.Response, []byte) {
	e.t.Helper()
	res, err := e.c.Get(e.srv.URL + path)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

func (e *env) login() { e.loginAs("admin", testAdminPassword) }

// loginAs signs in and keeps the access token for the following requests.
func (e *env) loginAs(user, pass string) {
	e.t.Helper()
	e.token = ""
	code, m := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": user, "sandi": pass})
	if code != 200 {
		e.t.Fatalf("login %s failed: %d %v", user, code, m)
	}
	e.token = str(m["token"])
}

func (e *env) upload(path, field, name string, content []byte) (int, map[string]interface{}) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile(field, name)
	_, _ = fw.Write(content)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return e.send(req)
}

func pngBytes(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 100, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// inspLaptop is a complete laptop check: everything Baik except the given codes.
func inspLaptop(bad ...string) map[string]interface{} {
	isBad := map[string]bool{}
	for _, b := range bad {
		isBad[b] = true
	}
	items := []map[string]interface{}{}
	for _, sec := range models.InspectionTemplates[models.ProfileLaptop].Sections {
		for _, it := range sec.Items {
			r, note := "ok", ""
			if isBad[it.Code] {
				r, note = "bad", "rusak: "+it.Label
			}
			items = append(items, map[string]interface{}{"code": it.Code, "result": r, "note": note})
		}
	}
	concl := "layak"
	if len(bad) > 0 {
		concl = "perlu_perbaikan"
	}
	return map[string]interface{}{"items": items, "conclusion": concl, "battery_health": 90}
}

func data(m map[string]interface{}) map[string]interface{} {
	d, _ := m["data"].(map[string]interface{})
	return d
}
func list(m map[string]interface{}) []interface{} { l, _ := m["data"].([]interface{}); return l }
func num(v interface{}) int                       { f, _ := v.(float64); return int(f) }
func str(v interface{}) string                    { s, _ := v.(string); return s }
func itoa(n int) string                           { return fmt.Sprint(n) }
func total(m map[string]interface{}) int          { return num(m["meta"].(map[string]interface{})["total"]) }

// category id by code from the seeded defaults
func (e *env) cat(code string) int {
	e.t.Helper()
	_, m := e.do("GET", "/api/categories?per_page=100", nil)
	for _, c := range list(m) {
		cm := c.(map[string]interface{})
		if cm["code"] == code {
			return num(cm["id"])
		}
	}
	e.t.Fatalf("category %s not found", code)
	return 0
}

func (e *env) emp(name string) int {
	e.t.Helper()
	code, m := e.do("POST", "/api/employees", map[string]interface{}{"name": name, "department": "IT"})
	if code != 201 {
		e.t.Fatalf("employee: %d %v", code, m)
	}
	return num(data(m)["id"])
}

func (e *env) asset(code string, extra map[string]interface{}) map[string]interface{} {
	e.t.Helper()
	body := map[string]interface{}{"category_id": e.cat(code)}
	for k, v := range extra {
		body[k] = v
	}
	c, m := e.do("POST", "/api/assets", body)
	if c != 201 {
		e.t.Fatalf("create asset: %d %v", c, m)
	}
	return data(m)
}

func TestAuthAndPassword(t *testing.T) {
	e := newEnv(t)
	e.token = ""
	if code, _ := e.do("GET", "/api/assets", nil); code != 401 {
		t.Fatalf("expected 401, got %d", code)
	}
	if code, _ := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": "admin", "sandi": "nope"}); code != 401 {
		t.Fatalf("bad password: %d", code)
	}
	e.login()
	if code, m := e.do("PUT", "/api/auth/password", map[string]string{"current_password": "wrong", "new_password": "newpassword-12345"}); code != 422 || m["errors"] == nil {
		t.Fatalf("wrong current password: %d", code)
	}
	if code, _ := e.do("PUT", "/api/auth/password", map[string]string{"current_password": testAdminPassword, "new_password": "short"}); code != 422 {
		t.Fatalf("short password: %d", code)
	}
	if code, _ := e.do("PUT", "/api/auth/password", map[string]string{"current_password": testAdminPassword, "new_password": "newpassword-12345"}); code != 200 {
		t.Fatalf("change password: %d", code)
	}
	e.token = ""
	if code, _ := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": "admin", "sandi": testAdminPassword}); code != 401 {
		t.Fatalf("old password still works")
	}
	if code, _ := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": "admin", "sandi": "newpassword-12345"}); code != 200 {
		t.Fatalf("new password rejected")
	}
}

func TestCategoriesAndEmployees(t *testing.T) {
	e := newEnv(t)
	_, m := e.do("GET", "/api/categories?per_page=100", nil)
	if total(m) < 15 {
		t.Fatalf("default categories missing: %d", total(m))
	}
	lpt := m["data"].([]interface{})
	var laptop map[string]interface{}
	for _, c := range lpt {
		if c.(map[string]interface{})["code"] == "LPT" {
			laptop = c.(map[string]interface{})
		}
	}
	if fields := laptop["spec_fields"].([]interface{}); len(fields) != 4 || fields[0] != "cpu" {
		t.Fatalf("laptop spec fields: %v", laptop["spec_fields"])
	}

	// code rules
	if code, _ := e.do("POST", "/api/categories", map[string]interface{}{"name": "Switch", "code": "x"}); code != 422 {
		t.Fatalf("bad code: %d", code)
	}
	if code, m := e.do("POST", "/api/categories", map[string]interface{}{"name": "Router", "code": "lpt"}); code != 422 || m["errors"].(map[string]interface{})["code"] == nil {
		t.Fatalf("duplicate code (case-insensitive): %d %v", code, m)
	}
	code, m := e.do("POST", "/api/categories", map[string]interface{}{"name": "Switch", "code": "swt", "spec_fields": []string{"os", "bogus", "cpu"}})
	if code != 201 || data(m)["code"] != "SWT" {
		t.Fatalf("create category: %d %v", code, m)
	}
	if f := data(m)["spec_fields"].([]interface{}); len(f) != 2 || f[0] != "cpu" || f[1] != "os" {
		t.Fatalf("spec_fields must be filtered + ordered: %v", f)
	}
	id := itoa(num(data(m)["id"]))
	if code, m = e.do("PUT", "/api/categories/"+id, map[string]interface{}{"name": "Network Switch", "code": "SWT", "spec_fields": []string{"os"}}); code != 200 || len(data(m)["spec_fields"].([]interface{})) != 1 {
		t.Fatalf("update category: %d %v", code, m)
	}

	// a category in use cannot be deleted
	e.asset("SWT", nil)
	if code, _ = e.do("DELETE", "/api/categories/"+id, nil); code != 409 {
		t.Fatalf("category in use: %d", code)
	}

	// employees
	if code, m = e.do("POST", "/api/employees", map[string]interface{}{"name": " ", "email": "bad"}); code != 422 || len(m["errors"].(map[string]interface{})) != 2 {
		t.Fatalf("employee validation: %d %v", code, m)
	}
	eid := e.emp("  Budi  ")
	code, m = e.do("GET", "/api/employees/"+itoa(eid), nil)
	if data(m)["name"] != "Budi" || data(m)["department"] != "IT" {
		t.Fatalf("employee: %v", m)
	}
	if code, _ = e.do("DELETE", "/api/employees/"+itoa(eid), nil); code != 200 {
		t.Fatalf("delete employee: %d", code)
	}
}

func TestAssetLifecycle(t *testing.T) {
	e := newEnv(t)

	a1 := e.asset("LPT", map[string]interface{}{
		"brand": "Lenovo", "model": "IdeaPad Slim 3", "serial_number": "PF123", "cpu": "Intel Core i5-12450H",
		"ram": "16 GB", "storage": "512 GB", "os": "Windows 11", "manufacture_year": 2023,
		"purchase_date": "2025-02-11", "purchase_price": 8500000, "warranty_until": "2027-02-11",
		"location": "Ruang IT", "notes": "baru",
	})
	a2 := e.asset("LPT", nil)
	m1 := e.asset("MON", map[string]interface{}{"screen_size": "24 inch", "quantity": 1})
	if a1["tag"] != "LPT-0001" || a2["tag"] != "LPT-0002" || m1["tag"] != "MON-0001" {
		t.Fatalf("tags: %v %v %v", a1["tag"], a2["tag"], m1["tag"])
	}
	if a1["status"] != "available" || a1["purchase_date"] != "2025-02-11" || num(a1["quantity"]) != 1 {
		t.Fatalf("defaults: %v", a1)
	}
	id := itoa(num(a1["id"]))

	// validation
	for name, body := range map[string]map[string]interface{}{
		"no category":   {},
		"bad year":      {"category_id": e.cat("LPT"), "manufacture_year": 1850},
		"negative":      {"category_id": e.cat("LPT"), "purchase_price": -5},
		"in_use create": {"category_id": e.cat("LPT"), "status": "in_use"},
		"bad status":    {"category_id": e.cat("LPT"), "status": "lost"},
		"unknown cat":   {"category_id": 9999},
	} {
		if code, _ := e.do("POST", "/api/assets", body); code != 422 {
			t.Fatalf("%s: want 422 got %d", name, code)
		}
	}

	// lookup by tag (what a scanned sticker resolves to), case-insensitive
	code, m := e.do("GET", "/api/assets/tag/lpt-0001", nil)
	if code != 200 || data(m)["serial_number"] != "PF123" || data(m)["category"].(map[string]interface{})["name"] != "Laptop" {
		t.Fatalf("by tag: %d %v", code, m)
	}
	if code, _ = e.do("GET", "/api/assets/tag/NOPE-1", nil); code != 404 {
		t.Fatalf("unknown tag: %d", code)
	}

	// update logs what changed
	code, m = e.do("PUT", "/api/assets/"+id, map[string]interface{}{
		"category_id": e.cat("LPT"), "brand": "Lenovo", "model": "IdeaPad Slim 3", "serial_number": "PF123", "cpu": "Intel Core i5-12450H",
		"ram": "32 GB", "storage": "512 GB", "os": "Windows 11", "manufacture_year": 2023,
		"purchase_date": "2025-02-11", "purchase_price": 8500000, "warranty_until": "2027-02-11", "location": "Meja Pak Nur", "notes": "baru",
	})
	if code != 200 || data(m)["ram"] != "32 GB" || data(m)["tag"] != "LPT-0001" {
		t.Fatalf("update: %d %v", code, m)
	}
	_, m = e.do("GET", "/api/assets/"+id+"/logs", nil)
	msgs := ""
	for _, l := range list(m) {
		msgs += str(l.(map[string]interface{})["message"]) + "|"
	}
	if !strings.Contains(msgs, "Diubah: RAM, lokasi") || !strings.Contains(msgs, "Aset didaftarkan") {
		t.Fatalf("log: %s", msgs)
	}

	// status changes
	if code, _ = e.do("PUT", "/api/assets/"+id+"/status", map[string]interface{}{"status": "in_use"}); code != 422 {
		t.Fatalf("in_use must go through handover: %d", code)
	}
	if code, _ = e.do("PUT", "/api/assets/"+id+"/status", map[string]interface{}{"status": "available"}); code != 422 {
		t.Fatalf("same status: %d", code)
	}
	code, m = e.do("PUT", "/api/assets/"+id+"/status", map[string]interface{}{"status": "repair", "note": "klaim garansi"})
	if code != 200 || data(m)["status"] != "repair" {
		t.Fatalf("to repair: %d %v", code, m)
	}
	if code, _ = e.do("POST", "/api/assets/"+id+"/notes", map[string]interface{}{"message": "  "}); code != 422 {
		t.Fatalf("empty note: %d", code)
	}
	if code, _ = e.do("POST", "/api/assets/"+id+"/notes", map[string]interface{}{"message": "Baterai diganti"}); code != 201 {
		t.Fatalf("note: %d", code)
	}

	// filters + search
	_, m = e.do("GET", "/api/assets?status=repair", nil)
	if total(m) != 1 {
		t.Fatalf("status filter: %v", m["meta"])
	}
	_, m = e.do("GET", "/api/assets?search=pf123", nil)
	if total(m) != 1 {
		t.Fatalf("search serial: %v", m["meta"])
	}
	_, m = e.do("GET", "/api/assets?search=%25", nil)
	if total(m) != 0 {
		t.Fatalf("wildcard must be literal: %v", m["meta"])
	}
	_, m = e.do("GET", "/api/assets?category_id="+itoa(e.cat("MON"))+"&sort=tag&order=asc", nil)
	if total(m) != 1 {
		t.Fatalf("category filter: %v", m["meta"])
	}
	_, m = e.do("GET", "/api/assets?ids="+id+","+itoa(num(a2["id"])), nil)
	if total(m) != 2 {
		t.Fatalf("ids filter: %v", m["meta"])
	}

	// photos
	code, m = e.upload("/api/assets/"+id+"/images", "images", "p.png", pngBytes(900, 700))
	if code != 201 || len(data(m)["images"].([]interface{})) != 1 {
		t.Fatalf("upload: %d %v", code, m)
	}
	img := data(m)["images"].([]interface{})[0].(map[string]interface{})
	res, b := e.get("/api/uploads/assets/" + str(img["name"]))
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if res.StatusCode != 200 || err != nil || cfg.Width > 480 || cfg.Height > 360 {
		t.Fatalf("photo not served/resized: %d %v %+v", res.StatusCode, err, cfg)
	}
	if code, _ = e.upload("/api/assets/"+id+"/images", "images", "x.png", []byte("<svg>not an image</svg>")); code != 422 {
		t.Fatalf("non-image: %d", code)
	}
	if res, _ = e.get("/api/uploads/assets/..%2F..%2Fetc%2Fpasswd"); res.StatusCode == 200 {
		t.Fatalf("path traversal")
	}
	if code, _ = e.do("DELETE", "/api/assets/"+id+"/images/"+itoa(num(img["id"])), nil); code != 200 {
		t.Fatalf("delete image: %d", code)
	}
	if res, _ = e.get("/api/uploads/assets/" + str(img["name"])); res.StatusCode == 200 {
		t.Fatalf("file must be removed")
	}

	// stickers: QR + Code 128 are PNGs; QR size respected
	res, b = e.get("/api/assets/" + id + "/qr?size=320")
	qrCfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if res.StatusCode != 200 || err != nil || format != "png" || qrCfg.Width != 320 {
		t.Fatalf("qr: %d %v %v %+v", res.StatusCode, err, format, qrCfg)
	}
	res, b = e.get("/api/assets/" + id + "/barcode")
	if _, format, err = image.DecodeConfig(bytes.NewReader(b)); res.StatusCode != 200 || err != nil || format != "png" {
		t.Fatalf("barcode: %d %v", res.StatusCode, err)
	}

	// deleting: fine for an untouched asset
	if code, _ = e.do("DELETE", "/api/assets/"+itoa(num(m1["id"])), nil); code != 200 {
		t.Fatalf("delete untouched asset: %d", code)
	}
	// config exposes the address stickers will point at
	_, m = e.do("GET", "/api/config", nil)
	if !strings.HasPrefix(str(data(m)["public_base_url"]), "http://127.0.0.1") {
		t.Fatalf("config: %v", m)
	}
}

func TestHandoverFlow(t *testing.T) {
	e := newEnv(t)
	budi, sari := e.emp("Budi"), e.emp("Sari")
	l1 := e.asset("LPT", map[string]interface{}{"brand": "Lenovo", "location": "Ruang IT"})
	l2 := e.asset("LPT", map[string]interface{}{"brand": "Asus"})
	broken := e.asset("MON", map[string]interface{}{"status": "broken"})
	id1, id2 := num(l1["id"]), num(l2["id"])

	out := func(emp int, ids ...int) (int, map[string]interface{}) {
		items := []map[string]interface{}{}
		for _, id := range ids {
			items = append(items, map[string]interface{}{"asset_id": id, "inspection": inspLaptop()})
		}
		return e.do("POST", "/api/transactions", map[string]interface{}{"type": "out", "employee_id": emp, "note": "kerja", "items": items})
	}

	// only Tersedia assets can be handed out
	if code, m := out(budi, num(broken["id"])); code != 422 || !strings.Contains(str(m["message"]), "Rusak") {
		t.Fatalf("broken asset handover: %d %v", code, m)
	}
	code, m := out(budi, id1, id2)
	if code != 201 {
		t.Fatalf("OUT: %d %v", code, m)
	}
	tx := data(m)
	txID := itoa(num(tx["id"]))
	if !strings.HasPrefix(str(tx["order_number"]), "OUT-"+time.Now().Format("20060102")+"-001") || len(tx["details"].([]interface{})) != 2 {
		t.Fatalf("bad tx: %v", tx)
	}
	_, m = e.do("GET", "/api/assets/"+itoa(id1), nil)
	if data(m)["status"] != "in_use" || num(data(m)["holder_id"]) != budi || data(m)["location"] != nil {
		t.Fatalf("asset after OUT: %v", data(m))
	}
	// second document of the day gets the next number
	l3 := e.asset("LPT", nil)
	_, m = out(sari, num(l3["id"]))
	if !strings.HasSuffix(str(data(m)["order_number"]), "-002") {
		t.Fatalf("sequence: %v", data(m)["order_number"])
	}

	// handing out something already out / duplicates / no items
	if code, _ = out(sari, id1); code != 422 {
		t.Fatalf("double OUT: %d", code)
	}
	if code, _ = out(sari, id2, id2); code != 422 {
		t.Fatalf("duplicate: %d", code)
	}
	if code, _ = e.do("POST", "/api/transactions", map[string]interface{}{"type": "out", "employee_id": sari, "items": []int{}}); code != 422 {
		t.Fatalf("no items: %d", code)
	}

	// return: wrong person, then right person with a result status per asset
	in := func(emp int, items []map[string]interface{}, loc string) (int, map[string]interface{}) {
		return e.do("POST", "/api/transactions", map[string]interface{}{"type": "in", "employee_id": emp, "location": loc, "items": items})
	}
	if code, m = in(sari, []map[string]interface{}{{"asset_id": id1}}, ""); code != 422 || !strings.Contains(str(m["message"]), "tidak dipegang") {
		t.Fatalf("wrong holder: %d %v", code, m)
	}
	if code, _ = in(budi, []map[string]interface{}{{"asset_id": id1, "result_status": "in_use"}}, ""); code != 422 {
		t.Fatalf("bad result status: %d", code)
	}
	code, m = in(budi, []map[string]interface{}{
		{"asset_id": id1, "result_status": "available", "inspection": inspLaptop()},
		{"asset_id": id2, "result_status": "broken", "note": "layar retak", "inspection": inspLaptop("hw_screen")},
	}, "Ruang Server")
	if code != 201 || !strings.HasPrefix(str(data(m)["order_number"]), "IN-") {
		t.Fatalf("IN: %d %v", code, m)
	}
	inID := itoa(num(data(m)["id"]))
	_, m = e.do("GET", "/api/assets/"+itoa(id1), nil)
	if data(m)["status"] != "available" || data(m)["holder_id"] != nil || data(m)["location"] != "Ruang Server" {
		t.Fatalf("after IN: %v", data(m))
	}
	_, m = e.do("GET", "/api/assets/"+itoa(id2), nil)
	if data(m)["status"] != "broken" {
		t.Fatalf("returned broken: %v", data(m)["status"])
	}
	// returning something not in use
	if code, _ = in(budi, []map[string]interface{}{{"asset_id": id1}}, ""); code != 422 {
		t.Fatalf("return of available asset: %d", code)
	}

	// listing, search by tag, detail, PDFs for both types
	_, m = e.do("GET", "/api/transactions?type=in", nil)
	if total(m) != 1 {
		t.Fatalf("type filter: %v", m["meta"])
	}
	_, m = e.do("GET", "/api/transactions?search=LPT-0001", nil)
	if total(m) != 2 {
		t.Fatalf("search by asset tag: %v", m["meta"])
	}
	for _, id := range []string{txID, inID} {
		res, b := e.get("/api/transactions/" + id + "/pdf")
		if res.StatusCode != 200 || !bytes.HasPrefix(b, []byte("%PDF")) {
			t.Fatalf("pdf %s: %d", id, res.StatusCode)
		}
	}

	// the OUT document cannot be undone: its assets moved again afterwards
	if code, _ = e.do("DELETE", "/api/transactions/"+txID, nil); code != 409 {
		t.Fatalf("older doc must be refused: %d", code)
	}
	// asset with history cannot be deleted
	if code, _ = e.do("DELETE", "/api/assets/"+itoa(id1), nil); code != 409 {
		t.Fatalf("asset with history: %d", code)
	}
	// undoing the newest document restores exactly what it changed
	if code, _ = e.do("DELETE", "/api/transactions/"+inID, nil); code != 200 {
		t.Fatalf("undo IN: %d", code)
	}
	_, m = e.do("GET", "/api/assets/"+itoa(id1), nil)
	if data(m)["status"] != "in_use" || num(data(m)["holder_id"]) != budi {
		t.Fatalf("after undo IN: %v", data(m))
	}
	_, m = e.do("GET", "/api/assets/"+itoa(id2), nil)
	if data(m)["status"] != "in_use" {
		t.Fatalf("id2 after undo IN: %v", data(m)["status"])
	}
	// now the OUT is the latest -> undo restores available + the original location
	if code, _ = e.do("DELETE", "/api/transactions/"+txID, nil); code != 200 {
		t.Fatalf("undo OUT: %d", code)
	}
	_, m = e.do("GET", "/api/assets/"+itoa(id1), nil)
	if data(m)["status"] != "available" || data(m)["holder_id"] != nil || data(m)["location"] != "Ruang IT" {
		t.Fatalf("after undo OUT: %v", data(m))
	}

	// an asset that is in use cannot be re-statused by hand
	_, m = out(sari, id1)
	if code, _ = e.do("PUT", "/api/assets/"+itoa(id1)+"/status", map[string]interface{}{"status": "broken"}); code != 409 {
		t.Fatalf("status change while in use: %d", code)
	}
	// a person who holds assets cannot be deleted
	if code, _ = e.do("DELETE", "/api/employees/"+itoa(sari), nil); code != 409 {
		t.Fatalf("employee holding assets: %d", code)
	}

	// monitoring: what Sari holds now, her history, and the global feed
	_, m = e.do("GET", "/api/monitoring/employees?holding=1", nil)
	if total(m) != 1 || num(list(m)[0].(map[string]interface{})["asset_count"]) != 2 {
		t.Fatalf("monitor employees: %v", m)
	}
	_, m = e.do("GET", "/api/monitoring/employees/"+itoa(sari)+"/assets", nil)
	if len(data(m)["assets"].([]interface{})) != 2 {
		t.Fatalf("employee assets: %v", m)
	}
	_, m = e.do("GET", "/api/monitoring/employees/"+itoa(sari)+"/history", nil)
	if len(list(m)) != 2 {
		t.Fatalf("employee history: %v", m)
	}
	_, m = e.do("GET", "/api/logs?action=assigned", nil)
	if total(m) < 3 {
		t.Fatalf("logs feed: %v", m["meta"])
	}
	// select helpers used by the handover form
	_, m = e.do("GET", "/api/select/employees?holding=1", nil)
	if code, _ := e.do("GET", "/api/select/assets?status=available&search=lpt", nil); code != 200 {
		t.Fatalf("select assets: %d", code)
	}
}

func TestConcurrentHandoverOnlyOneWins(t *testing.T) {
	e := newEnv(t)
	emp := e.emp("Budi")
	a := e.asset("LPT", nil)

	const n = 10
	codes := make(chan int, n)
	for i := 0; i < n; i++ {
		go func() {
			b, _ := json.Marshal(map[string]interface{}{"type": "out", "employee_id": emp, "items": []map[string]interface{}{{"asset_id": num(a["id"]), "inspection": inspLaptop()}}})
			req, _ := http.NewRequest("POST", e.srv.URL+"/api/transactions", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			res, err := e.c.Do(req)
			if err != nil {
				codes <- -1
				return
			}
			res.Body.Close()
			codes <- res.StatusCode
		}()
	}
	created, rejected := 0, 0
	for i := 0; i < n; i++ {
		switch <-codes {
		case 201:
			created++
		case 422:
			rejected++
		}
	}
	if created != 1 || rejected != n-1 {
		t.Fatalf("expected 1 created and %d rejected, got %d / %d", n-1, created, rejected)
	}
}

func TestCSVImportExport(t *testing.T) {
	e := newEnv(t)
	post := func(csv, query string) (int, map[string]interface{}) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("file", "x.csv")
		_, _ = fw.Write([]byte(csv))
		_ = mw.Close()
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/assets/import"+query, &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		return e.send(req)
	}

	// semicolon separated, Indonesian/English dates, free-text status like the spreadsheet
	good := "\xef\xbb\xbfKategori;Merk;Tipe;CPU;RAM;Storage;OS;Tahun Pembuatan;Tanggal Pembelian;Harga Beli;Status;Lokasi;Pengguna;Jumlah;Keterangan\n" +
		"Laptop;Lenovo;Ideapad Slim 3;Intel Core i5-12450H;16 GB;512 GB;Windows 11;2023;10 June 2026;Rp 8.500.000;;;Rizal;;Pemilik sebelumnya: Mentari\n" +
		"Laptop;Lenovo;Ideapad;AMD Ryzen 5 5500U;16 GB;512 GB;;;04 Mei 2026;;Tersedia;Ruang IT;;;\n" +
		"Monitor;LG;24MR400-B;;;;;2025;02/07/2026;;rusak;Ruang Server;;2;\n" +
		"Switch Jaringan;TP-Link;;;;;;;;;;Rak;;;\n" +
		"Laptop;Lenovo;Lama;;;;;;;;Rusak;;Nur;;\n"
	code, m := post(good, "?dry_run=1")
	if code != 200 || data(m)["dry_run"] != true || num(data(m)["created"]) != 5 {
		t.Fatalf("dry run: %d %v", code, m)
	}
	_, m = e.do("GET", "/api/assets", nil)
	if total(m) != 0 {
		t.Fatalf("dry run must not save anything")
	}

	code, m = post(good, "")
	if code != 200 || num(data(m)["created"]) != 5 {
		t.Fatalf("import: %d %v", code, m)
	}
	if cc := data(m)["categories_created"].([]interface{}); len(cc) != 1 || cc[0] != "Switch Jaringan" {
		t.Fatalf("categories created: %v", cc)
	}
	if ee := data(m)["employees_created"].([]interface{}); len(ee) != 1 || ee[0] != "Rizal" {
		t.Fatalf("employees created: %v", ee)
	}

	_, m = e.do("GET", "/api/assets/tag/LPT-0001", nil)
	a := data(m)
	if a["status"] != "in_use" || data(map[string]interface{}{"data": a["holder"]})["name"] != "Rizal" ||
		a["purchase_date"] != "2026-06-10" || num(a["purchase_price"]) != 8500000 || num(a["manufacture_year"]) != 2023 {
		t.Fatalf("imported asset: %v", a)
	}
	_, m = e.do("GET", "/api/assets/tag/LPT-0002", nil)
	if data(m)["purchase_date"] != "2026-05-04" || data(m)["status"] != "available" {
		t.Fatalf("Indonesian month: %v", data(m))
	}
	_, m = e.do("GET", "/api/assets/tag/MON-0001", nil)
	if data(m)["status"] != "broken" || data(m)["purchase_date"] != "2026-07-02" || num(data(m)["quantity"]) != 2 {
		t.Fatalf("monitor: %v", data(m))
	}
	// a holder on a non-in-use row is kept as a note, not as a holder
	_, m = e.do("GET", "/api/assets/tag/LPT-0003", nil)
	if data(m)["holder_id"] != nil || !strings.Contains(str(data(m)["notes"]), "Pengguna terakhir: Nur") {
		t.Fatalf("last user note: %v", data(m))
	}
	_, m = e.do("GET", "/api/assets/tag/SWT-0001", nil)
	if m["success"] != true {
		t.Fatalf("auto category tag: %v", m)
	}

	// one bad row aborts everything and reports each problem with its line number
	bad := "Kategori;Merk;Tanggal Pembelian;Status;Pengguna\n" +
		"Laptop;Dell;2026-01-01;Tersedia;\n" +
		"Laptop;HP;31/31/2026;Tersedia;\n" +
		"Laptop;Acer;;Hilang;\n" +
		"Laptop;Asus;;Dipakai;\n" +
		";Tanpa kategori;;;\n"
	code, m = post(bad, "")
	errs := data(m)["errors"].([]interface{})
	if code != 422 || len(errs) != 4 || num(errs[0].(map[string]interface{})["row"]) != 3 {
		t.Fatalf("row errors: %d %v", code, m)
	}
	_, m = e.do("GET", "/api/assets", nil)
	if total(m) != 5 {
		t.Fatalf("failed import must be all-or-nothing, have %d assets", total(m))
	}
	if code, _ = post("Merk;Tipe\nA;B\n", ""); code != 422 {
		t.Fatalf("missing category column: %d", code)
	}

	// export round-trips through the importer and neutralises formulas
	_, m = e.do("POST", "/api/assets", map[string]interface{}{"category_id": e.cat("MSE"), "brand": "=HYPERLINK(\"http://evil\")"})
	res, b := e.get("/api/assets/export")
	out := string(b)
	if res.StatusCode != 200 || !strings.HasPrefix(out, "\xef\xbb\xbfKode;Kategori") || !strings.Contains(out, "LPT-0001;Laptop;Lenovo") ||
		strings.Contains(out, ";=HYPERLINK") || !strings.Contains(out, "Dipakai") {
		t.Fatalf("export: %d\n%s", res.StatusCode, out)
	}
	res, b = e.get("/api/assets/import/template")
	if res.StatusCode != 200 || !strings.Contains(string(b), "Serial Number") {
		t.Fatalf("template: %d", res.StatusCode)
	}
	res, b = e.get("/api/assets/export?status=broken&delimiter=comma")
	if !strings.Contains(string(b), "Kode,Kategori") || strings.Count(string(b), "\n") != 3 {
		t.Fatalf("filtered comma export:\n%s", b)
	}
}

func TestDashboardAndSuggest(t *testing.T) {
	e := newEnv(t)
	emp := e.emp("Budi")
	soon := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	past := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	a := e.asset("LPT", map[string]interface{}{"brand": "Lenovo", "warranty_until": soon, "location": "Ruang IT"})
	e.asset("LPT", map[string]interface{}{"brand": "Lenovo", "warranty_until": past, "status": "broken"})
	e.asset("MON", map[string]interface{}{"brand": "LG", "status": "repair"})
	e.do("POST", "/api/transactions", map[string]interface{}{"type": "out", "employee_id": emp, "items": []map[string]interface{}{{"asset_id": num(a["id"]), "inspection": inspLaptop()}}})

	code, m := e.do("GET", "/api/dashboard", nil)
	d := data(m)
	st := d["by_status"].(map[string]interface{})
	if code != 200 || num(d["total"]) != 3 || num(st["in_use"]) != 1 || num(st["broken"]) != 1 || num(st["repair"]) != 1 || num(st["available"]) != 0 {
		t.Fatalf("dashboard totals: %v", d)
	}
	if len(d["warranty_soon"].([]interface{})) != 1 || num(d["warranty_expired"]) != 1 || len(d["attention"].([]interface{})) != 2 {
		t.Fatalf("dashboard lists: %v", d)
	}
	cats := d["by_category"].([]interface{})
	if first := cats[0].(map[string]interface{}); first["name"] != "Laptop" || num(first["total"]) != 2 {
		t.Fatalf("by category: %v", cats[0])
	}
	if len(d["recent_logs"].([]interface{})) == 0 {
		t.Fatalf("recent logs empty")
	}

	res, b := e.get("/api/suggest?field=brand&search=len")
	if res.StatusCode != 200 || strings.TrimSpace(string(b)) != `["Lenovo"]` {
		t.Fatalf("suggest: %s", b)
	}
	if res, _ = e.get("/api/suggest?field=password"); res.StatusCode != 400 {
		t.Fatalf("suggest must whitelist fields: %d", res.StatusCode)
	}
}

func newReq(e *env, method, path string) (*http.Request, error) {
	return http.NewRequest(method, e.srv.URL+path, nil)
}
