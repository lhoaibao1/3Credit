package matching

import (
	"strings"
	"testing"

	"github.com/threecredit/platform/internal/model"
)

func withIns(social, life *model.MinRule) model.Rules {
	r := vibLike
	r.SocialIns, r.LifeIns = social, life
	return r
}

func has(xs []string, sub string) bool {
	for _, x := range xs {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}

func TestInsuranceConditions(t *testing.T) {
	base := Evaluate(offer("X", "cash_loan", vibLike), office(), false)

	// no insurance condition: answers do not change the score
	p := office()
	p.SocialIns, p.LifeIns = "none", "none"
	if r := Evaluate(offer("X", "cash_loan", vibLike), p, false); r.Score != base.Score || r.Status != base.Status {
		t.Fatalf("insurance answers changed an offer without insurance rules: %d/%s vs %d/%s", r.Score, r.Status, base.Score, base.Status)
	}

	// verified BHXH ≥ 12 months
	soc := &model.MinRule{Min: 12, Conf: "B"}
	p = office()
	p.SocialIns = "gt12"
	r := Evaluate(offer("X", "cash_loan", withIns(soc, nil)), p, false)
	if r.Status != Eligible || !has(r.Pass, "BHXH") {
		t.Fatalf("gt12 vs 12 months: %s pass=%v", r.Status, r.Pass)
	}
	p.SocialIns = "lt6"
	r = Evaluate(offer("X", "cash_loan", withIns(soc, nil)), p, false)
	if r.Status != NotElig || !has(r.Fails, "BHXH tối thiểu 12 tháng") {
		t.Fatalf("lt6 vs 12 months must fail: %s fails=%v", r.Status, r.Fails)
	}
	p.SocialIns = ""
	r = Evaluate(offer("X", "cash_loan", withIns(soc, nil)), p, false)
	if r.Status != Potential || !has(r.Gaps, "BHXH") {
		t.Fatalf("undeclared BHXH must need checking: %s gaps=%v", r.Status, r.Gaps)
	}
	// reference-only (C) condition never excludes
	p.SocialIns = "none"
	r = Evaluate(offer("X", "cash_loan", withIns(&model.MinRule{Min: 6, Conf: "C"}, nil)), p, false)
	if r.Status == NotElig {
		t.Fatalf("C-level BHXH condition excluded the offer")
	}

	// BHNT: contract paid ≥ 1 year
	life := &model.MinRule{Min: 1, Conf: "A"}
	p = office()
	p.LifeIns = "ge1y"
	if r = Evaluate(offer("X", "cash_loan", withIns(nil, life)), p, false); r.Status != Eligible || !has(r.Pass, "nhân thọ") {
		t.Fatalf("ge1y: %s %v", r.Status, r.Pass)
	}
	for _, v := range []string{"none", "lt1y"} {
		p.LifeIns = v
		if r = Evaluate(offer("X", "cash_loan", withIns(nil, life)), p, false); r.Status != NotElig || !has(r.Fails, "nhân thọ") {
			t.Fatalf("%s must fail: %s %v", v, r.Status, r.Fails)
		}
	}
	// Min 0 = any contract
	p.LifeIns = "lt1y"
	if r = Evaluate(offer("X", "cash_loan", withIns(nil, &model.MinRule{Conf: "B"})), p, false); r.Status != Eligible {
		t.Fatalf("any contract: %s %v", r.Status, r.Fails)
	}
}
