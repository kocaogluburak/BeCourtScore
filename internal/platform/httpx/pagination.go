package httpx

import (
	"net/http"
	"strconv"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// PageParams are the pagination query parameters shared by every list endpoint.
type PageParams struct {
	Page     int
	PageSize int
}

// Offset returns the SQL OFFSET for these params.
func (p PageParams) Offset() int { return (p.Page - 1) * p.PageSize }

// ParsePage reads ?page and ?page_size, clamping to sane bounds.
// Missing or invalid values fall back to page=1, page_size=20.
func ParsePage(r *http.Request) PageParams {
	p := PageParams{Page: 1, PageSize: DefaultPageSize}

	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v >= 1 {
		p.Page = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && v >= 1 {
		if v > MaxPageSize {
			v = MaxPageSize
		}
		p.PageSize = v
	}
	return p
}

// Page is the envelope returned by every paginated list endpoint.
type Page[T any] struct {
	Items    []T   `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
	HasMore  bool  `json:"has_more"`
}

// NewPage builds the envelope. items must never be nil so JSON renders [].
func NewPage[T any](items []T, params PageParams, total int64) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{
		Items:    items,
		Page:     params.Page,
		PageSize: params.PageSize,
		Total:    total,
		HasMore:  int64(params.Offset()+len(items)) < total,
	}
}
