package pagination

// Cursor holds the decoded state for cursor-based pagination.
type Cursor struct {
	AfterID string
	Limit   int
}

const defaultLimit = 20

func New(afterID string, limit int) Cursor {
	if limit <= 0 || limit > 100 {
		limit = defaultLimit
	}
	return Cursor{AfterID: afterID, Limit: limit}
}
