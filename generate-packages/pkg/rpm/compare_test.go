package rpm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitPart(t *testing.T) {
	testCases := map[string][]string{
		"hello world": {"hello", "world"},
		"1.0.2":       {"1", "0", "2"},
		"1.0.2^":      {"1", "0", "2", "^"},
		"1.0~rc1":     {"1", "0", "~", "rc", "1"},
		"~~~":         {"~", "~", "~"},
	}
	for input, expected := range testCases {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, expected, splitPart(input))
		})
	}
}

func TestComparePart(t *testing.T) {
	// See `rpmdev-vercmp` for expected output.
	testCases := map[string]int{
		"1.0.0  1.0.0":   0,
		"1.0.0~ 1.0.0":   -1, // Tilde is smaller than nothing
		"1.0.0  1.0.0~":  1,  // Same
		"1.0.0^ 1.0.0":   1,  // Caret is bigger than nothing
		"1.0.0^ 1.0.0.a": -1, // Caret is smaller than alpha
		"0      a":       1,  // Digits are bigger than alpha
		"1.a    1.0":     -1, // Digits are bigger than alpha
		"^      ":        1,  // Caret is bigger than nothing
		"~      0":       -1,
		"~      ":        1,  // Tilde bigger than nothing (special case?)
		"~      ^":       -1, // Tilde smaller than caret
		"1.02   1.1":     1,
		"1.2    1.10":    -1, // Longest run of digits is bigger
		"1.002  1.10":    -1, // ... But remove leading zeros
	}
	for input, expected := range testCases {
		t.Run(input, func(t *testing.T) {
			left, right, _ := strings.Cut(input, " ")
			assert.Equal(t, expected,
				comparePart(strings.TrimSpace(left), strings.TrimSpace(right)))
		})
	}
}
