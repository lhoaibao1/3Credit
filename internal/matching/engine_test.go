package matching

import (
	"testing"

	"github.com/threecredit/platform/internal/model"
)

func offer(id, cat string, r model.Rules, flags ...string) model.Offer {
	return model.Offer{
		Campaign: model.Campaign{ID: id, Status: model.StatusPending},
		Product:  model.Product{ID: "P" + id, Category: cat},
		Partner:  model.Partner{ID: "p" + id, Name: id},
		Policy:   model.PolicyVersion{Rules: r, Flags: flags},
	}
}

func office() model.Profile {
	return model.Profile{Need: "cash_loan", Amount: 50e6, Term: 24, Age: 28, Job: "salaried_office", Income: 12, Channel: "bank_transfer", Credit: "good", Urgency: "flexible", Vehicle: "none"}
}

var vibLike = model.Rules{
	Age:     &model.Range{Min: 22, Max: 60, Conf: "B"},
	Income:  &model.IncomeRule{Min: 7, CivilMin: 5, Conf: "B"},
	Channel: &model.ListRule{Values: []string{"bank_transfer"}, Conf: "B"},
	Amount:  &model.Range{Min: 0, Max: 1e9, Conf: "B"},
	Term:    &model.Range{Min: 12, Max: 60, Conf: "B"},
}

func never(model.Campaign) bool { return false }

func TestVerifiedConditionsMakeEligible(t *testing.T) {
	r := Evaluate(offer("VIB", "cash_loan", vibLike), office(), false)
	if r.Status != Eligible {
		t.Fatalf("status = %s, want eligible; fails=%v gaps=%v", r.Status, r.Fails, r.Gaps)
	}
	if len(r.Pass) != 5 {
		t.Errorf("pass = %d, want 5: %v", len(r.Pass), r.Pass)
	}
}

func TestVerifiedFailureExcludes(t *testing.T) {
	p := office()
	p.Channel = "cash"
	r := Evaluate(offer("VIB", "cash_loan", vibLike), p, false)
	if r.Status != NotElig {
		t.Fatalf("status = %s, want not", r.Status)
	}
}

func TestReferenceFailureNeverExcludes(t *testing.T) {
	rules := model.Rules{
		Amount: &model.Range{Max: 100e6, Conf: "A"},
		Age:    &model.Range{Min: 20, Max: 60, Conf: "C"},
	}
	p := office()
	p.Age = 65
	r := Evaluate(offer("FE", "cash_loan", rules), p, false)
	if r.Status == NotElig {
		t.Fatal("a reference-only (C) value must not exclude a product")
	}
	if r.Status != Potential {
		t.Fatalf("status = %s, want potential", r.Status)
	}
}

func TestAdverseCreditNeverEligibleWithoutVerifiedPolicy(t *testing.T) {
	p := office()
	for _, c := range []string{"group2", "bad_settled", "bad_unsettled"} {
		p.Credit = c
		r := Evaluate(offer("VIB", "cash_loan", vibLike), p, false)
		if r.Status == Eligible {
			t.Fatalf("credit=%s: must not be eligible without a verified credit policy", c)
		}
	}
}

func TestVerifiedCreditPolicyDecides(t *testing.T) {
	rules := vibLike
	rules.Credit = &model.ListRule{Values: []string{"good", "none"}, Conf: "A"}
	p := office()
	p.Credit = "bad_unsettled"
	if r := Evaluate(offer("X", "cash_loan", rules), p, false); r.Status != NotElig {
		t.Fatalf("status = %s, want not", r.Status)
	}
	p.Credit = "none"
	if r := Evaluate(offer("X", "cash_loan", rules), p, false); r.Status != Eligible {
		t.Fatalf("status = %s, want eligible", r.Status)
	}
}

func TestNoVerifiedDataIsUnverified(t *testing.T) {
	r := Evaluate(offer("S", "cash_loan", model.Rules{}), office(), false)
	if r.Status != Unverified {
		t.Fatalf("status = %s, want unverified", r.Status)
	}
}

func TestFlagsPreventEligible(t *testing.T) {
	r := Evaluate(offer("VIB", "cash_loan", vibLike, "Tổng chi phí chưa xác minh"), office(), false)
	if r.Status != Potential {
		t.Fatalf("status = %s, want potential when the policy carries a warning", r.Status)
	}
}

func TestRankingOrderAndPriorityBound(t *testing.T) {
	unver := offer("U", "cash_loan", model.Rules{})
	unver.Campaign.Priority = 99 // must be clamped and never lift above eligible
	elig := offer("E", "cash_loan", vibLike)
	rs := Match([]model.Offer{unver, elig}, office(), never)
	if rs[0].CampaignID != "E" {
		t.Fatalf("eligible must rank first, got %s", rs[0].CampaignID)
	}
	if rs[1].Score > 100+5 {
		t.Fatalf("priority boost not bounded: %d", rs[1].Score)
	}
}

func TestCategoryFilter(t *testing.T) {
	car := offer("F88", "secured_vehicle", model.Rules{Vehicle: &model.FlagRule{Required: true, Conf: "A"}})
	cash := offer("VIB", "cash_loan", vibLike)
	p := office()
	p.Need = "secured"
	rs := Match([]model.Offer{car, cash}, p, never)
	if len(rs) != 1 || rs[0].CampaignID != "F88" {
		t.Fatalf("secured need should only show secured products, got %d", len(rs))
	}
	if rs[0].Status != NotElig {
		t.Fatalf("no vehicle declared: want not eligible, got %s", rs[0].Status)
	}
}
