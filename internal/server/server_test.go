package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/threecredit/platform/internal/db"
	"github.com/threecredit/platform/internal/leads"
	"github.com/threecredit/platform/internal/store"
	"github.com/threecredit/platform/internal/tracking"
)

type env struct {
	ts  *httptest.Server
	cli *http.Client
	st  *store.Store
	lr  *leads.Router
	tok string // CMS session token for admin requests
}

func (e *env) auth(req *http.Request) { req.AddCookie(&http.Cookie{Name: cmsCookie, Value: e.tok}) }

func setup(t *testing.T) *env {
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
	lr := leads.New(d, "k")
	srv, err := New(Config{BaseURL: "http://test", DataDir: dir, SessionSecret: "s", AdminUser: "admin", AdminPassword: "pw"}, st, tracking.New(d, "salt"), lr)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if err := st.CreateCMSUser("admin", "test-password-1"); err != nil {
		t.Fatal(err)
	}
	tok, err := st.CMSLogin("admin", "test-password-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return &env{ts: ts, cli: cli, st: st, lr: lr, tok: tok}
}

func (e *env) get(t *testing.T, path string, admin bool) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", e.ts.URL+path, nil)
	if admin {
		e.auth(req)
	}
	resp, err := e.cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (e *env) post(t *testing.T, path string, form url.Values, admin bool) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", e.ts.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if admin {
		e.auth(req)
	}
	resp, err := e.cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

var csrfRe = regexp.MustCompile(`name="csrf" value="([0-9a-f]+)"`)

func TestCustomerPages(t *testing.T) {
	e := setup(t)
	code, body := e.get(t, "/", false)
	if code != 200 || !strings.Contains(body, "Tìm sản phẩm tài chính") || !strings.Contains(body, "/static/partners/fecredit.webp") {
		t.Fatalf("home: %d", code)
	}
	if strings.Contains(body, "100% duyệt") || strings.Contains(body, "duyệt 100%\"") {
		t.Fatal("forbidden promise found")
	}
	code, body = e.get(t, "/api/match?need=cash_loan&credit=bad_unsettled", false)
	if code != 200 || !strings.Contains(body, "Về hồ sơ nợ xấu chưa tất toán") {
		t.Fatalf("bad-debt notice missing: %d", code)
	}
	if strings.Contains(body, "st eligible\">Đáp ứng điều kiện đã biết</span></div>") {
		t.Fatal("bad-debt profile must have no eligible product")
	}
	if code, _ = e.get(t, "/san-pham/C-VIB-01?job=worker&channel=cash", false); code != 200 {
		t.Fatalf("detail: %d", code)
	}
	if code, _ = e.get(t, "/so-sanh?ids=C-VIB-01,C-SHF-01", false); code != 200 {
		t.Fatalf("compare: %d", code)
	}
	// pending campaign has no outbound link
	req, _ := http.NewRequest("GET", e.ts.URL+"/go/C-VIB-01", nil)
	resp, _ := e.cli.Do(req)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("pending campaign must not redirect out, got %d", resp.StatusCode)
	}
}

func TestAdminRequiresAuth(t *testing.T) {
	e := setup(t)
	if code, _ := e.get(t, "/admin", false); code != http.StatusSeeOther {
		t.Fatalf("got %d", code)
	}
	if code, _ := e.get(t, "/admin", true); code != 200 {
		t.Fatalf("got %d", code)
	}
	resp := e.post(t, "/admin/campaigns/C-VIB-01", url.Values{"status": {"active"}}, true)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without CSRF must be rejected, got %d", resp.StatusCode)
	}
}

func TestCampaignClickPostbackFlow(t *testing.T) {
	e := setup(t)
	_, page := e.get(t, "/admin/campaigns/C-SHF-01", true)
	m := csrfRe.FindStringSubmatch(page)
	if m == nil {
		t.Fatal("csrf token not found")
	}
	form := url.Values{"csrf": {m[1]}, "name": {"Vay tín chấp theo lương"}, "status": {"active"}, "priority": {"2"},
		"tracking_url": {"https://partner.example/aff?offer=9"}, "subid_param": {"aff_sub"}, "postback_secret": {"topsecret"}}
	if resp := e.post(t, "/admin/campaigns/C-SHF-01", form, true); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save: %d", resp.StatusCode)
	}
	_, body := e.get(t, "/api/match?need=cash_loan", false)
	if !strings.Contains(body, `href="/go/C-SHF-01"`) {
		t.Fatal("active campaign should show register button")
	}
	// clicking Register goes straight to the partner (no interstitial page), carrying only the tracking code
	goTo := func() *url.URL {
		resp, err := e.cli.Get(e.ts.URL + "/go/C-SHF-01")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("/go status = %d, want 303 to the partner", resp.StatusCode)
		}
		u, err := url.Parse(resp.Header.Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	dest := goTo()
	if dest.Host != "partner.example" || dest.Query().Get("offer") != "9" {
		t.Fatalf("redirect target = %s", dest)
	}
	clickID := dest.Query().Get("aff_sub")
	if !strings.HasPrefix(clickID, "clk_") {
		t.Fatalf("click id not in outbound URL: %s", dest)
	}
	for k := range dest.Query() {
		if k == "phone" || k == "name" || k == "email" {
			t.Fatal("PII in URL")
		}
	}
	// dedupe: same session → same click id
	if again := goTo(); again.Query().Get("aff_sub") != clickID {
		t.Fatal("click should be deduplicated within the window")
	}
	// postback: bad signature rejected, good one recorded once
	pb := func(sig string) int {
		v := url.Values{"click_id": {clickID}, "conversion_id": {"cv-1"}, "status": {"approved"}, "payout": {"150000"}, "sig": {sig}}
		resp, _ := http.Get(e.ts.URL + "/api/postback/accesstrade?" + v.Encode())
		resp.Body.Close()
		return resp.StatusCode
	}
	if pb("deadbeef") != 400 {
		t.Fatal("bad signature accepted")
	}
	good := tracking.Sign("topsecret", clickID, "cv-1", "approved", "150000")
	if pb(good) != 200 || pb(good) != 200 {
		t.Fatal("good postback rejected")
	}
	if n := e.st.Count(`SELECT COUNT(*) FROM conversions`); n != 1 {
		t.Fatalf("conversions = %d, want 1 (idempotent)", n)
	}
	if code, body := e.get(t, "/admin/tracking", true); code != 200 || !strings.Contains(body, "cv-1") {
		t.Fatal("tracking page should list the conversion")
	}
}

func TestLeadRoutingWithConsent(t *testing.T) {
	e := setup(t)
	var mu sync.Mutex
	var got []map[string]any
	var keys []string
	crm := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		got = append(got, m)
		keys = append(keys, r.Header.Get("Idempotency-Key")+"|"+r.Header.Get("X-Api-Key"))
		mu.Unlock()
		w.WriteHeader(201)
	}))
	defer crm.Close()
	e.lr.Client = crm.Client()

	_, page := e.get(t, "/admin/campaigns/C-VIB-01", true)
	tok := csrfRe.FindStringSubmatch(page)[1]
	e.post(t, "/admin/campaigns/C-VIB-01", url.Values{"csrf": {tok}, "name": {"Vay tiêu dùng tín chấp"}, "status": {"active"}, "priority": {"0"},
		"tracking_url": {"https://partner.example/vib"}, "lead_endpoint": {crm.URL + "/leads"}, "lead_auth_header": {"X-Api-Key: abc"}}, true)

	_, detail := e.get(t, "/san-pham/C-VIB-01", false)
	m := csrfRe.FindStringSubmatch(detail)
	if m == nil {
		t.Fatal("lead form should appear for a live campaign with an endpoint")
	}
	// without consent → rejected, nothing stored
	e.post(t, "/dang-ky/C-VIB-01", url.Values{"csrf": {m[1]}, "name": {"Nguyễn Văn A"}, "phone": {"0912345678"}}, false)
	if n := e.st.Count(`SELECT COUNT(*) FROM leads`); n != 0 {
		t.Fatalf("lead stored without consent")
	}
	for i := 0; i < 2; i++ { // second submit is idempotent
		e.post(t, "/dang-ky/C-VIB-01", url.Values{"csrf": {m[1]}, "name": {"Nguyễn Văn A"}, "phone": {"0912345678"}, "consent": {"yes"}}, false)
	}
	if n := e.st.Count(`SELECT COUNT(*) FROM leads`); n != 1 {
		t.Fatalf("leads = %d, want 1", n)
	}
	var enc []byte
	e.st.DB.QueryRow(`SELECT payload_enc FROM leads`).Scan(&enc)
	if strings.Contains(string(enc), "0912345678") {
		t.Fatal("phone stored in clear text")
	}
	if _, err := e.lr.ProcessDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["phone"] != "0912345678" || !strings.HasSuffix(keys[0], "|abc") {
		t.Fatalf("delivery wrong: %v %v", got, keys)
	}
	var status string
	e.st.DB.QueryRow(`SELECT status FROM leads`).Scan(&status)
	if status != "delivered" {
		t.Fatalf("status = %s", status)
	}
	if n := e.st.Count(`SELECT COUNT(*) FROM consents WHERE recipient='vib'`); n != 1 {
		t.Fatalf("consent rows = %d", n)
	}
}
