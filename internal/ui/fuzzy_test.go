package ui

import "testing"

func TestFuzzyMatchBasics(t *testing.T) {
	if _, ok := fuzzyMatch("web/github.com", "ghub"); !ok {
		t.Error("subsequence match should succeed")
	}
	if _, ok := fuzzyMatch("web/github.com", "zzz"); ok {
		t.Error("non-subsequence match should fail")
	}
	if _, ok := fuzzyMatch("anything", ""); !ok {
		t.Error("an empty pattern should match everything")
	}
}

func TestFuzzyMatchIsCaseInsensitive(t *testing.T) {
	if _, ok := fuzzyMatch("web/GitHub.com", "github"); !ok {
		t.Error("matching should ignore case")
	}
}

func TestFuzzyMatchPositionsPointAtMatchedRunes(t *testing.T) {
	m, ok := fuzzyMatch("web/github.com", "wgh")
	if !ok {
		t.Fatal("expected a match")
	}
	runes := []rune("web/github.com")
	want := []rune{'w', 'g', 'h'}
	if len(m.positions) != len(want) {
		t.Fatalf("positions = %v", m.positions)
	}
	for i, p := range m.positions {
		if runes[p] != want[i] {
			t.Errorf("position %d points at %q, want %q", i, runes[p], want[i])
		}
	}
}

func TestFilterRanksConsecutiveAndBoundaryMatchesFirst(t *testing.T) {
	names := []string{"misc/graph-hub-notes", "web/github.com"}
	got := filterItems(names, "ghub")
	if len(got) != 2 {
		t.Fatalf("expected both names to match, got %d", len(got))
	}
	if names[got[0].index] != "web/github.com" {
		t.Errorf("best match = %q, want web/github.com", names[got[0].index])
	}
}

func TestFilterPrefersTheShorterName(t *testing.T) {
	names := []string{"work/aws-production-root-account", "aws"}
	got := filterItems(names, "aws")
	if names[got[0].index] != "aws" {
		t.Errorf("best match = %q, want the exact short name", names[got[0].index])
	}
}

func TestFilterEmptyQueryKeepsOriginalOrder(t *testing.T) {
	names := []string{"a", "b", "c"}
	got := filterItems(names, "")
	if len(got) != 3 {
		t.Fatalf("expected all names, got %d", len(got))
	}
	for i, m := range got {
		if m.index != i {
			t.Errorf("position %d holds index %d; an empty query must not reorder", i, m.index)
		}
	}
}

func TestFilterIsStableForEqualScores(t *testing.T) {
	// Identical names score identically; the order must still be deterministic,
	// because a list that reshuffles under the cursor is how you delete the
	// wrong entry.
	names := []string{"dup", "dup", "dup"}
	first := filterItems(names, "dup")
	for i := 0; i < 20; i++ {
		got := filterItems(names, "dup")
		for j := range got {
			if got[j].index != first[j].index {
				t.Fatalf("ordering changed between runs at position %d", j)
			}
		}
	}
}

func TestFilterExcludesNonMatches(t *testing.T) {
	names := []string{"github", "gitlab", "email"}
	got := filterItems(names, "git")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
	for _, m := range got {
		if names[m.index] == "email" {
			t.Error("email should not match the query git")
		}
	}
}
