// Package model holds the domain types shared by all 3Credit modules.
package model

import "time"

// Confidence of a policy value, see docs/research/source-register.md.
//
//	A = read directly on the partner's official website
//	B = official but indirect (banking association, other partner page)
//	C = press / aggregator, reference only
//	"" = not verified
type Confidence string

// Verified reports whether a value may be used as a hard eligibility rule.
func (c Confidence) Verified() bool { return c == "A" || c == "B" }

type Partner struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	LegalName   string     `json:"legal_name"`
	OrgType     string     `json:"org_type"`
	Group       string     `json:"group"`
	LegalConf   Confidence `json:"legal_conf"`
	LogoPath    string     `json:"logo_path"`
	Website     string     `json:"website"`
	Integration string     `json:"integration"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Product struct {
	ID        string `json:"id"`
	PartnerID string `json:"partner_id"`
	Name      string `json:"name"`
	Category  string `json:"category"` // cash_loan | app_loan | credit_card | secured_vehicle | installment
}

// Range is an inclusive numeric range with its source confidence.
type Range struct {
	Min  float64    `json:"min"`
	Max  float64    `json:"max"`
	Conf Confidence `json:"conf"`
	Note string     `json:"note,omitempty"`
}

type IncomeRule struct {
	Min      float64    `json:"min"`                 // triệu VNĐ / tháng
	CivilMin float64    `json:"civil_min,omitempty"` // lower minimum for civil servants
	Conf     Confidence `json:"conf"`
}

// MinRule is a "at least N" condition (months of social insurance, years of life-insurance premiums).
type MinRule struct {
	Min  float64    `json:"min"`
	Conf Confidence `json:"conf"`
}

type ListRule struct {
	Values []string   `json:"values"`
	Mode   string     `json:"mode,omitempty"` // allow | boost
	Why    string     `json:"why,omitempty"`
	Conf   Confidence `json:"conf"`
}

type FlagRule struct {
	Required bool       `json:"required"`
	Conf     Confidence `json:"conf"`
}

// Rules are the eligibility conditions of one policy version.
// A nil field means "not verified / not published".
type Rules struct {
	Amount    *Range      `json:"amount,omitempty"` // VNĐ
	Term      *Range      `json:"term,omitempty"`   // months
	Age       *Range      `json:"age,omitempty"`
	Income    *IncomeRule `json:"income,omitempty"`
	Channel   *ListRule   `json:"channel,omitempty"`
	Jobs      *ListRule   `json:"jobs,omitempty"`
	Regions   *ListRule   `json:"regions,omitempty"`
	Vehicle   *FlagRule   `json:"vehicle,omitempty"`
	Credit    *ListRule   `json:"credit,omitempty"`     // accepted credit_history values
	SocialIns *MinRule    `json:"social_ins,omitempty"` // minimum months of social insurance (BHXH)
	LifeIns   *MinRule    `json:"life_ins,omitempty"`   // minimum years of life-insurance premiums paid (BHNT)
}

// Fact is a displayable policy value with its confidence.
type Fact struct {
	Text string     `json:"text"`
	Conf Confidence `json:"conf"`
}

type Info struct {
	Rate   *Fact `json:"rate,omitempty"`
	Amount *Fact `json:"amount,omitempty"`
	Term   *Fact `json:"term,omitempty"`
	Fee    *Fact `json:"fee,omitempty"`
	Docs   *Fact `json:"docs,omitempty"`
	Speed  *Fact `json:"speed,omitempty"`
}

type PolicyVersion struct {
	ID         int64     `json:"id"`
	ProductID  string    `json:"product_id"`
	Version    int       `json:"version"`
	Rules      Rules     `json:"rules"`
	Info       Info      `json:"info"`
	Flags      []string  `json:"flags"`
	Source     string    `json:"source"`
	VerifiedOn string    `json:"verified_on"`
	ChangedBy  string    `json:"changed_by"`
	ChangeNote string    `json:"change_note"`
	CreatedAt  time.Time `json:"created_at"`
}

const (
	StatusDraft    = "draft"
	StatusPending  = "pending"
	StatusActive   = "active"
	StatusPaused   = "paused"
	StatusExpired  = "expired"
	StatusRejected = "rejected"
)

var CampaignStatuses = []string{StatusDraft, StatusPending, StatusActive, StatusPaused, StatusExpired, StatusRejected}

type Campaign struct {
	ID              string     `json:"id"`
	ProductID       string     `json:"product_id"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	Priority        int        `json:"priority"` // 0..5, tie-break only
	TrackingURL     string     `json:"tracking_url"`
	SubIDParam      string     `json:"subid_param"`
	PostbackSecret  string     `json:"-"`
	CommissionModel string     `json:"commission_model"` // CPL | CPA | CPS | RevShare
	CommissionValue float64    `json:"commission_value"`
	DailyLeadCap    int        `json:"daily_lead_cap"`
	TotalLeadCap    int        `json:"total_lead_cap"`
	TargetSegments  []string   `json:"target_segments"`
	StartAt         *time.Time `json:"start_at"`
	EndAt           *time.Time `json:"end_at"`
	LeadEndpoint    string     `json:"lead_endpoint"`
	LeadAuthHeader  string     `json:"-"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Live reports whether customers may be sent to this campaign right now.
func (c Campaign) Live(now time.Time) bool {
	if c.Status != StatusActive || c.TrackingURL == "" {
		return false
	}
	if c.StartAt != nil && now.Before(*c.StartAt) {
		return false
	}
	if c.EndAt != nil && now.After(*c.EndAt) {
		return false
	}
	return true
}

// Offer is a campaign joined with its product, partner and current policy.
type Offer struct {
	Campaign Campaign
	Product  Product
	Partner  Partner
	Policy   PolicyVersion
}

// Profile is what a customer self-declares. It never contains PII.
type Profile struct {
	Need      string  `json:"need"`   // cash_loan | credit_card | secured | compare
	Amount    float64 `json:"amount"` // VNĐ
	Term      int     `json:"term"`   // months
	Age       int     `json:"age"`
	Region    string  `json:"region"`
	Job       string  `json:"job"`
	Income    float64 `json:"income"`               // triệu / tháng
	Channel   string  `json:"channel"`              // bank_transfer | cash | mixed
	Credit    string  `json:"credit"`               // "" | good | none | past_late_resolved | group2 | bad_settled | bad_unsettled
	Urgency   string  `json:"urgency"`              // flexible | this_week | today
	Vehicle   string  `json:"vehicle"`              // none | motorbike | car
	SocialIns string  `json:"social_ins,omitempty"` // "" | none | lt6 | 6to12 | gt12 (BHXH, months)
	LifeIns   string  `json:"life_ins,omitempty"`   // "" | none | lt1y | ge1y (BHNT premiums paid)
}

type Segment struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Rule    string `json:"rule"` // JSON rule, see internal/rules
	Active  bool   `json:"active"`
	Version int    `json:"version"`
}
