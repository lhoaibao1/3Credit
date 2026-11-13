package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRegions(t *testing.T) {
	codes := RegionCodes()
	if len(codes) != 34 {
		t.Fatalf("want 34 provincial units, got %d", len(codes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if seen[c] || c == "other" {
			t.Fatalf("duplicate or legacy code offered: %q", c)
		}
		seen[c] = true
	}
	for _, c := range []string{"hn", "hcm", "hp", "dn", "ct", "hue"} {
		if !seen[c] {
			t.Fatalf("missing city code %q", c)
		}
	}
	cases := map[string]string{"hue": "hue", "vinhlong": "vinhlong", "hcm": "hcm", "other": "other", "": "hcm", "atlantis": "hcm"}
	for in, want := range cases {
		if got := ParseProfile(url.Values{"region": {in}}).Region; got != want {
			t.Errorf("region %q: got %q want %q", in, got, want)
		}
	}
	if Label("other") != "Tỉnh/thành khác" || Label("hue") != "Huế" || Label("daklak") != "Đắk Lắk" {
		t.Error("region labels")
	}
}

func TestInsuranceProfile(t *testing.T) {
	p := ParseProfile(url.Values{"social_ins": {"gt12"}, "life_ins": {"ge1y"}})
	if p.SocialIns != "gt12" || p.LifeIns != "ge1y" {
		t.Fatalf("parse: %+v", p)
	}
	if q := profileQuery(p); q.Get("social_ins") != "gt12" || q.Get("life_ins") != "ge1y" {
		t.Fatal("profileQuery drops insurance")
	}
	if p = ParseProfile(url.Values{"social_ins": {"x"}, "life_ins": {"forever"}}); p.SocialIns != "" || p.LifeIns != "" {
		t.Fatalf("unknown codes must be ignored: %+v", p)
	}
	if Label("social_ins:gt12") != "BHXH trên 12 tháng" || Label("life_ins:lt1y") != "BHNT đã đóng dưới 1 năm" {
		t.Error("insurance labels")
	}
	// "none" is also a credit-history code: insurance must not borrow its label ("Chưa từng vay")
	if Label("social_ins:none") != "Không tham gia BHXH" || Label("life_ins:none") != "Không có BHNT" || Label("none") != "Chưa từng vay" || Label("social_ins:") != "" {
		t.Errorf("insurance 'none' labels: %q %q %q", Label("social_ins:none"), Label("life_ins:none"), Label("none"))
	}
}

func TestProductFormsAreSeparate(t *testing.T) {
	e := setup(t)
	// each product has its own one-page form; nothing is matched before it is sent
	for path, must := range map[string][]string{
		"/vay-tien-mat":   {"Vay tiền mặt", `name="term"`, `name="social_ins"`},
		"/the-tin-dung":   {"Mở thẻ tín dụng", `name="social_ins"`},
		"/vay-dang-ky-xe": {"Vay bằng đăng ký xe", `name="vehicle"`, `name="term"`},
		"/so-sanh-tat-ca": {"So sánh tất cả", `name="vehicle"`, `name="term"`},
	} {
		code, body := e.get(t, path, false)
		if code != 200 || strings.Contains(body, `class="rsum"`) || strings.Contains(body, "Bước ") {
			t.Fatalf("%s: want a plain form, got %d", path, code)
		}
		for _, m := range must {
			if !strings.Contains(body, m) {
				t.Errorf("%s: missing %s", path, m)
			}
		}
	}
	if _, body := e.get(t, "/the-tin-dung", false); strings.Contains(body, `name="term"`) || strings.Contains(body, `name="vehicle"`) {
		t.Error("card form must not ask loan term or vehicle")
	}
	if _, body := e.get(t, "/vay-dang-ky-xe", false); strings.Contains(body, `value="none"`+" checked") || strings.Contains(body, `name="social_ins"`) {
		t.Error("vehicle form: no 'no vehicle' option, no insurance questions")
	}
	// the page decides the product even if the query says otherwise
	_, body := e.get(t, "/vay-tien-mat?need=credit_card&amount=50&term=24&social_ins=gt12&xem=1", false)
	if !strings.Contains(body, `class="rsum"`) || !strings.Contains(body, "24 tháng") {
		t.Fatal("sent form must show results and the profile summary")
	}
	if strings.Contains(body, "100% duyệt") || strings.Contains(body, "duyệt 100%\"") {
		t.Fatal("forbidden promise found")
	}
	// old links keep working
	req, _ := http.NewRequest("GET", e.ts.URL+"/?need=secured&amount=50&xem=1", nil)
	resp, err := e.cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); resp.StatusCode != http.StatusFound || !strings.HasPrefix(loc, "/vay-dang-ky-xe?") {
		t.Fatalf("old link: %d %q", resp.StatusCode, loc)
	}
	// the home page itself has no step-by-step form any more
	if _, body := e.get(t, "/", false); strings.Contains(body, `id="pf"`) || !strings.Contains(body, `href="/vay-tien-mat"`) {
		t.Fatal("home must link products to their own forms")
	}
}
