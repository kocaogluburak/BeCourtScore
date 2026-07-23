package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestParsePage_Defaults(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/matches", nil)
	p := ParsePage(r)
	if p.Page != 1 || p.PageSize != 20 {
		t.Fatalf("got page=%d size=%d, want 1/20", p.Page, p.PageSize)
	}
	if p.Offset() != 0 {
		t.Fatalf("offset: got %d, want 0", p.Offset())
	}
}

func TestParsePage_ClampsAndParses(t *testing.T) {
	cases := []struct {
		url          string
		wantPage     int
		wantPageSize int
	}{
		{"/x?page=3&page_size=50", 3, 50},
		{"/x?page=0&page_size=0", 1, 20},
		{"/x?page=-5&page_size=-1", 1, 20},
		{"/x?page_size=500", 1, 100},
		{"/x?page=abc&page_size=xyz", 1, 20},
	}
	for _, c := range cases {
		p := ParsePage(httptest.NewRequest("GET", c.url, nil))
		if p.Page != c.wantPage || p.PageSize != c.wantPageSize {
			t.Errorf("%s: got page=%d size=%d, want %d/%d", c.url, p.Page, p.PageSize, c.wantPage, c.wantPageSize)
		}
	}
}

func TestNewPage_Envelope(t *testing.T) {
	p := NewPage([]int{1, 2, 3}, PageParams{Page: 1, PageSize: 3}, 10)
	if !p.HasMore {
		t.Error("expected has_more=true")
	}
	if p.Total != 10 {
		t.Errorf("total: got %d, want 10", p.Total)
	}

	last := NewPage([]int{1}, PageParams{Page: 4, PageSize: 3}, 10)
	if last.HasMore {
		t.Error("expected has_more=false on last page")
	}

	empty := NewPage[int](nil, PageParams{Page: 1, PageSize: 20}, 0)
	if empty.Items == nil {
		t.Error("items must not be nil")
	}
}
