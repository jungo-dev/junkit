package pagination_test

import (
	"testing"

	"github.com/jungo-dev/junkit/pagination"
)

func intPtr(v int) *int { return &v }

func TestNewPagination(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		pageSize   int
		totalItems int64
		want       pagination.Pagination
	}{
		{
			name:       "middle page has both next and prev",
			page:       2,
			pageSize:   10,
			totalItems: 35,
			want: pagination.Pagination{
				Page: 2, PageSize: 10, TotalItems: 35, TotalPages: 4,
				HasNext: true, HasPrev: true,
				NextPage: intPtr(3), PrevPage: intPtr(1),
			},
		},
		{
			name:       "first page has no prev",
			page:       1,
			pageSize:   10,
			totalItems: 35,
			want: pagination.Pagination{
				Page: 1, PageSize: 10, TotalItems: 35, TotalPages: 4,
				HasNext: true, HasPrev: false,
				NextPage: intPtr(2), PrevPage: nil,
			},
		},
		{
			name:       "last page has no next",
			page:       4,
			pageSize:   10,
			totalItems: 35,
			want: pagination.Pagination{
				Page: 4, PageSize: 10, TotalItems: 35, TotalPages: 4,
				HasNext: false, HasPrev: true,
				NextPage: nil, PrevPage: intPtr(3),
			},
		},
		{
			name:       "no items still reports one page",
			page:       1,
			pageSize:   10,
			totalItems: 0,
			want: pagination.Pagination{
				Page: 1, PageSize: 10, TotalItems: 0, TotalPages: 1,
				HasNext: false, HasPrev: false,
			},
		},
		{
			name:       "zero page is clamped to 1",
			page:       0,
			pageSize:   10,
			totalItems: 35,
			want: pagination.Pagination{
				Page: 1, PageSize: 10, TotalItems: 35, TotalPages: 4,
				HasNext: true, HasPrev: false,
				NextPage: intPtr(2), PrevPage: nil,
			},
		},
		{
			name:       "negative page size is clamped to 1",
			page:       1,
			pageSize:   -5,
			totalItems: 3,
			want: pagination.Pagination{
				Page: 1, PageSize: 1, TotalItems: 3, TotalPages: 3,
				HasNext: true, HasPrev: false,
				NextPage: intPtr(2), PrevPage: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pagination.NewPagination(tt.page, tt.pageSize, tt.totalItems)

			if got.Page != tt.want.Page ||
				got.PageSize != tt.want.PageSize ||
				got.TotalItems != tt.want.TotalItems ||
				got.TotalPages != tt.want.TotalPages ||
				got.HasNext != tt.want.HasNext ||
				got.HasPrev != tt.want.HasPrev {
				t.Fatalf("NewPagination(%d, %d, %d) = %+v, want %+v",
					tt.page, tt.pageSize, tt.totalItems, got, tt.want)
			}

			assertIntPtrEqual(t, "NextPage", got.NextPage, tt.want.NextPage)
			assertIntPtrEqual(t, "PrevPage", got.PrevPage, tt.want.PrevPage)
		})
	}
}

func assertIntPtrEqual(t *testing.T, field string, got, want *int) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		return
	case got == nil || want == nil:
		t.Fatalf("%s = %v, want %v", field, derefOrNil(got), derefOrNil(want))
	case *got != *want:
		t.Fatalf("%s = %d, want %d", field, *got, *want)
	}
}

func derefOrNil(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestRequest_SetDefaults(t *testing.T) {
	tests := []struct {
		name             string
		req              pagination.Request
		defaultSortBy    string
		defaultSortOrder string
		wantPage         int
		wantPageSize     int
		wantSortBy       string
		wantSortOrder    string
	}{
		{
			name:             "empty request gets every default",
			req:              pagination.Request{},
			defaultSortBy:    "created_at",
			defaultSortOrder: "desc",
			wantPage:         1,
			wantPageSize:     10,
			wantSortBy:       "created_at",
			wantSortOrder:    "desc",
		},
		{
			name:             "already-set fields are left untouched",
			req:              pagination.Request{Page: 3, PageSize: 25, SortBy: "name", SortOrder: "asc"},
			defaultSortBy:    "created_at",
			defaultSortOrder: "desc",
			wantPage:         3,
			wantPageSize:     25,
			wantSortBy:       "name",
			wantSortOrder:    "asc",
		},
		{
			name:             "negative page and page_size fall back to defaults too",
			req:              pagination.Request{Page: -1, PageSize: -1},
			defaultSortBy:    "created_at",
			defaultSortOrder: "desc",
			wantPage:         1,
			wantPageSize:     10,
			wantSortBy:       "created_at",
			wantSortOrder:    "desc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req
			req.SetDefaults(tt.defaultSortBy, tt.defaultSortOrder)

			if req.Page != tt.wantPage || req.PageSize != tt.wantPageSize ||
				req.SortBy != tt.wantSortBy || req.SortOrder != tt.wantSortOrder {
				t.Fatalf("SetDefaults() = %+v, want page=%d page_size=%d sort_by=%q sort_order=%q",
					req, tt.wantPage, tt.wantPageSize, tt.wantSortBy, tt.wantSortOrder)
			}
		})
	}
}

func TestRequest_GetOffset(t *testing.T) {
	tests := []struct {
		name string
		req  pagination.Request
		want int32
	}{
		{name: "first page starts at offset 0", req: pagination.Request{Page: 1, PageSize: 10}, want: 0},
		{name: "second page offsets by one page size", req: pagination.Request{Page: 2, PageSize: 10}, want: 10},
		{name: "third page with a smaller page size", req: pagination.Request{Page: 3, PageSize: 5}, want: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.GetOffset(); got != tt.want {
				t.Fatalf("GetOffset() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRequest_GetLimit(t *testing.T) {
	req := pagination.Request{PageSize: 25}

	if got, want := req.GetLimit(), int32(25); got != want {
		t.Fatalf("GetLimit() = %d, want %d", got, want)
	}
}
