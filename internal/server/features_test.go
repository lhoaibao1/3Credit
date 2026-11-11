package server

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func (e *env) csrfFrom(t *testing.T, path string, admin bool) string {
	t.Helper()
	_, page := e.get(t, path, admin)
	m := csrfRe.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no csrf token on %s", path)
	}
	return m[1]
}

func TestInquiryFlow(t *testing.T) {
	e := setup(t)
	tok := e.csrfFrom(t, "/", false)
	base := url.Values{"csrf": {tok}, "name": {"Trần Thị B"}, "phone": {"0987 654 321"}, "message": {"=HYPERLINK(\"x\")"},
		"profile": {"need=cash_loan&amount=30&term=12&job=worker&income=6&channel=cash&credit=good"}, "back": {"/"}, "source": {"trang-chu"}}
	// no consent → nothing stored
	e.post(t, "/tu-van", base, false)
	if n := e.st.Count(`SELECT COUNT(*) FROM inquiries`); n != 0 {
		t.Fatal("stored without consent")
	}
	base.Set("consent", "yes")
	e.post(t, "/tu-van", base, false)
	e.post(t, "/tu-van", base, false) // same day duplicate
	if n := e.st.Count(`SELECT COUNT(*) FROM inquiries`); n != 1 {
		t.Fatalf("inquiries = %d, want 1", n)
	}
	var enc []byte
	e.st.DB.QueryRow(`SELECT payload_enc FROM inquiries`).Scan(&enc)
	if strings.Contains(string(enc), "0987654321") || strings.Contains(string(enc), "Trần") {
		t.Fatal("inquiry stored in clear text")
	}
	code, page := e.get(t, "/admin/inquiries", true)
	if code != 200 || !strings.Contains(page, "0987654321") || !strings.Contains(page, "Trần Thị B") {
		t.Fatalf("admin list should show decrypted contact: %d", code)
	}
	// CSV export: BOM, phone kept as text, formula neutralised
	req, _ := http.NewRequest("GET", e.ts.URL+"/admin/inquiries.csv", nil)
	e.auth(req)
	resp, _ := e.cli.Do(req)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	csv := string(b)
	if !strings.HasPrefix(csv, "\xEF\xBB\xBF") || !strings.Contains(csv, "'0987654321") || !strings.Contains(csv, `"'=HYPERLINK`) {
		t.Fatalf("bad csv: %q", csv[:min(300, len(csv))])
	}
	// status update
	tok = e.csrfFrom(t, "/admin/inquiries", true)
	e.post(t, "/admin/inquiries/1", url.Values{"csrf": {tok}, "status": {"contacted"}, "note": {"Gọi lúc 10h"}}, true)
	var st, note string
	e.st.DB.QueryRow(`SELECT status,note FROM inquiries WHERE id=1`).Scan(&st, &note)
	if st != "contacted" || note != "Gọi lúc 10h" {
		t.Fatalf("update failed: %s %s", st, note)
	}
}

func TestDeletionErasesInquiry(t *testing.T) {
	e := setup(t)
	tok := e.csrfFrom(t, "/", false)
	e.post(t, "/tu-van", url.Values{"csrf": {tok}, "name": {"Lê C"}, "phone": {"0911222333"}, "consent": {"yes"}, "back": {"/"}}, false)
	tok = e.csrfFrom(t, "/xoa-du-lieu", false)
	e.post(t, "/xoa-du-lieu", url.Values{"csrf": {tok}, "phone": {"0911222333"}}, false)
	var masked string
	e.st.DB.QueryRow(`SELECT phone_masked FROM deletion_requests`).Scan(&masked)
	if masked != "091****333" {
		t.Fatalf("masked = %q", masked)
	}
	tok = e.csrfFrom(t, "/admin/deletions", true)
	e.post(t, "/admin/deletions/1", url.Values{"csrf": {tok}}, true)
	if n := e.st.Count(`SELECT COUNT(*) FROM inquiries`); n != 0 {
		t.Fatal("inquiry not erased")
	}
}

func TestContentAndLegalPages(t *testing.T) {
	e := setup(t)
	if err := e.st.SeedPosts(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/kien-thuc", "/kien-thuc/no-xau-la-gi-5-nhom-no-va-cach-xu-ly", "/dieu-khoan", "/chinh-sach", "/chinh-sach/chinh-sach-bao-mat", "/chinh-sach/quy-che-hoat-dong", "/tieu-chi-doi-tac", "/xoa-du-lieu"} {
		if code, _ := e.get(t, p, false); code != 200 {
			t.Errorf("%s = %d", p, code)
		}
	}
	_, body := e.get(t, "/kien-thuc/no-xau-la-gi-5-nhom-no-va-cach-xu-ly", false)
	if !strings.Contains(body, "<h2>5 nhóm nợ</h2>") || !strings.Contains(body, "<strong>Nhóm 3") {
		t.Fatal("markdown not rendered")
	}
	_, home := e.get(t, "/", false)
	if !strings.Contains(home, `href="/kien-thuc/`) || !strings.Contains(home, `id="tu-van"`) || !strings.Contains(home, `href="/dieu-khoan"`) {
		t.Fatal("home should link posts, consult form and legal pages")
	}
	// admin writes a post; script tags must be escaped
	tok := e.csrfFrom(t, "/admin/posts/new", true)
	e.post(t, "/admin/posts/new", url.Values{"csrf": {tok}, "title": {"Thẻ tín dụng là gì?"}, "body": {"Đoạn 1\n\n<script>alert(1)</script>"}, "published": {"1"}}, true)
	code, page := e.get(t, "/kien-thuc/the-tin-dung-la-gi", false)
	if code != 200 || strings.Contains(page, "<script>alert") {
		t.Fatalf("post page %d or unescaped script", code)
	}
	// settings appear in footer
	tok = e.csrfFrom(t, "/admin/settings", true)
	e.post(t, "/admin/settings", url.Values{"csrf": {tok}, "company_name": {"Công ty TNHH 3Credit"}, "hotline": {"1900 0000"}}, true)
	if _, home = e.get(t, "/", false); !strings.Contains(home, "Công ty TNHH 3Credit") {
		t.Fatal("company name not in footer")
	}
}

func TestPolicyFormAndQuickToggle(t *testing.T) {
	e := setup(t)
	// quick toggle without URL is refused
	tok := e.csrfFrom(t, "/admin/campaigns", true)
	e.post(t, "/admin/campaigns/C-SFI-01/status", url.Values{"csrf": {tok}, "status": {"active"}}, true)
	var st string
	e.st.DB.QueryRow(`SELECT status FROM campaigns WHERE id='C-SFI-01'`).Scan(&st)
	if st == "active" {
		t.Fatal("activated without tracking URL")
	}
	// fill SFinance policy via the form → it becomes eligible for an office profile
	tok = e.csrfFrom(t, "/admin/policies/P-SFI-01", true)
	form := url.Values{"csrf": {tok},
		"amount_min": {"10"}, "amount_max": {"100"}, "amount_conf": {"A"},
		"term_min": {"6"}, "term_max": {"36"}, "term_conf": {"A"},
		"income_min": {"5"}, "income_conf": {"A"},
		"info_rate": {"Từ 2%/tháng"}, "info_rate_conf": {"A"},
		"source": {"Hợp đồng affiliate 11/2026"}, "verified_on": {"2026-11-05"}, "change_note": {"Nhập theo tài liệu đối tác"}}
	e.post(t, "/admin/policies/P-SFI-01", form, true)
	var v int
	e.st.DB.QueryRow(`SELECT MAX(version) FROM policy_versions WHERE product_id='P-SFI-01'`).Scan(&v)
	if v != 2 {
		t.Fatalf("version = %d, want 2", v)
	}
	// customers only see campaigns switched on in the CMS
	e.st.DB.Exec(`UPDATE campaigns SET status='active', tracking_url='https://partner.example/aff?offer=1', target_segments='["SEG01","SEG02","SEG03","SEG04","SEG05","SEG06","SEG07","SEG08","SEG09"]' WHERE id='C-SFI-01'`)
	_, res := e.get(t, "/api/match?need=cash_loan&amount=50&term=24&income=12", false)
	i := strings.Index(res, "Từ 2%/tháng")
	if i < 0 {
		t.Fatal("new policy info not shown")
	}
	// bad input: max < min is rejected, no new version
	form.Set("amount_max", "5")
	form.Set("csrf", e.csrfFrom(t, "/admin/policies/P-SFI-01", true))
	e.post(t, "/admin/policies/P-SFI-01", form, true)
	e.st.DB.QueryRow(`SELECT MAX(version) FROM policy_versions WHERE product_id='P-SFI-01'`).Scan(&v)
	if v != 2 {
		t.Fatal("invalid policy saved")
	}
	// new product + campaign
	tok = e.csrfFrom(t, "/admin/campaigns", true)
	e.post(t, "/admin/products/new", url.Values{"csrf": {tok}, "partner_id": {"vib"}, "name": {"Thẻ tín dụng VIB"}, "category": {"credit_card"}, "campaign_id": {"C-VIB-CARD"}}, true)
	if n := e.st.Count(`SELECT COUNT(*) FROM campaigns WHERE id='C-VIB-CARD' AND status='draft'`); n != 1 {
		t.Fatal("new campaign not created as draft")
	}
}

func TestCustomersOnlySeeCampaignsSwitchedOn(t *testing.T) {
	e := setup(t)
	_, none := e.get(t, "/api/match?need=cash_loan&amount=50&term=24", false)
	if strings.Contains(none, "<article") || !strings.Contains(none, "Chưa tìm thấy sản phẩm") {
		t.Fatal("with no campaign switched on, results must be empty")
	}
	if !strings.Contains(none, "0 sản phẩm được đối chiếu") {
		t.Fatal("summary must count only visible products")
	}
	e.st.DB.Exec(`UPDATE campaigns SET status='active', tracking_url='https://partner.example/aff', target_segments='["SEG01","SEG02","SEG03","SEG04","SEG05","SEG06","SEG07","SEG08","SEG09"]' WHERE id IN ('C-SFI-01','C-VIB-01')`)
	_, two := e.get(t, "/api/match?need=cash_loan&amount=50&term=24", false)
	if n := strings.Count(two, "<article"); n != 2 || !strings.Contains(two, "2 sản phẩm được đối chiếu") {
		t.Fatalf("want exactly the 2 switched-on products, got %d cards", n)
	}
	if strings.Contains(two, `href="/go/C-FEC-01"`) {
		t.Fatal("a campaign that is off must not appear")
	}
	// paused again → gone
	e.st.DB.Exec(`UPDATE campaigns SET status='paused' WHERE id='C-VIB-01'`)
	_, one := e.get(t, "/api/match?need=cash_loan&amount=50&term=24", false)
	if n := strings.Count(one, "<article"); n != 1 {
		t.Fatalf("paused campaign still shown (%d cards)", n)
	}
}

func TestTargetSegmentsDecideWhoSeesACampaign(t *testing.T) {
	e := setup(t)
	has := func(q string) bool {
		_, b := e.get(t, "/api/match?need=cash_loan&amount=50&term=24&income=12&"+q, false)
		return strings.Contains(b, `href="/go/C-SFI-01"`)
	}
	// switched on, but no target group chosen yet: nobody sees it
	e.st.DB.Exec(`UPDATE campaigns SET status='active', tracking_url='https://partner.example/aff', target_segments='[]' WHERE id='C-SFI-01'`)
	if has("credit=bad_unsettled") || has("credit=good") {
		t.Fatal("a campaign with no target group must stay hidden")
	}
	// aimed at the good-credit group: only customers in that group see it
	e.st.DB.Exec(`UPDATE campaigns SET target_segments='["SEG05"]' WHERE id='C-SFI-01'`)
	if !has("credit=good") {
		t.Fatal("good-credit customer should see the campaign aimed at SEG05")
	}
	if has("credit=none") {
		t.Fatal("a customer outside the target group must not see it")
	}
}

func TestBadDebtCustomersOnlySeeProductsSetToAcceptIt(t *testing.T) {
	e := setup(t)
	e.st.DB.Exec(`UPDATE campaigns SET status='active', tracking_url='https://partner.example/aff', target_segments='["SEG01","SEG02","SEG03","SEG04","SEG05","SEG06","SEG07","SEG08","SEG09"]' WHERE id='C-SFI-01'`)
	has := func(credit string) bool {
		_, b := e.get(t, "/api/match?need=cash_loan&amount=50&term=24&income=12&credit="+credit, false)
		return strings.Contains(b, `href="/go/C-SFI-01"`)
	}
	// nothing set in the CMS about bad debt: a customer with bad debt must not be offered this product
	if has("bad_unsettled") || has("group2") {
		t.Fatal("product not set to accept bad debt was offered to a bad-debt customer")
	}
	if !has("good") {
		t.Fatal("customers with good credit are not filtered by this rule")
	}
	// admin ticks "Đang có nợ xấu chưa tất toán" in the policy form and picks a source
	tok := e.csrfFrom(t, "/admin/policies/P-SFI-01", true)
	e.post(t, "/admin/policies/P-SFI-01", url.Values{"csrf": {tok},
		"amount_min": {"10"}, "amount_max": {"100"}, "amount_conf": {"A"},
		"credit": {"bad_unsettled"}, "credit_conf": {"B"},
		"source": {"Hợp đồng"}, "verified_on": {"2026-11-05"}, "change_note": {"nhận nợ xấu"}}, true)
	if !has("bad_unsettled") {
		t.Fatal("product set to accept bad_unsettled should be offered")
	}
	if has("group2") || has("bad_settled") {
		t.Fatal("only the ticked debt type is accepted, not the others")
	}
	// the CMS list says which debt types each product accepts
	_, list := e.get(t, "/admin/campaigns", true)
	if !strings.Contains(list, "Đang có nợ xấu chưa tất toán") {
		t.Fatal("campaign list should show the debt types a product accepts")
	}
}

func TestPostbackWithTokenAsAffiliateNetworksSendIt(t *testing.T) {
	e := setup(t)
	// saving a tracking link creates the postback secret and the CMS shows a ready URL
	tok := e.csrfFrom(t, "/admin/campaigns/C-SFI-01", true)
	e.post(t, "/admin/campaigns/C-SFI-01", url.Values{"csrf": {tok}, "name": {"SFinance"}, "status": {"active"}, "priority": {"1"},
		"segments": {"SEG01", "SEG05"}, "tracking_url": {"https://partner.example/aff?offer=9"}, "subid_param": {"aff_sub"}}, true)
	var secret string
	e.st.DB.QueryRow(`SELECT postback_secret FROM campaigns WHERE id='C-SFI-01'`).Scan(&secret)
	if len(secret) < 16 {
		t.Fatalf("secret not generated: %q", secret)
	}
	if _, page := e.get(t, "/admin/campaigns/C-SFI-01", true); !strings.Contains(page, "/api/postback/") || !strings.Contains(page, "token="+secret) {
		t.Fatal("CMS should show the postback URL with the token")
	}
	resp, _ := e.cli.Get(e.ts.URL + "/go/C-SFI-01")
	resp.Body.Close()
	u, _ := url.Parse(resp.Header.Get("Location"))
	click := u.Query().Get("aff_sub")
	call := func(q string) int {
		r, _ := http.Get(e.ts.URL + "/api/postback/accesstrade?click_id=" + click + "&" + q)
		r.Body.Close()
		return r.StatusCode
	}
	if call("conversion_id=tx1&status=success&payout=120000&token=wrong") != 400 {
		t.Fatal("wrong token accepted")
	}
	if call("conversion_id=tx1&status=hold&payout=120000") != 400 {
		t.Fatal("postback without token or signature accepted")
	}
	if call("conversion_id=tx1&status=hold&payout=120000&token="+secret) != 200 || call("conversion_id=tx1&status=Success&payout=120000&token="+secret) != 200 {
		t.Fatal("valid token postback rejected")
	}
	var st string
	var n int
	e.st.DB.QueryRow(`SELECT COUNT(*), MAX(status) FROM conversions WHERE conversion_id='tx1'`).Scan(&n, &st)
	if n != 1 || st != "approved" {
		t.Fatalf("conversion rows=%d status=%q, want 1 approved", n, st)
	}
	// the token must never be written to the postback log
	if e.st.Count(`SELECT COUNT(*) FROM postback_logs WHERE query LIKE '%`+secret+`%'`) != 0 {
		t.Fatal("token leaked into postback log")
	}
}
