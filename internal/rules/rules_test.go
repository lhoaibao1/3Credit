package rules

import (
	"testing"

	"github.com/threecredit/platform/internal/model"
)

func TestEval(t *testing.T) {
	p := model.Profile{Job: "freelancer", Income: 12, Channel: "cash", Credit: "bad_settled"}
	cases := []struct {
		rule string
		want bool
	}{
		{`{"field":"job","op":"eq","value":"freelancer"}`, true},
		{`{"field":"job","op":"in","value":["worker","salaried_office"]}`, false},
		{`{"all":[{"field":"income","op":"gte","value":10},{"field":"channel","op":"ne","value":"bank_transfer"}]}`, true},
		{`{"any":[{"field":"credit","op":"eq","value":"good"},{"field":"credit","op":"in","value":["bad_settled"]}]}`, true},
		{`{"not":{"field":"income","op":"lt","value":5}}`, true},
	}
	for _, c := range cases {
		r, err := Parse(c.rule)
		if err != nil {
			t.Fatalf("%s: %v", c.rule, err)
		}
		if got := r.Eval(p); got != c.want {
			t.Errorf("%s = %v, want %v", c.rule, got, c.want)
		}
	}
}

func TestParseRejectsBadRules(t *testing.T) {
	for _, bad := range []string{
		`{"field":"phone","op":"eq","value":"1"}`,
		`{"field":"job","op":"like","value":"x"}`,
		`{"all":[],"field":"job","op":"eq","value":"x"}`,
		`not json`,
	} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("expected error for %s", bad)
		}
	}
}
