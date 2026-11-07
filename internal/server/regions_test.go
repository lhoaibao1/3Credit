package server

import (
	"net/url"
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
