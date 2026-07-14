package pagination

const (
	DefaultPage    = 1
	DefaultPerPage = 20
	MaxPerPage     = 100
)

func Normalize(page, perPage int) (int, int) {
	if page < 1 {
		page = DefaultPage
	}
	if perPage < 1 || perPage > MaxPerPage {
		perPage = DefaultPerPage
	}
	return page, perPage
}
