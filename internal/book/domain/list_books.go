package domain

type ListBooksInput struct {
	Search   string
	Category string
	Page     int
	PerPage  int
}
