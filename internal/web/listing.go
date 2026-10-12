package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// parsePage reads ?page=; anything unusable is page 1 (stores clamp the rest).
func parsePage(r *http.Request) int {
	n, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// Pager carries the previous/next links of a list page.
type Pager struct {
	Page  int
	Pages int
	Prev  string
	Next  string
}

// newPager builds links that keep the current filters.
func newPager(path string, params url.Values, page, pages int) Pager {
	p := Pager{Page: page, Pages: pages}
	if page > 1 {
		p.Prev = pageURL(path, params, page-1)
	}
	if page < pages {
		p.Next = pageURL(path, params, page+1)
	}
	return p
}

func pageURL(path string, params url.Values, page int) string {
	q := url.Values{}
	for k, vs := range params {
		for _, v := range vs {
			if v != "" {
				q.Add(k, v)
			}
		}
	}
	q.Set("page", strconv.Itoa(page))
	return path + "?" + q.Encode()
}

// formValues collects the named fields of a parsed form, trimmed.
func formValues(r *http.Request, fields ...string) map[string]string {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		out[f] = strings.TrimSpace(r.PostFormValue(f))
	}
	return out
}
