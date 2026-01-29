// Suffix Array implementation for pattern search.
// Adapted from golang_dsa library.
package dsa

import (
	"sort"
)

// SuffixArray represents a suffix array with optional LCP array.
// Enables O(m log n) pattern search where m is pattern length, n is text length.
type SuffixArray struct {
	Text string // Original text
	SA   []int  // Suffix array: SA[i] = start position of i-th smallest suffix
	LCP  []int  // LCP array: LCP[i] = longest common prefix of SA[i] and SA[i-1]
	Rank []int  // Inverse suffix array: Rank[i] = position of suffix i in SA
}

// BuildSuffixArray constructs a suffix array for the given text.
// Uses prefix doubling algorithm.
// Time Complexity: O(n log n)
// Space Complexity: O(n)
func BuildSuffixArray(text string) *SuffixArray {
	n := len(text)
	if n == 0 {
		return &SuffixArray{Text: text, SA: []int{}, LCP: []int{}, Rank: []int{}}
	}

	sa := &SuffixArray{
		Text: text,
		SA:   make([]int, n),
		Rank: make([]int, n),
	}

	// Initialize suffix array with all positions
	for i := 0; i < n; i++ {
		sa.SA[i] = i
		sa.Rank[i] = int(text[i])
	}

	// Prefix doubling algorithm
	tmpRank := make([]int, n)
	for k := 1; k < n; k *= 2 {
		// Sort by (rank[i], rank[i+k]) pairs
		sort.Slice(sa.SA, func(i, j int) bool {
			if sa.Rank[sa.SA[i]] != sa.Rank[sa.SA[j]] {
				return sa.Rank[sa.SA[i]] < sa.Rank[sa.SA[j]]
			}
			ri := -1
			if sa.SA[i]+k < n {
				ri = sa.Rank[sa.SA[i]+k]
			}
			rj := -1
			if sa.SA[j]+k < n {
				rj = sa.Rank[sa.SA[j]+k]
			}
			return ri < rj
		})

		// Compute new ranks
		tmpRank[sa.SA[0]] = 0
		for i := 1; i < n; i++ {
			tmpRank[sa.SA[i]] = tmpRank[sa.SA[i-1]]

			prev, curr := sa.SA[i-1], sa.SA[i]
			if sa.Rank[prev] != sa.Rank[curr] {
				tmpRank[sa.SA[i]]++
			} else {
				rPrev := -1
				if prev+k < n {
					rPrev = sa.Rank[prev+k]
				}
				rCurr := -1
				if curr+k < n {
					rCurr = sa.Rank[curr+k]
				}
				if rPrev != rCurr {
					tmpRank[sa.SA[i]]++
				}
			}
		}

		copy(sa.Rank, tmpRank)

		// Early termination if all suffixes have unique ranks
		if sa.Rank[sa.SA[n-1]] == n-1 {
			break
		}
	}

	return sa
}

// BuildLCP computes the LCP array using Kasai's algorithm.
// Time Complexity: O(n)
func (sa *SuffixArray) BuildLCP() {
	n := len(sa.Text)
	if n == 0 {
		sa.LCP = []int{}
		return
	}

	sa.LCP = make([]int, n)
	h := 0

	for i := 0; i < n; i++ {
		if sa.Rank[i] == 0 {
			sa.LCP[0] = 0
			continue
		}

		j := sa.SA[sa.Rank[i]-1]

		for i+h < n && j+h < n && sa.Text[i+h] == sa.Text[j+h] {
			h++
		}

		sa.LCP[sa.Rank[i]] = h

		if h > 0 {
			h--
		}
	}
}

// Search finds all occurrences of pattern in text using binary search.
// Time Complexity: O(m log n) where m = len(pattern), n = len(text)
func (sa *SuffixArray) Search(pattern string) []int {
	if len(pattern) == 0 || len(sa.SA) == 0 {
		return []int{}
	}

	n := len(sa.SA)
	m := len(pattern)

	// Binary search for leftmost occurrence
	left := sort.Search(n, func(i int) bool {
		suffix := sa.Text[sa.SA[i]:]
		if len(suffix) < m {
			return suffix >= pattern[:len(suffix)]
		}
		return suffix[:m] >= pattern
	})

	// Binary search for rightmost occurrence
	right := sort.Search(n, func(i int) bool {
		suffix := sa.Text[sa.SA[i]:]
		if len(suffix) < m {
			return suffix > pattern[:len(suffix)]
		}
		return suffix[:m] > pattern
	})

	// Collect all matches
	var matches []int
	for i := left; i < right; i++ {
		pos := sa.SA[i]
		if pos+m <= len(sa.Text) && sa.Text[pos:pos+m] == pattern {
			matches = append(matches, pos)
		}
	}

	sort.Ints(matches)
	return matches
}

// SearchFirst finds the first occurrence of pattern.
// Returns -1 if not found.
func (sa *SuffixArray) SearchFirst(pattern string) int {
	matches := sa.Search(pattern)
	if len(matches) == 0 {
		return -1
	}
	return matches[0]
}

// Count returns the number of occurrences of pattern.
func (sa *SuffixArray) Count(pattern string) int {
	return len(sa.Search(pattern))
}

// GetSuffix returns the suffix starting at position SA[i].
func (sa *SuffixArray) GetSuffix(i int) string {
	if i < 0 || i >= len(sa.SA) {
		return ""
	}
	return sa.Text[sa.SA[i]:]
}
