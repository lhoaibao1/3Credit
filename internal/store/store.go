// Package store is the data access layer (partners, products, versioned
// policies, campaigns, segments, analytics) plus the initial seed.
package store

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/threecredit/platform/internal/model"
	"github.com/threecredit/platform/internal/rules"
)

type Store struct{ DB *sql.DB }

func New(db *sql.DB) *Store { return &Store{DB: db} }

//go:embed seed.json
var seedJSON []byte

type seedFile struct {
	VerifiedOn string          `json:"verified_on"`
	Partners   []model.Partner `json:"partners"`
	Products   []struct {
		model.Product
		Rules  model.Rules `json:"rules"`
		Info   model.Info  `json:"info"`
		Flags  []string    `json:"flags"`
		Source string      `json:"source"`
	} `json:"products"`
	Segments []struct {
		ID   string          `json:"id"`
		Name string          `json:"name"`
		Rule json.RawMessage `json:"rule"`
	} `json:"segments"`
}

// Seed loads research data into an empty database. Campaigns start as
// "pending" with no tracking URL: 3Credit never invents partner links.
func (s *Store) Seed() error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM partners`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	var f seedFile
	if err := json.Unmarshal(seedJSON, &f); err != nil {
		return err
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range f.Partners {
		if _, err := tx.Exec(`INSERT INTO partners(id,name,legal_name,org_type,grp,legal_conf,logo_path,website) VALUES(?,?,?,?,?,?,?,?)`,
			p.ID, p.Name, p.LegalName, p.OrgType, p.Group, string(p.LegalConf), p.LogoPath, p.Website); err != nil {
			return err
		}
	}
	for _, p := range f.Products {
		if _, err := tx.Exec(`INSERT INTO products(id,partner_id,name,category) VALUES(?,?,?,?)`, p.ID, p.PartnerID, p.Name, p.Category); err != nil {
			return err
		}
		rj, _ := json.Marshal(p.Rules)
		ij, _ := json.Marshal(p.Info)
		fl := p.Flags
		if fl == nil {
			fl = []string{}
		}
		fj, _ := json.Marshal(fl)
		if _, err := tx.Exec(`INSERT INTO policy_versions(product_id,version,rules_json,info_json,flags_json,source,verified_on,changed_by,change_note) VALUES(?,?,?,?,?,?,?,?,?)`,
			p.ID, 1, string(rj), string(ij), string(fj), p.Source, f.VerifiedOn, "seed", "Khởi tạo từ nghiên cứu Giai đoạn 1"); err != nil {
			return err
		}
		cid := "C" + strings.TrimPrefix(p.ID, "P")
		if _, err := tx.Exec(`INSERT INTO campaigns(id,product_id,name,status) VALUES(?,?,?,?)`, cid, p.ID, p.Name, model.StatusPending); err != nil {
			return err
		}
	}
	for _, sg := range f.Segments {
		if _, err := rules.Parse(string(sg.Rule)); err != nil {
			return fmt.Errorf("segment %s: %w", sg.ID, err)
		}
		if _, err := tx.Exec(`INSERT INTO segments(id,name,rule_json) VALUES(?,?,?)`, sg.ID, sg.Name, string(sg.Rule)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- partners ----------

func (s *Store) Partners() ([]model.Partner, error) {
	rows, err := s.DB.Query(`SELECT id,name,legal_name,org_type,grp,legal_conf,logo_path,website,integration,updated_at FROM partners ORDER BY grp, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Partner
	for rows.Next() {
		var p model.Partner
		var lc string
		if err := rows.Scan(&p.ID, &p.Name, &p.LegalName, &p.OrgType, &p.Group, &lc, &p.LogoPath, &p.Website, &p.Integration, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.LegalConf = model.Confidence(lc)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdatePartner(p model.Partner, actor string) error {
	_, err := s.DB.Exec(`UPDATE partners SET legal_name=?,org_type=?,legal_conf=?,logo_path=?,website=?,integration=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		p.LegalName, p.OrgType, string(p.LegalConf), p.LogoPath, p.Website, p.Integration, p.ID)
	if err == nil {
		s.Audit(actor, "update", "partner", p.ID, p.LegalName)
	}
	return err
}

// ---------- policies (versioned) ----------

func scanPolicy(row interface{ Scan(...any) error }) (model.PolicyVersion, error) {
	var v model.PolicyVersion
	var rj, ij, fj string
	err := row.Scan(&v.ID, &v.ProductID, &v.Version, &rj, &ij, &fj, &v.Source, &v.VerifiedOn, &v.ChangedBy, &v.ChangeNote, &v.CreatedAt)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(rj), &v.Rules); err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(ij), &v.Info); err != nil {
		return v, err
	}
	_ = json.Unmarshal([]byte(fj), &v.Flags)
	return v, nil
}

const policyCols = `id,product_id,version,rules_json,info_json,flags_json,source,verified_on,changed_by,change_note,created_at`

func (s *Store) PolicyHistory(productID string) ([]model.PolicyVersion, error) {
	rows, err := s.DB.Query(`SELECT `+policyCols+` FROM policy_versions WHERE product_id=? ORDER BY version DESC`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.PolicyVersion
	for rows.Next() {
		v, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// AddPolicyVersion appends a new version; old versions are never modified.
func (s *Store) AddPolicyVersion(v model.PolicyVersion) (int, error) {
	if v.Source == "" || v.VerifiedOn == "" {
		return 0, errors.New("cần ghi nguồn và ngày kiểm tra")
	}
	if _, err := time.Parse("2006-01-02", v.VerifiedOn); err != nil {
		return 0, errors.New("ngày kiểm tra phải có dạng YYYY-MM-DD")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var cur int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(version),0) FROM policy_versions WHERE product_id=?`, v.ProductID).Scan(&cur); err != nil {
		return 0, err
	}
	rj, _ := json.Marshal(v.Rules)
	ij, _ := json.Marshal(v.Info)
	if v.Flags == nil {
		v.Flags = []string{}
	}
	fj, _ := json.Marshal(v.Flags)
	if _, err := tx.Exec(`INSERT INTO policy_versions(product_id,version,rules_json,info_json,flags_json,source,verified_on,changed_by,change_note) VALUES(?,?,?,?,?,?,?,?,?)`,
		v.ProductID, cur+1, string(rj), string(ij), string(fj), v.Source, v.VerifiedOn, v.ChangedBy, v.ChangeNote); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`INSERT INTO audit_log(actor,action,entity,entity_id,detail) VALUES(?,?,?,?,?)`,
		v.ChangedBy, "new_version", "policy", v.ProductID, fmt.Sprintf("v%d: %s", cur+1, v.ChangeNote)); err != nil {
		return 0, err
	}
	return cur + 1, tx.Commit()
}

// ---------- offers & campaigns ----------

const campaignCols = `c.id,c.product_id,c.name,c.status,c.priority,c.tracking_url,c.subid_param,c.postback_secret,c.commission_model,c.commission_value,c.daily_lead_cap,c.total_lead_cap,c.target_segments,c.start_at,c.end_at,c.lead_endpoint,c.lead_auth_header,c.updated_at`

func scanCampaign(dst *model.Campaign, extra ...any) []any {
	return append([]any{&dst.ID, &dst.ProductID, &dst.Name, &dst.Status, &dst.Priority, &dst.TrackingURL, &dst.SubIDParam, &dst.PostbackSecret,
		&dst.CommissionModel, &dst.CommissionValue, &dst.DailyLeadCap, &dst.TotalLeadCap, new(string), &dst.StartAt, &dst.EndAt,
		&dst.LeadEndpoint, &dst.LeadAuthHeader, &dst.UpdatedAt}, extra...)
}

// Offers returns every campaign joined with product, partner and current policy.
func (s *Store) Offers() ([]model.Offer, error) {
	q := `SELECT ` + campaignCols + `, c.target_segments,
	  p.id,p.partner_id,p.name,p.category,
	  pa.id,pa.name,pa.legal_name,pa.org_type,pa.grp,pa.legal_conf,pa.logo_path,pa.website,pa.integration,pa.updated_at,
	  pv.` + strings.ReplaceAll(policyCols, ",", ",pv.") + `
	FROM campaigns c
	JOIN products p ON p.id=c.product_id
	JOIN partners pa ON pa.id=p.partner_id
	JOIN policy_versions pv ON pv.product_id=p.id AND pv.version=(SELECT MAX(version) FROM policy_versions WHERE product_id=p.id)
	ORDER BY c.id`
	rows, err := s.DB.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Offer
	for rows.Next() {
		var o model.Offer
		var segs, lc, rj, ij, fj string
		dest := scanCampaign(&o.Campaign, &segs,
			&o.Product.ID, &o.Product.PartnerID, &o.Product.Name, &o.Product.Category,
			&o.Partner.ID, &o.Partner.Name, &o.Partner.LegalName, &o.Partner.OrgType, &o.Partner.Group, &lc, &o.Partner.LogoPath, &o.Partner.Website, &o.Partner.Integration, &o.Partner.UpdatedAt,
			&o.Policy.ID, &o.Policy.ProductID, &o.Policy.Version, &rj, &ij, &fj, &o.Policy.Source, &o.Policy.VerifiedOn, &o.Policy.ChangedBy, &o.Policy.ChangeNote, &o.Policy.CreatedAt)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		o.Partner.LegalConf = model.Confidence(lc)
		_ = json.Unmarshal([]byte(segs), &o.Campaign.TargetSegments)
		if err := json.Unmarshal([]byte(rj), &o.Policy.Rules); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(ij), &o.Policy.Info)
		_ = json.Unmarshal([]byte(fj), &o.Policy.Flags)
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) Offer(campaignID string) (model.Offer, error) {
	all, err := s.Offers()
	if err != nil {
		return model.Offer{}, err
	}
	for _, o := range all {
		if o.Campaign.ID == campaignID {
			return o, nil
		}
	}
	return model.Offer{}, sql.ErrNoRows
}

func (s *Store) Campaign(id string) (model.Campaign, error) {
	var c model.Campaign
	var segs string
	err := s.DB.QueryRow(`SELECT `+campaignCols+`, c.target_segments FROM campaigns c WHERE c.id=?`, id).Scan(scanCampaign(&c, &segs)...)
	_ = json.Unmarshal([]byte(segs), &c.TargetSegments)
	return c, err
}

// SaveCampaign validates and stores admin edits. Effective immediately.
func (s *Store) SaveCampaign(c model.Campaign, actor string) error {
	valid := false
	for _, st := range model.CampaignStatuses {
		if st == c.Status {
			valid = true
		}
	}
	if !valid {
		return errors.New("trạng thái không hợp lệ")
	}
	if c.Priority < 0 || c.Priority > 5 {
		return errors.New("ưu tiên phải từ 0 đến 5")
	}
	if c.TrackingURL != "" && !strings.HasPrefix(c.TrackingURL, "https://") {
		return errors.New("tracking URL phải bắt đầu bằng https://")
	}
	if c.LeadEndpoint != "" && !strings.HasPrefix(c.LeadEndpoint, "https://") {
		return errors.New("lead endpoint phải bắt đầu bằng https://")
	}
	if c.Status == model.StatusActive && c.TrackingURL == "" {
		return errors.New("cần tracking URL trước khi kích hoạt")
	}
	if c.SubIDParam == "" {
		c.SubIDParam = "sub_id"
	}
	if c.TargetSegments == nil {
		c.TargetSegments = []string{}
	}
	segs, _ := json.Marshal(c.TargetSegments)
	_, err := s.DB.Exec(`UPDATE campaigns SET name=?,status=?,priority=?,tracking_url=?,subid_param=?,postback_secret=?,commission_model=?,commission_value=?,
	  daily_lead_cap=?,total_lead_cap=?,target_segments=?,start_at=?,end_at=?,lead_endpoint=?,lead_auth_header=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		c.Name, c.Status, c.Priority, c.TrackingURL, c.SubIDParam, c.PostbackSecret, c.CommissionModel, c.CommissionValue,
		c.DailyLeadCap, c.TotalLeadCap, string(segs), c.StartAt, c.EndAt, c.LeadEndpoint, c.LeadAuthHeader, c.ID)
	if err == nil {
		s.Audit(actor, "update", "campaign", c.ID, fmt.Sprintf("status=%s priority=%d url_set=%t", c.Status, c.Priority, c.TrackingURL != ""))
	}
	return err
}

// CreateCampaign adds a new campaign for an existing product.
func (s *Store) CreateCampaign(id, productID, name, actor string) error {
	if id == "" || productID == "" || name == "" {
		return errors.New("cần mã, sản phẩm và tên chiến dịch")
	}
	_, err := s.DB.Exec(`INSERT INTO campaigns(id,product_id,name,status) VALUES(?,?,?,?)`, id, productID, name, model.StatusDraft)
	if err == nil {
		s.Audit(actor, "create", "campaign", id, productID)
	}
	return err
}

// ---------- segments ----------

func (s *Store) Segments() ([]model.Segment, error) {
	rows, err := s.DB.Query(`SELECT id,name,rule_json,active,version FROM segments ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Segment
	for rows.Next() {
		var g model.Segment
		if err := rows.Scan(&g.ID, &g.Name, &g.Rule, &g.Active, &g.Version); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) SaveSegment(g model.Segment, actor string) error {
	if _, err := rules.Parse(g.Rule); err != nil {
		return fmt.Errorf("luật không hợp lệ: %w", err)
	}
	_, err := s.DB.Exec(`INSERT INTO segments(id,name,rule_json,active,version) VALUES(?,?,?,?,1)
	  ON CONFLICT(id) DO UPDATE SET name=excluded.name, rule_json=excluded.rule_json, active=excluded.active, version=segments.version+1`,
		g.ID, g.Name, g.Rule, g.Active)
	if err == nil {
		s.Audit(actor, "save", "segment", g.ID, g.Rule)
	}
	return err
}

// ---------- analytics ----------

func (s *Store) LogSearch(session string, p model.Profile, segs []string, counts [4]int, top string) {
	pj, _ := json.Marshal(p)
	s.DB.Exec(`INSERT INTO searches(session_id,profile_json,segments,n_eligible,n_potential,n_unverified,n_not,top_campaign) VALUES(?,?,?,?,?,?,?,?)`,
		session, string(pj), strings.Join(segs, ","), counts[0], counts[1], counts[2], counts[3], top)
}

type SegmentStat struct {
	Segment     string
	Searches    int
	ZeroElig    int
	AvgEligible float64
}

func (s *Store) SegmentStats() ([]SegmentStat, error) {
	segs, err := s.Segments()
	if err != nil {
		return nil, err
	}
	var out []SegmentStat
	for _, g := range segs {
		var st SegmentStat
		st.Segment = g.ID + " · " + g.Name
		like := "%" + g.ID + "%"
		if err := s.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN n_eligible=0 THEN 1 ELSE 0 END),0), COALESCE(AVG(n_eligible),0) FROM searches WHERE segments LIKE ?`, like).
			Scan(&st.Searches, &st.ZeroElig, &st.AvgEligible); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

type Counter struct {
	Key   string
	Count int
}

func (s *Store) TopRecommended(limit int) ([]Counter, error) {
	rows, err := s.DB.Query(`SELECT top_campaign, COUNT(*) FROM searches WHERE top_campaign<>'' GROUP BY top_campaign ORDER BY 2 DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Counter
	for rows.Next() {
		var c Counter
		if err := rows.Scan(&c.Key, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) Count(q string, args ...any) int {
	var n int
	s.DB.QueryRow(q, args...).Scan(&n)
	return n
}

func (s *Store) Sum(q string, args ...any) float64 {
	var n float64
	s.DB.QueryRow(q, args...).Scan(&n)
	return n
}

func (s *Store) Audit(actor, action, entity, id, detail string) {
	s.DB.Exec(`INSERT INTO audit_log(actor,action,entity,entity_id,detail) VALUES(?,?,?,?,?)`, actor, action, entity, id, detail)
}

type AuditRow struct {
	Actor, Action, Entity, EntityID, Detail string
	At                                      time.Time
}

func (s *Store) AuditLog(limit int) ([]AuditRow, error) {
	rows, err := s.DB.Query(`SELECT actor,action,entity,entity_id,detail,created_at FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditRow
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.Actor, &a.Action, &a.Entity, &a.EntityID, &a.Detail, &a.At); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
