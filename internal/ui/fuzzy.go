package ui

import "strings"

// match is one entry that survived the filter, with the positions that matched
// so the list can highlight them.
type match struct {
	index     int // index into the unfiltered item slice
	score     int
	positions []int
}

// fuzzyMatch scores a subsequence match of pattern within text.
//
// Scoring favours, in order: consecutive runs, matches at a path segment or
// word boundary, and matches near the start. That ordering is what makes
// "ghub" rank "web/github.com" above "misc/graph-hub-notes" — the run of
// consecutive characters after a boundary dominates.
//
// An empty pattern matches everything with score 0, which keeps the unfiltered
// list in its natural sorted order.
func fuzzyMatch(text, pattern string) (match, bool) {
	if pattern == "" {
		return match{}, true
	}

	lowerText := []rune(strings.ToLower(text))
	lowerPat := []rune(strings.ToLower(pattern))

	positions := make([]int, 0, len(lowerPat))
	score := 0
	ti := 0
	prevMatch := -2

	for _, pr := range lowerPat {
		found := -1
		for ; ti < len(lowerText); ti++ {
			if lowerText[ti] == pr {
				found = ti
				break
			}
		}
		if found < 0 {
			return match{}, false
		}

		gain := 1
		if found == prevMatch+1 {
			gain += 8 // consecutive run: the strongest signal of intent
		}
		if found == 0 || isBoundary(lowerText[found-1]) {
			gain += 6 // start of a path segment or word
		}
		if found < 12 {
			gain += (12 - found) / 3 // mild preference for early matches
		}
		score += gain

		positions = append(positions, found)
		prevMatch = found
		ti = found + 1
	}

	// Shorter names that contain the same match are more likely to be the one
	// the user meant, so charge a small penalty for surrounding noise.
	score -= (len([]rune(text)) - len(lowerPat)) / 8

	return match{score: score, positions: positions}, true
}

func isBoundary(r rune) bool {
	switch r {
	case '/', '-', '_', '.', ' ', '@':
		return true
	}
	return false
}

// filterItems ranks names against a query.
//
// Ties are broken by the original index rather than left to sort's discretion,
// so the list never reshuffles between identical queries — a jittering list
// under the cursor is how you delete the wrong entry.
func filterItems(names []string, query string) []match {
	out := make([]match, 0, len(names))
	for i, name := range names {
		m, ok := fuzzyMatch(name, query)
		if !ok {
			continue
		}
		m.index = i
		out = append(out, m)
	}
	if strings.TrimSpace(query) == "" {
		return out
	}
	// Insertion sort: the list is already index-ordered and stores are small
	// enough that the simplicity is worth more than the asymptotics.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			if out[j].score > out[j-1].score {
				out[j], out[j-1] = out[j-1], out[j]
				continue
			}
			break
		}
	}
	return out
}
