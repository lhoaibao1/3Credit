package server

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/threecredit/platform/internal/db"
	"github.com/threecredit/platform/internal/leads"
	"github.com/threecredit/platform/internal/store"
	"github.com/threecredit/platform/internal/tracking"
)

func cmsSetup(t *testing.T, cfg Config) (*httptest.Server, *http.Client, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	st := store.New(d)
	if err := st.Seed(); err != nil {
		t.Fatal(err)
	}
	cfg.DataDir, cfg.SessionSecret = dir, "s"
	srv, err := New(cfg, st, tracking.New(d, "salt"), leads.New(d, "k"))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	return ts, &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, st
}

func formPost(t *testing.T, c *http.Client, u string, v url.Values) *http.Response {
	t.Helper()
	r, err := c.PostForm(u, v)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func csrfOf(t *testing.T, c *http.Client, u string) string {
	t.Helper()
	r, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b := new(strings.Builder)
	buf := make([]byte, 1<<16)
	for {
		n, err := r.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	m := csrfRe.FindStringSubmatch(b.String())
	if m == nil {
		t.Fatalf("no csrf on %s (%d)", u, r.StatusCode)
	}
	return m[1]
}

func TestCMSFirstRunSetupAndLogin(t *testing.T) {
	ts, c, st := cmsSetup(t, Config{BaseURL: "http://test", AdminPassword: "setup-code-123"})
	// no account yet: login sends you to setup
	if r, _ := c.Get(ts.URL + "/login"); r.StatusCode != http.StatusSeeOther || r.Header.Get("Location") != "/setup" {
		t.Fatalf("login without accounts: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	tok := csrfOf(t, c, ts.URL+"/setup")
	// wrong setup code is refused
	formPost(t, c, ts.URL+"/setup", url.Values{"csrf": {tok}, "code": {"nope"}, "username": {"bao"}, "password": {"mat-khau-dai-123"}, "password2": {"mat-khau-dai-123"}})
	if n, _ := st.CMSUserCount(); n != 0 {
		t.Fatal("account created with a wrong setup code")
	}
	r := formPost(t, c, ts.URL+"/setup", url.Values{"csrf": {tok}, "code": {"setup-code-123"}, "username": {"Bao"}, "password": {"mat-khau-dai-123"}, "password2": {"mat-khau-dai-123"}})
	if r.StatusCode != http.StatusSeeOther || r.Header.Get("Location") != "/admin" {
		t.Fatalf("setup: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	if r, _ := c.Get(ts.URL + "/admin"); r.StatusCode != 200 {
		t.Fatalf("logged in after setup: %d", r.StatusCode)
	}
	// setup is closed once an account exists
	if r, _ := c.Get(ts.URL + "/setup"); r.Header.Get("Location") != "/login" {
		t.Fatal("setup must close after the first account")
	}
	if err := st.CreateFirstCMSUser("other", "mat-khau-dai-456"); err == nil {
		t.Fatal("second first-account must fail")
	}
	// logout then login with a wrong / right password
	formPost(t, c, ts.URL+"/logout", url.Values{"csrf": {tok}})
	if r, _ := c.Get(ts.URL + "/admin"); r.StatusCode != http.StatusSeeOther {
		t.Fatal("still logged in after logout")
	}
	ltok := csrfOf(t, c, ts.URL+"/login")
	formPost(t, c, ts.URL+"/login", url.Values{"csrf": {ltok}, "username": {"bao"}, "password": {"sai"}})
	if r, _ := c.Get(ts.URL + "/admin"); r.StatusCode != http.StatusSeeOther {
		t.Fatal("wrong password must not log in")
	}
	r = formPost(t, c, ts.URL+"/login", url.Values{"csrf": {ltok}, "username": {"bao"}, "password": {"mat-khau-dai-123"}, "next": {"//evil.com"}})
	if r.Header.Get("Location") != "/admin" {
		t.Fatalf("open redirect after login: %s", r.Header.Get("Location"))
	}
	if r, _ := c.Get(ts.URL + "/admin/users"); r.StatusCode != 200 {
		t.Fatalf("accounts page: %d", r.StatusCode)
	}
}

func TestCMSHostRouting(t *testing.T) {
	ts, c, _ := cmsSetup(t, Config{BaseURL: "https://3credit.asia", CMSHost: "cms.3credit.asia"})
	get := func(host, path string) *http.Response {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.Host = host
		r, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r
	}
	if r := get("3credit.asia", "/admin/campaigns"); r.StatusCode != http.StatusFound || r.Header.Get("Location") != "https://cms.3credit.asia/admin/campaigns" {
		t.Fatalf("main host /admin: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	if r := get("cms.3credit.asia", "/"); r.Header.Get("Location") != "/admin" {
		t.Fatalf("cms root: %s", r.Header.Get("Location"))
	}
	if r := get("cms.3credit.asia", "/kien-thuc"); r.Header.Get("Location") != "/admin" {
		t.Fatal("public pages must not be served on the CMS host")
	}
	if r := get("cms.3credit.asia", "/setup"); r.StatusCode != 200 {
		t.Fatalf("cms setup: %d", r.StatusCode)
	}
	if r := get("3credit.asia", "/"); r.StatusCode != 200 {
		t.Fatalf("public home: %d", r.StatusCode)
	}
}

func TestVersionEndpointReportsBuild(t *testing.T) {
	ts, c, _ := cmsSetup(t, Config{BaseURL: "http://test", Version: "abc1234"})
	r, err := c.Get(ts.URL + "/version")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b := make([]byte, 32)
	n, _ := r.Body.Read(b)
	if r.StatusCode != 200 || string(b[:n]) != "abc1234" {
		t.Fatalf("/version = %d %q", r.StatusCode, b[:n])
	}
}
