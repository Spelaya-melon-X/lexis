package training

import "lexis/internal/platform/httpx"

// NormalizeSelection distinguishes omission (nil) from an explicit empty selection.
// Repeated IDs are harmless: each selected word produces one card.
func NormalizeSelection(ids []int64) ([]int64, error) {
	if ids == nil {
		return nil, nil
	}
	if len(ids) == 0 {
		return nil, httpx.BadRequest("invalid_word_selection", "select at least one word")
	}
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, httpx.BadRequest("invalid_word_selection", "word ids must be positive")
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}
