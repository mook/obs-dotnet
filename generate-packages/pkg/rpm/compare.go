package rpm

import (
	"cmp"
	"regexp"
	"strings"
)

var (
	versionSplitRE = regexp.MustCompile(`[0-9]+|[A-Za-z]+|\^|\~`)
)

// Compare two RPM versions, returning -1 if a < b, 1 if a > b, 0 if equal.
func Compare(a, b Version) int {
	if a.Epoch != nil && b.Epoch != nil {
		if *a.Epoch != *b.Epoch {
			// If epochs are different, compare them.
			return cmp.Compare(*a.Epoch, *b.Epoch)
		}
	}
	if a.Ver != b.Ver {
		return comparePart(a.Ver, b.Ver)
	}
	if a.Rel != nil && b.Rel != nil {
		return comparePart(*a.Rel, *b.Rel)
	}
	return 0
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// Compare part of a RPM version, either the version or the release.
func comparePart(a, b string) int {
	// RPM versions are a little insane: while it looks like dot-separated
	// segments, the dots don't actually matter.
	segsA := splitPart(a)
	segsB := splitPart(b)
	count := min(len(segsA), len(segsB))
	for i := 0; i < count; i++ {
		partA := segsA[i]
		partB := segsB[i]
		if partA == partB {
			// Simplify comparison by skipping equal parts
			continue
		}
		if partA == "~" || partB == "~" {
			// Tilde sorts before everything else, even caret.
			if partA == "~" {
				return -1
			}
			return 1
		}
		if partA == "^" || partB == "^" {
			// Caret sorts before everything else, unless we hit EOF, which we
			// can't do in this loop.
			if partA == "^" {
				return -1
			}
			return 1
		}
		isDigitA := isDigit(partA[0])
		isDigitB := isDigit(partB[0])
		if isDigitA != isDigitB {
			// Numeric segments are always newer than alpha segments.
			if isDigitA {
				return 1
			}
			return -1
		}
		if isDigitA {
			// If the parts are digits, remove leading zeros.
			partA = strings.TrimLeft(partA, "0")
			partB = strings.TrimLeft(partB, "0")
			if len(partA) != len(partB) {
				if len(partA) > len(partB) {
					return 1
				}
				return -1
			}
		}
		return cmp.Compare(partA, partB)
	}
	if len(segsA) == len(segsB) {
		// If the two strings all have the same segments, they're equal.
		return 0
	}
	// The longer one is newer, unless the next segment is ~, in which case it's
	// smaller.  Except when the other side is empty, in which case ~ is bigger
	// again.
	if len(segsA) > len(segsB) {
		if len(segsB) > 0 && segsA[len(segsB)] == "~" {
			return -1
		}
		return 1
	} else {
		// B is longer than A
		if len(segsA) > 0 && segsB[len(segsA)] == "~" {
			return 1
		}
		return -1
	}
}

func splitPart(input string) []string {
	return versionSplitRE.FindAllString(input, -1)
}
