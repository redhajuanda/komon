package pagination

type Pagination struct {
	Type           string `query:"pagination_type" validate:"required,oneof=offset cursor" form:"pagination_type"`
	Page           int    `query:"page" form:"page"`
	PerPage        int    `query:"per_page" form:"per_page"`
	CountTotalData bool   `query:"count_total_data" form:"count_total_data"`
	Cursor         string `query:"cursor" form:"cursor"`
	Result         *Result
}

type Result struct {
	Type   string                  `json:"type"`
	Offset *ResultPaginationOffset `json:"offset,omitempty"`
	Cursor *ResultPaginationCursor `json:"cursor,omitempty"`
}

type ResultPaginationOffset struct {
	TotalData   *int `json:"total_data,omitempty"`
	TotalPage   *int `json:"total_page,omitempty"`
	CurrentPage int  `json:"current_page"`
	PerPage     int  `json:"per_page"`
}

// HasNext checks if there is a next page.
func (r *ResultPaginationOffset) HasNext() bool {
	if r.TotalPage == nil {
		return false
	}
	return r.CurrentPage < *r.TotalPage
}

// NextPage returns the next page number.
func (r *ResultPaginationOffset) NextPage() int {
	return r.CurrentPage + 1
}

type ResultPaginationCursor struct {
	TotalData *int   `json:"total_data,omitempty"`
	Next      string `json:"next"`
	Prev      string `json:"prev"`
}

// GetOffset calculates the offset based on the current page and items per page.
func (p *Pagination) GetOffset() int {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PerPage <= 0 {
		p.PerPage = 10
	}
	return (p.Page - 1) * p.PerPage
}

// BuildResponseOffset builds the pagination response for offset-based pagination.
func (p *Pagination) BuildResponseOffset(totalData *int) {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PerPage <= 0 {
		p.PerPage = 10
	}

	if totalData == nil {
		p.Result = &Result{
			Type: "offset",
			Offset: &ResultPaginationOffset{
				CurrentPage: p.Page,
				PerPage:     p.PerPage,
			},
		}
		return
	}

	totalPage := (*totalData + p.PerPage - 1) / p.PerPage // Calculate total pages

	p.Result = &Result{
		Type: "offset",
		Offset: &ResultPaginationOffset{
			TotalData:   totalData,
			TotalPage:   &totalPage,
			CurrentPage: p.Page,
			PerPage:     p.PerPage,
		},
	}
}

// BuildResponseCursor builds the pagination response for cursor-based pagination.
func (p *Pagination) BuildResponseCursor(next, prev string, totalData *int) {
	p.Result = &Result{
		Type: "cursor",
		Cursor: &ResultPaginationCursor{
			TotalData: totalData,
			Next:      next,
			Prev:      prev,
		},
	}
}
