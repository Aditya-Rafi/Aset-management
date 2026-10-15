package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Aditya-Rafi/asset-management-system/backend/internal/auth"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/config"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/database"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/handlers"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/surel"
	"github.com/Aditya-Rafi/asset-management-system/backend/internal/platform/token"
)

// outbox is a fake mail queue that records what would have been sent.
type outbox struct {
	mu   sync.Mutex
	sent []surel.Surat
}

func (o *outbox) Masukkan(s surel.Surat) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, s)
	return true
}

func (o *outbox) last(to string) (surel.Surat, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i := len(o.sent) - 1; i >= 0; i-- {
		if o.sent[i].Ke == to {
			return o.sent[i], true
		}
	}
	return surel.Surat{}, false
}

// withMail turns on mail and guest mode, and gives the admin an address.
func (e *env) withMail() *outbox {
	e.t.Helper()
	ob := &outbox{}
	pt, err := token.NewPenanda(e.cfg.JWTSecret, 10*time.Minute)
	if err != nil {
		e.t.Fatal(err)
	}
	e.auth.PasangMandiri(auth.Mandiri{Surel: ob, PenandaTamu: pt, URLPublik: "http://app.test", Sinkron: true})
	if code, m := e.do("GET", "/api/pengguna", nil); code != 200 {
		e.t.Fatalf("list users: %d %v", code, m)
	}
	ref := e.adminRef()
	if code, m := e.do("PATCH", "/api/pengguna/"+ref, map[string]any{"email": "admin@kantor.test"}); code != 200 {
		e.t.Fatalf("set admin email: %d %v", code, m)
	}
	return ob
}

func (e *env) adminRef() string {
	e.t.Helper()
	res, b := e.get("/api/auth/saya")
	if res.StatusCode != 200 {
		e.t.Fatalf("saya: %d", res.StatusCode)
	}
	m := regexp.MustCompile(`"ref":"([^"]+)"`).FindStringSubmatch(string(b))
	return m[1]
}

func TestLoginByEmailAndUsername(t *testing.T) {
	e := newEnv(t)
	ob := e.withMail()
	_ = ob
	e.token = ""
	// Admin has an e-mail now; both identifiers work.
	for _, id := range []string{"admin", "ADMIN", "admin@kantor.test"} {
		if code, m := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": id, "sandi": testAdminPassword}); code != 200 || m["token"] == nil {
			t.Fatalf("login as %q: %d %v", id, code, m)
		}
	}
	// One generic message for unknown user and wrong password.
	_, a := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": "ghost", "sandi": "x-wrong-password"})
	_, b := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": "admin", "sandi": "x-wrong-password"})
	if str(a["galat"]) != str(b["galat"]) || str(a["galat"]) == "" {
		t.Fatalf("messages differ: %v vs %v", a, b)
	}
}

func TestLockout(t *testing.T) {
	e := newEnv(t)
	e.token = ""
	for i := 0; i < 5; i++ {
		e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": "admin", "sandi": "salah-salah-salah"})
	}
	if code, _ := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": "admin", "sandi": testAdminPassword}); code != 429 {
		t.Fatalf("expected 429 after 5 failures, got %d", code)
	}
}

func TestRefreshRotationAndReuse(t *testing.T) {
	e := newEnv(t)
	e.token = ""
	_, s1 := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": "admin", "sandi": testAdminPassword})
	r1 := str(s1["penyegar"])
	code, s2 := e.do("POST", "/api/auth/refresh", map[string]string{"penyegar": r1})
	if code != 200 || str(s2["penyegar"]) == r1 {
		t.Fatalf("refresh did not rotate: %d", code)
	}
	r2 := str(s2["penyegar"])
	// Replaying the old one revokes the whole family, so r2 dies too.
	if code, _ := e.do("POST", "/api/auth/refresh", map[string]string{"penyegar": r1}); code != 401 {
		t.Fatalf("reuse should be 401, got %d", code)
	}
	if code, _ := e.do("POST", "/api/auth/refresh", map[string]string{"penyegar": r2}); code != 401 {
		t.Fatalf("family should be revoked, got %d", code)
	}
}

func TestLogoutRevokesRefresh(t *testing.T) {
	e := newEnv(t)
	e.token = ""
	_, s := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": "admin", "sandi": testAdminPassword})
	r := str(s["penyegar"])
	e.token = str(s["token"])
	if code, _ := e.do("POST", "/api/auth/logout", map[string]string{"penyegar": r}); code != 204 {
		t.Fatalf("logout: %d", code)
	}
	if code, _ := e.do("POST", "/api/auth/refresh", map[string]string{"penyegar": r}); code != 401 {
		t.Fatalf("refresh after logout: %d", code)
	}
}

func TestPasswordChangeRevokesOtherSessions(t *testing.T) {
	e := newEnv(t)
	e.token = ""
	_, other := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": "admin", "sandi": testAdminPassword})
	e.login()
	if code, _ := e.do("PUT", "/api/auth/password", map[string]string{"current_password": testAdminPassword, "new_password": "another-long-pass-1"}); code != 200 {
		t.Fatalf("change: %d", code)
	}
	if code, _ := e.do("POST", "/api/auth/refresh", map[string]string{"penyegar": str(other["penyegar"])}); code != 401 {
		t.Fatalf("other device still signed in: %d", code)
	}
}

func TestTokenErrors(t *testing.T) {
	e := newEnv(t)
	e.token = "bukan.token.sah"
	if code, m := e.do("GET", "/api/assets", nil); code != 401 || str(m["galat"]) != "token tidak sah" {
		t.Fatalf("garbage token: %d %v", code, m)
	}
	// Identity headers from the client are ignored.
	e.token = ""
	req, _ := newReq(e, "GET", "/api/auth/saya")
	req.Header.Set("X-Aset-Pengguna", "usr_x")
	req.Header.Set("X-Aset-Peran", "admin")
	if code, _ := e.send(req); code != 401 {
		t.Fatalf("forged headers accepted: %d", code)
	}
	if code, m := e.do("GET", "/api/rute-ngawur", nil); code != 404 && code != 401 {
		t.Fatalf("unknown route: %d %v", code, m)
	}
}

func TestExpiredToken(t *testing.T) {
	e := newEnv(t)
	pen, _ := token.NewPenanda(e.cfg.JWTSecret, time.Second)
	// Issue a token that is already dead by signing with a 1s lifetime then waiting.
	tok, _, err := pen.Terbitkan("usr_x", "admin", "x", "jti_x")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2100 * time.Millisecond)
	e.token = tok
	if code, m := e.do("GET", "/api/assets", nil); code != 401 || str(m["galat"]) != "token kedaluwarsa" {
		t.Fatalf("expired: %d %v", code, m)
	}
}

func TestRegisterApproveLogin(t *testing.T) {
	e := newEnv(t)
	ob := e.withMail()
	e.token = ""
	code, m := e.do("POST", "/api/auth/daftar", map[string]string{
		"nama_lengkap": "Budi Teknisi", "email": "budi@kantor.test", "sandi": "rahasia-panjang-123", "peran": "teknisi"})
	if code != 202 && code != 200 && code != 201 {
		t.Fatalf("register: %d %v", code, m)
	}
	// Pending: correct password gives 403, not a session.
	if code, m := e.do("POST", "/api/auth/login", map[string]string{"nama_pengguna": "budi@kantor.test", "sandi": "rahasia-panjang-123"}); code != 403 {
		t.Fatalf("pending login: %d %v", code, m)
	}
	mail, ok := ob.last("admin@kantor.test")
	if !ok {
		t.Fatal("admin was not notified")
	}
	tok := regexp.MustCompile(`t=([A-Za-z0-9_-]+)&aksi=terima`).FindStringSubmatch(mail.Teks)
	if tok == nil {
		t.Fatalf("no approval link in mail:\n%s", mail.Teks)
	}
	if code, m := e.do("POST", "/api/auth/persetujuan/lihat", map[string]string{"token": tok[1]}); code != 200 || str(m["email"]) != "budi@kantor.test" && m["nama_lengkap"] == nil {
		t.Fatalf("lihat: %d %v", code, m)
	}
	if code, m := e.do("POST", "/api/auth/persetujuan", map[string]string{"token": tok[1], "keputusan": "terima"}); code != 200 {
		t.Fatalf("approve: %d %v", code, m)
	}
	// The link works once only.
	if code, _ := e.do("POST", "/api/auth/persetujuan", map[string]string{"token": tok[1], "keputusan": "terima"}); code == 200 {
		t.Fatal("approval link reused")
	}
	e.loginAs("budi@kantor.test", "rahasia-panjang-123")
	// operator: can write assets, cannot touch accounts.
	if code, _ := e.do("GET", "/api/pengguna", nil); code != 403 {
		t.Fatalf("operator listing users: %d", code)
	}
	if code, _ := e.do("POST", "/api/categories", map[string]any{"name": "Dummy", "code": "DMY"}); code != 201 {
		t.Fatalf("operator create category: %d", code)
	}
}

func TestOTPReset(t *testing.T) {
	e := newEnv(t)
	ob := e.withMail()
	e.token = ""
	// Unknown e-mail answers exactly like a known one.
	c1, m1 := e.do("POST", "/api/auth/lupa-sandi", map[string]string{"email": "tidak-ada@kantor.test"})
	c2, m2 := e.do("POST", "/api/auth/lupa-sandi", map[string]string{"email": "admin@kantor.test"})
	if c1 != c2 || str(m1["pesan"]) != str(m2["pesan"]) {
		t.Fatalf("enumeration leak: %d/%d %v %v", c1, c2, m1, m2)
	}
	mail, ok := ob.last("admin@kantor.test")
	if !ok {
		t.Fatal("no OTP mail")
	}
	kode := regexp.MustCompile(`Kode Anda: (\d{6})`).FindStringSubmatch(mail.Teks)[1]
	if code, _ := e.do("POST", "/api/auth/setel-ulang-sandi", map[string]string{"email": "admin@kantor.test", "kode": "000000", "sandi_baru": "sandi-baru-panjang-9"}); code != 400 {
		t.Fatalf("wrong code: %d", code)
	}
	if code, m := e.do("POST", "/api/auth/setel-ulang-sandi", map[string]string{"email": "admin@kantor.test", "kode": kode, "sandi_baru": "sandi-baru-panjang-9"}); code != 200 {
		t.Fatalf("reset: %d %v", code, m)
	}
	e.loginAs("admin", "sandi-baru-panjang-9")
}

func TestGuestTokenCannotReadData(t *testing.T) {
	e := newEnv(t)
	e.withMail()
	e.token = ""
	code, m := e.do("POST", "/api/auth/tamu", nil)
	if code != 200 || m["token"] == nil {
		t.Fatalf("guest: %d %v", code, m)
	}
	e.token = str(m["token"])
	for _, p := range []string{"/api/assets", "/api/employees", "/api/dashboard", "/api/pengguna", "/api/auth/saya"} {
		if code, _ := e.do("GET", p, nil); code != 403 {
			t.Fatalf("guest on %s: %d", p, code)
		}
	}
}

func TestRolesMatrix(t *testing.T) {
	e := newEnv(t)
	mk := func(user, role string) {
		if code, m := e.do("POST", "/api/pengguna", map[string]any{
			"nama_pengguna": user, "sandi": "rahasia-panjang-123", "nama_lengkap": user, "peran": role}); code != 201 {
			t.Fatalf("create %s: %d %v", user, code, m)
		}
	}
	mk("opr", "operator")
	mk("aud", "auditor")
	mk("vwr", "viewer")
	cases := []struct {
		user, method, path string
		want               int // 0 = anything but 401/403
	}{
		{"opr", "GET", "/api/assets/export", 0},
		{"aud", "GET", "/api/assets/export", 0},
		{"vwr", "GET", "/api/assets/export", 403},
		{"vwr", "GET", "/api/assets", 0},
		{"vwr", "POST", "/api/categories", 403},
		{"aud", "POST", "/api/categories", 403},
		{"opr", "DELETE", "/api/categories/1", 403},
		{"opr", "GET", "/api/pengguna", 403},
		{"vwr", "POST", "/api/transactions", 403},
	}
	for _, c := range cases {
		e.loginAs(c.user, "rahasia-panjang-123")
		code, _ := e.do(c.method, c.path, map[string]any{})
		if c.want == 0 && (code == 401 || code == 403) || c.want != 0 && code != c.want {
			t.Fatalf("%s %s %s: got %d want %d", c.user, c.method, c.path, code, c.want)
		}
	}
}

func TestBootstrapRules(t *testing.T) {
	e := newEnv(t)
	// Second call is a no-op: it must not reset the admin or create another.
	made, err := auth.Bootstrap(e.auth.Store, "admin", "something-else-123")
	if err != nil || made {
		t.Fatalf("bootstrap on populated table: made=%v err=%v", made, err)
	}
	e.login()
	// Fresh table: no default password, minimum length.
	e2 := newEnvNoAdmin(t)
	if _, err := auth.Bootstrap(e2.auth.Store, "admin", ""); err == nil {
		t.Fatal("empty password accepted")
	}
	if _, err := auth.Bootstrap(e2.auth.Store, "admin", "short"); err == nil || !strings.Contains(err.Error(), "12") {
		t.Fatalf("short password accepted: %v", err)
	}
}

// A database from the first version has users(name,email,password-bcrypt).
// It must upgrade in place, keep the admin able to sign in, and rehash the
// password to the new scheme on that first sign-in.
func TestLegacyUsersUpgrade(t *testing.T) {
	cfg := &config.Config{DBDriver: "sqlite", DBSQLitePath: t.TempDir() + "/old.db", JWTSecret: "test-secret-test-secret-0123456789", AccessTokenTTL: time.Minute}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), 4)
	for _, q := range []string{
		"CREATE TABLE users (id integer primary key autoincrement, name text, email text, password text, created_at datetime, updated_at datetime)",
		"CREATE UNIQUE INDEX idx_users_email ON users(email)",
	} {
		if err := db.Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("INSERT INTO users (name,email,password,created_at,updated_at) VALUES (?,?,?,?,?)",
		"Admin IT", "admin@mindotek.com", string(hash), time.Now(), time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	pen, _ := token.NewPenanda(cfg.JWTSecret, cfg.AccessTokenTTL)
	as := auth.NewServer(db, pen)
	router, err := handlers.NewRouter(cfg, db, as)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router)
	defer srv.Close()
	e := &env{t: t, srv: srv, db: db, auth: as, cfg: cfg}
	e.c = &http.Client{Transport: bearer{e}}

	e.loginAs("admin@mindotek.com", "password")
	var stored string
	if err := db.Raw("SELECT password FROM users WHERE email = ?", "admin@mindotek.com").Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(stored, "$2") {
		t.Fatal("legacy bcrypt hash was not upgraded on sign-in")
	}
	e.loginAs("admin", "password") // the derived username works as well
	if code, _ := e.do("GET", "/api/pengguna", nil); code != 200 {
		t.Fatalf("legacy admin is not admin: %d", code)
	}
}
