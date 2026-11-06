package server

import (
	"io/fs"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/threecredit/platform/web"
)

// Policy is one published policy document (web/policies/N-slug.md).
// The same files are exported as the documents filed with the Ministry of Industry and Trade.
type Policy struct {
	N           int
	Slug, Title string
	Body        string
}

var policyName = regexp.MustCompile(`^(\d+)-([a-z0-9-]+)\.md$`)

var policies = func() []Policy {
	var out []Policy
	files, _ := fs.ReadDir(web.FS, "policies")
	for _, f := range files {
		m := policyName.FindStringSubmatch(f.Name())
		if m == nil {
			continue
		}
		b, _ := web.FS.ReadFile("policies/" + f.Name())
		title, body, _ := strings.Cut(string(b), "\n")
		n := 0
		for _, c := range m[1] {
			n = n*10 + int(c-'0')
		}
		out = append(out, Policy{N: n, Slug: m[2], Title: strings.TrimPrefix(title, "# "), Body: body})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].N < out[j].N })
	return out
}()

func (s *Server) policyIndex(w http.ResponseWriter, r *http.Request) {
	d := s.base(r, "Quy chế và chính sách – 3Credit")
	d["Policies"] = policies
	s.render(w, "policy.html", d)
}

func (s *Server) policyPage(w http.ResponseWriter, r *http.Request) {
	for _, p := range policies {
		if p.Slug == r.PathValue("slug") {
			d := s.base(r, p.Title+" – 3Credit")
			d["Policy"], d["BodyHTML"], d["Policies"] = p, Markdown(p.Body), policies
			s.render(w, "policy.html", d)
			return
		}
	}
	http.NotFound(w, r)
}
