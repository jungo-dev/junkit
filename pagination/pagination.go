// Package pagination provides utilities for parsing request pagination parameters and calculating SQL offset/limit.
package pagination

import "math"

// Pagination holds navigation metadata returned alongside a list response.
type Pagination struct {
	Page       int   `json:"page" example:"1"`
	PageSize   int   `json:"page_size" example:"10"`
	TotalItems int64 `json:"total_items" example:"100"`
	TotalPages int   `json:"total_pages" example:"10"`
	HasNext    bool  `json:"has_next" example:"true"`
	HasPrev    bool  `json:"has_prev" example:"false"`
	NextPage   *int  `json:"next_page"`
	PrevPage   *int  `json:"prev_page"`
}

// PaginatedResponse combines a page of items with their Pagination metadata.
type PaginatedResponse[T any] struct {
	Items      []T        `json:"items"`
	Pagination Pagination `json:"pagination"`
}

// Request represents standard list query parameters.
type Request struct {
	Page      int    `form:"page" binding:"omitempty,min=1" example:"1"`
	PageSize  int    `form:"page_size" binding:"omitempty,min=1,max=100" example:"10"`
	Search    string `form:"search" binding:"omitempty" example:"John"`
	SortBy    string `form:"sort_by" binding:"omitempty" example:"created_at"`
	SortOrder string `form:"sort_order" binding:"omitempty,oneof=asc desc" example:"desc"`
}

// NewPagination computes Pagination metadata for a page of results.
//
// Usage:
//
//	meta := pagination.NewPagination(req.Page, req.PageSize, totalItems)
func NewPagination(page, pageSize int, totalItems int64) Pagination {
	if pageSize <= 0 {
		pageSize = 1
	}
	if page <= 0 {
		page = 1
	}

	totalPages := int(math.Ceil(float64(totalItems) / float64(pageSize)))
	if totalPages == 0 {
		totalPages = 1
	}

	hasNext := page < totalPages
	hasPrev := page > 1

	var nextPage, prevPage *int
	if hasNext {
		next := page + 1
		nextPage = &next
	}
	if hasPrev {
		prev := page - 1
		prevPage = &prev
	}

	return Pagination{
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
		HasNext:    hasNext,
		HasPrev:    hasPrev,
		NextPage:   nextPage,
		PrevPage:   prevPage,
	}
}

// SetDefaults fills default values for missing request pagination fields.
//
// Usage:
//
//	var req pagination.Request
//	_ = ctx.ShouldBindQuery(&req)
//	req.SetDefaults("created_at", "desc")
func (r *Request) SetDefaults(defaultSortBy, defaultSortOrder string) {
	if r.Page <= 0 {
		r.Page = 1
	}
	if r.PageSize <= 0 {
		r.PageSize = 10
	}
	if r.SortBy == "" {
		r.SortBy = defaultSortBy
	}
	if r.SortOrder == "" {
		r.SortOrder = defaultSortOrder
	}
}

// GetOffset returns the SQL OFFSET for the current page.
func (r *Request) GetOffset() int32 {
	return int32((r.Page - 1) * r.PageSize)
}

// GetLimit returns the SQL LIMIT for the current page.
func (r *Request) GetLimit() int32 {
	return int32(r.PageSize)
}
