package engine

// Stats counts how many cluster objects were ensured, patched, and deleted
// during execution. Counts reflect successful operations only.
type Stats struct {
	Ensured int `json:"ensured"`
	Patched int `json:"patched"`
	Deleted int `json:"deleted"`
}

// Add returns the element-wise sum of s and other.
func (s Stats) Add(other Stats) Stats {
	return Stats{
		Ensured: s.Ensured + other.Ensured,
		Patched: s.Patched + other.Patched,
		Deleted: s.Deleted + other.Deleted,
	}
}
