package server

import (
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
	if Label("gt12") != "BHXH trên 12 tháng" || Label("lt1y") != "BHNT đã đóng dưới 1 năm" {
		t.Error("insurance labels")
	}
}

func TestHomeShowsResultsOnlyAfterFlow(t *testing.T) {
	e := setup(t)
	_, body := e.get(t, "/?need=cash_loan&amount=50&term=24", false)
	if strings.Contains(body, `class="rsum"`) || strings.Contains(body, `class="card"`) || !strings.Contains(body, `data-start="2"`) {
		t.Fatal("results must not show before the question flow is finished")
	}
	_, body = e.get(t, "/?need=cash_loan&amount=50&term=24&social_ins=gt12&xem=1", false)
	if !strings.Contains(body, `class="rsum"`) || !strings.Contains(body, "BHXH trên 12 tháng") {
		t.Fatal("finished flow must show results and the profile summary")
	}
	if strings.Contains(body, "100% duyệt") || strings.Contains(body, "duyệt 100%\"") {
		t.Fatal("forbidden promise found")
	}
}
