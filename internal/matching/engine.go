// Package matching implements the product matching engine:
// hard eligibility rules → scoring → ranking, with an explanation per result.
//
// Only verified policy values (confidence A or B) can exclude a product.
// Reference values (C) can only add a "check this" note and lower the score.
// Commission is never part of the score; campaign priority (0..5) is a
// bounded tie-breaker that an admin sets openly.
package matching

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/threecredit/platform/internal/model"
)

type Status string

const (
	Eligible   Status = "eligible"   // meets every verified condition we know
	Potential  Status = "potential"  // no verified failure, but something must be checked
	Unverified Status = "unverified" // no verified condition to compare against
	NotElig    Status = "not"        // fails a verified condition
)

var statusOrder = map[Status]int{Eligible: 0, Potential: 1, Unverified: 2, NotElig: 3}

func (s Status) Label() string {
	switch s {
	case Eligible:
		return "Đáp ứng điều kiện đã biết"
	case Potential:
		return "Có thể phù hợp, cần kiểm tra"
	case Unverified:
		return "Chính sách chưa xác minh"
	}
	return "Không phù hợp"
}

type Result struct {
	Offer      model.Offer `json:"-"`
	CampaignID string      `json:"campaign_id"`
	Status     Status      `json:"status"`
	Pass       []string    `json:"pass"`      // verified conditions met
	Reference  []string    `json:"reference"` // reference-only conditions met
	Gaps       []string    `json:"gaps"`      // to check
	Fails      []string    `json:"fails"`     // verified conditions failed
	Fit        int         `json:"fit"`       // 0..100
	Confidence float64     `json:"confidence"`
	Score      int         `json:"score"`
	Live       bool        `json:"live"`
}

// Categories shown for each customer need.
var needCategories = map[string][]string{
	"cash_loan":   {"cash_loan", "app_loan"},
	"credit_card": {"credit_card"},
	"secured":     {"secured_vehicle"},
	"compare":     {"cash_loan", "app_loan", "credit_card", "secured_vehicle", "installment"},
}

func CategoriesFor(need string) []string {
	if c, ok := needCategories[need]; ok {
		return c
	}
	return needCategories["compare"]
}

var creditText = map[string]string{
	"group2":        "nợ nhóm 2",
	"bad_settled":   "nợ xấu đã tất toán",
	"bad_unsettled": "nợ xấu chưa tất toán",
}

// AdverseCredit reports whether the declared history is group-2 debt or bad debt.
func AdverseCredit(c string) bool { _, ok := creditText[c]; return ok }

// AcceptsCredit reports whether the admin set this product, in the CMS policy, as accepting the
// given credit history (the box is ticked and a source is chosen). Nothing set means "not accepted".
func AcceptsCredit(o model.Offer, credit string) bool {
	r := o.Policy.Rules.Credit
	return r != nil && contains(r.Values, credit)
}

func money(v float64) string {
	if v >= 1e9 {
		return trimNum(v/1e9) + " tỷ"
	}
	return trimNum(v/1e6) + " triệu"
}

func trimNum(f float64) string {
	s := fmt.Sprintf("%.1f", f)
	s = strings.TrimSuffix(s, ".0")
	return strings.Replace(s, ".", ",", 1)
}

type eval struct {
	pass, ref, gaps, fails []string
	soft                   int
}

func (e *eval) check(ok bool, conf model.Confidence, okTxt, badTxt string) {
	switch {
	case ok && conf.Verified():
		e.pass = append(e.pass, okTxt)
	case ok:
		e.ref = append(e.ref, okTxt+" (nguồn tham khảo)")
	case conf.Verified():
		e.fails = append(e.fails, badTxt)
	default:
		e.gaps = append(e.gaps, badTxt+" (nguồn tham khảo, cần kiểm tra)")
		e.soft++
	}
}

// insurance answers are bands; a band passes when its lower end meets the minimum,
// fails when its upper end is below it, and is unknown otherwise (or when not declared).
type span struct{ lo, hi float64 }

var (
	socialSpan = map[string]span{"none": {0, 0}, "lt6": {0.001, 5.999}, "6to12": {6, 12}, "gt12": {12.001, math.Inf(1)}}
	lifeSpan   = map[string]span{"none": {-1, -1}, "lt1y": {0, 0.999}, "ge1y": {1, math.Inf(1)}}
)

// insCheck returns 1 = meets, -1 = does not meet, 0 = unknown.
func insCheck(s span, min float64) int {
	switch {
	case s == span{}:
		return 0
	case s.lo >= min:
		return 1
	case s.hi < min:
		return -1
	}
	return 0
}

// tri is check() with an "unknown" outcome that only asks the customer to verify.
func (e *eval) tri(v int, conf model.Confidence, okTxt, badTxt, unkTxt string) {
	if v == 0 {
		e.gaps = append(e.gaps, unkTxt)
		if conf.Verified() {
			e.soft++ // a published condition we cannot confirm keeps the offer at "cần kiểm tra"
		}
		return
	}
	e.check(v > 0, conf, okTxt, badTxt)
}

func rangeText(r *model.Range, unit func(float64) string) string {
	if r.Min > 0 {
		return unit(r.Min) + "–" + unit(r.Max)
	}
	return "tối đa " + unit(r.Max)
}

// Evaluate scores one offer against a profile.
func Evaluate(o model.Offer, p model.Profile, live bool) Result {
	R := o.Policy.Rules
	e := &eval{}
	months := func(v float64) string { return trimNum(v) + " tháng" }

	if R.Amount != nil {
		e.check(p.Amount >= R.Amount.Min && p.Amount <= R.Amount.Max, R.Amount.Conf,
			"Số tiền "+money(p.Amount)+" trong hạn mức",
			"Số tiền "+money(p.Amount)+" ngoài hạn mức "+rangeText(R.Amount, money))
	} else {
		e.gaps = append(e.gaps, "Hạn mức chưa xác minh")
	}
	if o.Product.Category != "credit_card" {
		if R.Term != nil {
			t := float64(p.Term)
			e.check(t >= R.Term.Min && t <= R.Term.Max, R.Term.Conf,
				fmt.Sprintf("Kỳ hạn %d tháng trong khung", p.Term),
				fmt.Sprintf("Kỳ hạn %d tháng ngoài khung %s", p.Term, rangeText(R.Term, months)))
		} else {
			e.gaps = append(e.gaps, "Kỳ hạn chưa xác minh")
		}
	}
	if R.Age != nil {
		note := ""
		if R.Age.Note != "" {
			note = " (" + R.Age.Note + ")"
		}
		a := float64(p.Age)
		e.check(a >= R.Age.Min && a <= R.Age.Max, R.Age.Conf,
			fmt.Sprintf("Tuổi %d trong khung %s–%s%s", p.Age, trimNum(R.Age.Min), trimNum(R.Age.Max), note),
			fmt.Sprintf("Tuổi %d ngoài khung %s–%s", p.Age, trimNum(R.Age.Min), trimNum(R.Age.Max)))
	} else {
		e.gaps = append(e.gaps, "Điều kiện độ tuổi chưa xác minh")
	}
	if R.Income != nil {
		need := R.Income.Min
		if p.Job == "civil_servant" && R.Income.CivilMin > 0 {
			need = R.Income.CivilMin
		}
		e.check(p.Income >= need, R.Income.Conf,
			"Thu nhập đạt mức tối thiểu "+trimNum(need)+" triệu/tháng",
			"Thu nhập dưới mức tối thiểu "+trimNum(need)+" triệu/tháng")
	} else {
		e.gaps = append(e.gaps, "Thu nhập tối thiểu chưa xác minh")
	}
	if R.Channel != nil {
		e.check(contains(R.Channel.Values, p.Channel), R.Channel.Conf,
			"Cách nhận lương đúng yêu cầu", "Yêu cầu nhận lương qua chuyển khoản")
	}
	if R.Regions != nil {
		e.check(contains(R.Regions.Values, p.Region), R.Regions.Conf,
			"Khu vực của bạn được hỗ trợ", "Khu vực của bạn chưa được hỗ trợ")
	}
	if R.Jobs != nil {
		in := contains(R.Jobs.Values, p.Job)
		if R.Jobs.Mode == "allow" {
			e.check(in, R.Jobs.Conf, R.Jobs.Why, "Không dành cho nhóm nghề của bạn ("+strings.ToLower(R.Jobs.Why)+")")
		} else if in {
			if R.Jobs.Conf.Verified() {
				e.pass = append(e.pass, R.Jobs.Why)
			} else {
				e.ref = append(e.ref, R.Jobs.Why)
			}
		}
	}
	if R.Vehicle != nil && R.Vehicle.Required {
		e.check(p.Vehicle != "" && p.Vehicle != "none", R.Vehicle.Conf,
			"Bạn có giấy đăng ký xe làm tài sản", "Cần giấy đăng ký xe máy hoặc ô tô")
	}
	if R.SocialIns != nil {
		m := trimNum(R.SocialIns.Min)
		e.tri(insCheck(socialSpan[p.SocialIns], R.SocialIns.Min), R.SocialIns.Conf,
			"Thời gian đóng BHXH đạt yêu cầu tối thiểu "+m+" tháng",
			"Cần đóng BHXH tối thiểu "+m+" tháng",
			"Cần kiểm tra thời gian đóng BHXH (yêu cầu tối thiểu "+m+" tháng)")
	}
	if R.LifeIns != nil {
		want := "Cần có hợp đồng bảo hiểm nhân thọ"
		if R.LifeIns.Min > 0 {
			want += " đã đóng phí tối thiểu " + trimNum(R.LifeIns.Min) + " năm"
		}
		e.tri(insCheck(lifeSpan[p.LifeIns], R.LifeIns.Min), R.LifeIns.Conf,
			"Hợp đồng bảo hiểm nhân thọ đạt yêu cầu", want, "Cần kiểm tra hợp đồng bảo hiểm nhân thọ ("+strings.ToLower(want[:1])+want[1:]+")")
	}
	adverse := AdverseCredit(p.Credit)
	if R.Credit != nil && R.Credit.Conf.Verified() {
		if p.Credit != "" {
			e.check(contains(R.Credit.Values, p.Credit), R.Credit.Conf,
				"Chính sách nhận lịch sử tín dụng của bạn", "Chính sách không nhận lịch sử tín dụng của bạn")
		}
	} else if adverse {
		e.gaps = append(e.gaps, "Chưa xác minh đối tác có nhận hồ sơ "+creditText[p.Credit]+" hay không")
	} else {
		e.gaps = append(e.gaps, "Điều kiện lịch sử tín dụng (CIC) chưa được công bố")
	}
	e.gaps = append(e.gaps, o.Policy.Flags...)

	var st Status
	switch {
	case len(e.fails) > 0:
		st = NotElig
	case len(e.pass) == 0:
		st = Unverified
	case adverse && !(R.Credit != nil && R.Credit.Conf.Verified()):
		st = Potential
	case e.soft > 0 || len(o.Policy.Flags) > 0:
		st = Potential
	case len(e.pass) >= 2:
		st = Eligible
	default:
		st = Potential
	}

	known := 0
	for _, v := range []bool{
		R.Amount != nil && R.Amount.Conf.Verified(), R.Term != nil && R.Term.Conf.Verified(),
		R.Age != nil && R.Age.Conf.Verified(), R.Income != nil && R.Income.Conf.Verified(),
		R.Channel != nil && R.Channel.Conf.Verified(), R.Jobs != nil && R.Jobs.Conf.Verified(),
		R.Vehicle != nil && R.Vehicle.Conf.Verified(),
		R.SocialIns != nil && R.SocialIns.Conf.Verified(), R.LifeIns != nil && R.LifeIns.Conf.Verified(),
	} {
		if v {
			known++
		}
	}
	conf := 0.3 + float64(known)/5*0.7
	if conf > 1 {
		conf = 1
	}
	fit := 45 + len(e.pass)*12 + len(e.ref)*4 - e.soft*12 - len(e.fails)*30
	if p.Urgency == "today" && o.Product.Category == "app_loan" {
		fit += 6
	}
	if p.Need == "secured" && o.Product.Category == "secured_vehicle" {
		fit += 10
	}
	fit = clamp(fit, 0, 100)
	prio := clamp(o.Campaign.Priority, 0, 5)

	return Result{
		Offer: o, CampaignID: o.Campaign.ID, Status: st,
		Pass: e.pass, Reference: e.ref, Gaps: e.gaps, Fails: e.fails,
		Fit: fit, Confidence: conf, Score: int(float64(fit)*conf+0.5) + prio, Live: live,
	}
}

// Match evaluates and ranks offers for a profile.
func Match(offers []model.Offer, p model.Profile, live func(model.Campaign) bool) []Result {
	cats := CategoriesFor(p.Need)
	out := make([]Result, 0, len(offers))
	for _, o := range offers {
		if !contains(cats, o.Product.Category) {
			continue
		}
		// A customer with group-2 or bad debt is only offered products the CMS marks as accepting that debt type.
		if AdverseCredit(p.Credit) && !AcceptsCredit(o, p.Credit) {
			continue
		}
		out = append(out, Evaluate(o, p, live(o.Campaign)))
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if statusOrder[a.Status] != statusOrder[b.Status] {
			return statusOrder[a.Status] < statusOrder[b.Status]
		}
		return a.Score > b.Score
	})
	return out
}

// Counts summarises results by status.
func Counts(rs []Result) map[Status]int {
	m := map[Status]int{Eligible: 0, Potential: 0, Unverified: 0, NotElig: 0}
	for _, r := range rs {
		m[r.Status]++
	}
	return m
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
