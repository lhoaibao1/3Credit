package store

// 3Credit is a product of 3RDVN Technology (Công ty TNHH Công nghệ 3RDVN),
// the same company that builds and operates 3RDHub Vietnam.
// Source: 3rdvn.io.vn / 3rdhubvietnam.tech (organization data), 05/11/2026.

// CompanyDefaults fill empty settings on startup. Values set by an admin are never overwritten.
var CompanyDefaults = map[string]string{
	"company_name": "Công ty TNHH Công nghệ 3RDVN (3RDVN Technology)",
	"tax_id":       "4001342613",
	"address":      "Tổ 10 Tỉnh Thủy, Phường Quảng Phú, Thành phố Đà Nẵng",
	"hotline":      "(+84) 398 192 414",
	"email":        "admin_support@3rdvn.io.vn",
	"dpo_email":    "admin_support@3rdvn.io.vn",
}

type EcoMember struct {
	Name, Role, Desc, URL, Logo string
	Current                     bool
}

// Parent is the company that owns and operates 3Credit.
var Parent = struct{ Name, Legal, Tagline, Desc, URL, Logo, Icon string }{
	Name: "3RDVN Technology", Legal: "Công ty TNHH Công nghệ 3RDVN",
	Tagline: "Connecting Resources. Accelerating Growth.",
	Desc:    "Đối tác dịch vụ tài chính, công nghệ và vận hành. 3RDVN Technology xây dựng và vận hành các sản phẩm trong hệ sinh thái, trong đó có 3RDHub Vietnam và 3Credit.",
	URL:     "https://3rdvn.io.vn", Logo: "/static/eco/3rdvn-icon.webp", Icon: "/static/eco/3rdvn-icon.webp",
}

// Ecosystem lists the products of 3RDVN Technology shown on 3Credit.
var Ecosystem = []EcoMember{
	{Name: "3RDHub Vietnam", Role: "Tiếp thị liên kết", Desc: "Nền tảng tiếp thị liên kết, minh bạch từng lượt bấm, từng hồ sơ và từng đồng hoa hồng.", URL: "https://3rdhubvietnam.tech", Logo: "/static/eco/3rdhub-icon.webp"},
	{Name: "3Credit", Role: "So sánh tài chính", Desc: "Tìm và so sánh sản phẩm tài chính theo hồ sơ, có giải thích vì sao phù hợp.", URL: "/", Logo: "/static/brand/3credit-icon.svg", Current: true},
	{Name: "SaaS 3RDVN HUB", Role: "Phần mềm phân phối", Desc: "Phần mềm vận hành phân phối, chiến dịch và đối soát hoa hồng, dùng ngay không cần xây.", URL: "https://demo.3rdvn.io.vn", Logo: "/static/eco/3rdvn-hub-icon.webp"},
}

// EcoServices are the services 3RDVN Technology provides.
var EcoServices = []struct{ Name, Desc string }{
	{"Thiết kế Website / CMS", "Website đẹp, nhanh, tự quản trị nội dung"},
	{"Xây dựng CRM / ERP", "Phần mềm quản trị theo đúng quy trình của doanh nghiệp"},
	{"Mail Server doanh nghiệp", "Email theo tên miền riêng"},
}

// SeedEcosystem is idempotent: fills missing partner logos and empty company settings.
func (s *Store) SeedEcosystem() error {
	logos := map[string]string{
		"lotte":    "/static/partners/lotte.webp",
		"mafc":     "/static/partners/mirae.webp",
		"sfinance": "/static/partners/sfinance.webp",
		"tpbank":   "/static/partners/tpbank.webp",
	}
	for id, p := range logos {
		if _, err := s.DB.Exec(`UPDATE partners SET logo_path=? WHERE id=? AND logo_path=''`, p, id); err != nil {
			return err
		}
	}
	// FE CREDIT: switch the small favicon to the full logo, unless an admin already uploaded one
	if _, err := s.DB.Exec(`UPDATE partners SET logo_path='/static/partners/fecredit.webp' WHERE id='fecredit' AND logo_path='/static/partners/fecredit.svg'`); err != nil {
		return err
	}
	// Remove partners that an earlier version copied from the 3RDHub partner list,
	// unless an admin has since attached products to them.
	if _, err := s.DB.Exec(`DELETE FROM partners WHERE id IN ('tnex','hoangthien') AND id NOT IN (SELECT partner_id FROM products)`); err != nil {
		return err
	}
	for k, v := range CompanyDefaults {
		if _, err := s.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value WHERE settings.value=''`, k, v); err != nil {
			return err
		}
	}
	return nil
}
