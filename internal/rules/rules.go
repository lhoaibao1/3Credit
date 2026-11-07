// Package rules evaluates the JSON rule language used for customer segments.
//
// Grammar:
//
//	{"all": [rule, ...]}           every rule true
//	{"any": [rule, ...]}           at least one true
//	{"not": rule}
//	{"field": "job", "op": "in", "value": ["worker", "freelancer"]}
//
// Operators: eq, ne, in, nin, gt, gte, lt, lte, ratio_gt (value: {"of": "field", "gt": 0.5}).
package rules

import (
	"encoding/json"
	"fmt"

	"github.com/threecredit/platform/internal/model"
)

type Rule struct {
	All   []Rule          `json:"all,omitempty"`
	Any   []Rule          `json:"any,omitempty"`
	Not   *Rule           `json:"not,omitempty"`
	Field string          `json:"field,omitempty"`
	Op    string          `json:"op,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// Parse validates a rule document.
func Parse(s string) (Rule, error) {
	var r Rule
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return r, err
	}
	return r, r.validate()
}

func (r Rule) validate() error {
	n := 0
	if r.All != nil {
		n++
	}
	if r.Any != nil {
		n++
	}
	if r.Not != nil {
		n++
	}
	if r.Field != "" {
		n++
		if _, ok := fieldValue(model.Profile{}, r.Field); !ok {
			return fmt.Errorf("unknown field %q", r.Field)
		}
		switch r.Op {
		case "eq", "ne", "in", "nin", "gt", "gte", "lt", "lte":
		default:
			return fmt.Errorf("unknown op %q", r.Op)
		}
	}
	if n != 1 {
		return fmt.Errorf("rule must have exactly one of all/any/not/field")
	}
	for _, c := range append(append([]Rule{}, r.All...), r.Any...) {
		if err := c.validate(); err != nil {
			return err
		}
	}
	if r.Not != nil {
		return r.Not.validate()
	}
	return nil
}

func fieldValue(p model.Profile, f string) (any, bool) {
	switch f {
	case "need":
		return p.Need, true
	case "amount":
		return p.Amount, true
	case "term":
		return float64(p.Term), true
	case "age":
		return float64(p.Age), true
	case "region":
		return p.Region, true
	case "job":
		return p.Job, true
	case "income":
		return p.Income, true
	case "channel":
		return p.Channel, true
	case "credit":
		return p.Credit, true
	case "urgency":
		return p.Urgency, true
	case "vehicle":
		return p.Vehicle, true
	case "social_ins":
		return p.SocialIns, true
	case "life_ins":
		return p.LifeIns, true
	}
	return nil, false
}

// Eval returns whether the profile satisfies the rule.
func (r Rule) Eval(p model.Profile) bool {
	switch {
	case r.All != nil:
		for _, c := range r.All {
			if !c.Eval(p) {
				return false
			}
		}
		return true
	case r.Any != nil:
		for _, c := range r.Any {
			if c.Eval(p) {
				return true
			}
		}
		return false
	case r.Not != nil:
		return !r.Not.Eval(p)
	}
	v, ok := fieldValue(p, r.Field)
	if !ok {
		return false
	}
	switch x := v.(type) {
	case string:
		switch r.Op {
		case "eq", "ne":
			var want string
			if json.Unmarshal(r.Value, &want) != nil {
				return false
			}
			return (x == want) == (r.Op == "eq")
		case "in", "nin":
			var set []string
			if json.Unmarshal(r.Value, &set) != nil {
				return false
			}
			in := false
			for _, s := range set {
				if s == x {
					in = true
					break
				}
			}
			return in == (r.Op == "in")
		}
	case float64:
		var want float64
		if json.Unmarshal(r.Value, &want) != nil {
			return false
		}
		switch r.Op {
		case "eq":
			return x == want
		case "ne":
			return x != want
		case "gt":
			return x > want
		case "gte":
			return x >= want
		case "lt":
			return x < want
		case "lte":
			return x <= want
		}
	}
	return false
}

// Segments returns the IDs of active segments the profile belongs to.
func Segments(segs []model.Segment, p model.Profile) []model.Segment {
	var out []model.Segment
	for _, s := range segs {
		if !s.Active {
			continue
		}
		r, err := Parse(s.Rule)
		if err != nil {
			continue
		}
		if r.Eval(p) {
			out = append(out, s)
		}
	}
	return out
}
