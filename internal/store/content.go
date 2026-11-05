package store

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/threecredit/platform/internal/model"
	"golang.org/x/text/unicode/norm"
)

// ---------- posts ----------

type Post struct {
	ID          int64
	Slug        string
	Title       string
	Summary     string
	Body        string
	Category    string
	CoverPath   string
	Published   bool
	PublishedAt *time.Time
	UpdatedAt   time.Time
}

const postCols = `id,slug,title,summary,body,category,cover_path,published,published_at,updated_at`

func scanPost(row interface{ Scan(...any) error }) (Post, error) {
	var p Post
	err := row.Scan(&p.ID, &p.Slug, &p.Title, &p.Summary, &p.Body, &p.Category, &p.CoverPath, &p.Published, &p.PublishedAt, &p.UpdatedAt)
	return p, err
}

func (s *Store) Posts(onlyPublished bool, limit int) ([]Post, error) {
	q := `SELECT ` + postCols + ` FROM posts`
	if onlyPublished {
		q += ` WHERE published=1`
	}
	q += ` ORDER BY COALESCE(published_at, created_at) DESC LIMIT ?`
	rows, err := s.DB.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) PostBySlug(slug string) (Post, error) {
	return scanPost(s.DB.QueryRow(`SELECT `+postCols+` FROM posts WHERE slug=?`, slug))
}

func (s *Store) PostByID(id int64) (Post, error) {
	return scanPost(s.DB.QueryRow(`SELECT `+postCols+` FROM posts WHERE id=?`, id))
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify turns a Vietnamese title into a URL slug.
func Slugify(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "đ", "d"), "Đ", "D")
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	out := strings.Trim(slugRe.ReplaceAllString(b.String(), "-"), "-")
	if len(out) > 80 {
		out = strings.Trim(out[:80], "-")
	}
	return out
}

// SavePost creates (ID==0) or updates a post and returns its ID.
func (s *Store) SavePost(p Post, actor string) (int64, error) {
	p.Title = strings.TrimSpace(p.Title)
	if p.Title == "" {
		return 0, errors.New("cần tiêu đề")
	}
	if p.Slug = Slugify(p.Slug); p.Slug == "" {
		p.Slug = Slugify(p.Title)
	}
	if p.Category == "" {
		p.Category = "Kiến thức"
	}
	var pubAt any
	if p.Published {
		if p.PublishedAt == nil {
			now := time.Now().UTC()
			p.PublishedAt = &now
		}
		pubAt = *p.PublishedAt
	}
	if p.ID == 0 {
		res, err := s.DB.Exec(`INSERT INTO posts(slug,title,summary,body,category,cover_path,published,published_at) VALUES(?,?,?,?,?,?,?,?)`,
			p.Slug, p.Title, p.Summary, p.Body, p.Category, p.CoverPath, p.Published, pubAt)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return 0, errors.New("đường dẫn (slug) đã tồn tại")
			}
			return 0, err
		}
		id, _ := res.LastInsertId()
		s.Audit(actor, "create", "post", p.Slug, p.Title)
		return id, nil
	}
	_, err := s.DB.Exec(`UPDATE posts SET slug=?,title=?,summary=?,body=?,category=?,cover_path=?,published=?,published_at=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		p.Slug, p.Title, p.Summary, p.Body, p.Category, p.CoverPath, p.Published, pubAt, p.ID)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return 0, errors.New("đường dẫn (slug) đã tồn tại")
	}
	if err == nil {
		s.Audit(actor, "update", "post", p.Slug, p.Title)
	}
	return p.ID, err
}

func (s *Store) DeletePost(id int64, actor string) error {
	_, err := s.DB.Exec(`DELETE FROM posts WHERE id=?`, id)
	if err == nil {
		s.Audit(actor, "delete", "post", "", "")
	}
	return err
}

//go:embed posts_seed.json
var postsSeed []byte

// SeedPosts loads the starter articles once.
func (s *Store) SeedPosts() error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&n); err != nil || n > 0 {
		return err
	}
	var ps []Post
	if err := json.Unmarshal(postsSeed, &ps); err != nil {
		return err
	}
	for i, p := range ps {
		p.Published = true
		t := time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Hour)
		p.PublishedAt = &t
		if _, err := s.SavePost(p, "seed"); err != nil {
			return err
		}
	}
	return nil
}

// ---------- settings ----------

var SettingKeys = []struct{ Key, Label string }{
	{"company_name", "Tên pháp nhân vận hành 3Credit"},
	{"tax_id", "Mã số doanh nghiệp"},
	{"address", "Địa chỉ"},
	{"hotline", "Hotline"},
	{"email", "Email hỗ trợ"},
	{"dpo_email", "Email bảo vệ dữ liệu cá nhân"},
}

func (s *Store) Settings() map[string]string {
	m := map[string]string{}
	rows, err := s.DB.Query(`SELECT key,value FROM settings`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		m[k] = v
	}
	return m
}

func (s *Store) SaveSettings(m map[string]string, actor string) error {
	for _, k := range SettingKeys {
		v := strings.TrimSpace(m[k.Key])
		if len([]rune(v)) > 300 {
			return errors.New(k.Label + " quá dài")
		}
		if _, err := s.DB.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k.Key, v); err != nil {
			return err
		}
	}
	s.Audit(actor, "update", "settings", "", "")
	return nil
}

// ---------- products & partners (admin forms) ----------

var partnerIDRe = regexp.MustCompile(`^[a-z0-9-]{2,30}$`)

func (s *Store) CreatePartner(p model.Partner, actor string) error {
	if !partnerIDRe.MatchString(p.ID) || strings.TrimSpace(p.Name) == "" {
		return errors.New("mã đối tác gồm chữ thường, số, dấu gạch ngang; cần có tên")
	}
	if p.Group != "A" && p.Group != "B" {
		p.Group = "A"
	}
	_, err := s.DB.Exec(`INSERT INTO partners(id,name,legal_name,org_type,grp) VALUES(?,?,?,?,?)`, p.ID, strings.TrimSpace(p.Name), p.LegalName, p.OrgType, p.Group)
	if err == nil {
		s.Audit(actor, "create", "partner", p.ID, p.Name)
	}
	return err
}

var Categories = []struct{ V, L string }{
	{"cash_loan", "Vay tiêu dùng / tiền mặt"},
	{"app_loan", "Vay nhanh qua app"},
	{"credit_card", "Thẻ tín dụng"},
	{"secured_vehicle", "Vay bằng đăng ký xe"},
	{"installment", "Trả góp"},
}

// CreateProduct adds a product with an empty v1 policy and a draft campaign.
func (s *Store) CreateProduct(partnerID, name, category, campaignID, actor string) error {
	name = strings.TrimSpace(name)
	okCat := false
	for _, c := range Categories {
		if c.V == category {
			okCat = true
		}
	}
	if name == "" || !okCat {
		return errors.New("cần tên sản phẩm và loại hợp lệ")
	}
	var exists int
	s.DB.QueryRow(`SELECT COUNT(*) FROM partners WHERE id=?`, partnerID).Scan(&exists)
	if exists == 0 {
		return errors.New("đối tác không tồn tại")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	tx.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&n)
	pid := "P-" + strings.ToUpper(partnerID) + "-" + time.Now().Format("0601021504")
	if _, err := tx.Exec(`INSERT INTO products(id,partner_id,name,category) VALUES(?,?,?,?)`, pid, partnerID, name, category); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO policy_versions(product_id,version,rules_json,info_json,flags_json,source,verified_on,changed_by,change_note) VALUES(?,1,'{}','{}','[]','',?,?,?)`,
		pid, time.Now().Format("2006-01-02"), actor, "Tạo sản phẩm"); err != nil {
		return err
	}
	if campaignID == "" {
		campaignID = "C" + strings.TrimPrefix(pid, "P")
	}
	if _, err := tx.Exec(`INSERT INTO campaigns(id,product_id,name,status) VALUES(?,?,?,?)`, campaignID, pid, name, model.StatusDraft); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return errors.New("mã chiến dịch đã tồn tại")
		}
		return err
	}
	if _, err := tx.Exec(`INSERT INTO audit_log(actor,action,entity,entity_id,detail) VALUES(?,?,?,?,?)`, actor, "create", "product", pid, name); err != nil {
		return err
	}
	return tx.Commit()
}

// SetCampaignStatus is the quick on/off switch in the campaign list.
func (s *Store) SetCampaignStatus(id, status, actor string) error {
	c, err := s.Campaign(id)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("không tìm thấy chiến dịch")
	}
	if err != nil {
		return err
	}
	c.Status = status
	return s.SaveCampaign(c, actor)
}

type DeletionRow struct {
	ID          int64
	PhoneHash   string
	PhoneMasked string
	Note        string
	Status      string
	CreatedAt   time.Time
}

func (s *Store) DeletionRequests() ([]DeletionRow, error) {
	rows, err := s.DB.Query(`SELECT id,phone_hash,phone_masked,note,status,created_at FROM deletion_requests ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeletionRow
	for rows.Next() {
		var d DeletionRow
		rows.Scan(&d.ID, &d.PhoneHash, &d.PhoneMasked, &d.Note, &d.Status, &d.CreatedAt)
		out = append(out, d)
	}
	return out, rows.Err()
}
