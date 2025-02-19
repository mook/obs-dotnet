package utils_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/mook/obs-dotnet/generate-packages/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestInsertLines(t *testing.T) {
	for pos, posText := range []string{"start", "middle", "end"} {
		for relative, relText := range []string{"before", "after"} {
			t.Run(fmt.Sprintf("insert %s %s", relText, posText), func(t *testing.T) {
				input := []string{
					"first line",
					"second line",
					"third line",
				}
				additions := []string{
					"new line",
					"added line",
				}
				var matcher *regexp.Regexp
				var expected []string
				isBefore := relative == 0
				expected = append(expected, input[:pos+relative]...)
				expected = append(expected, additions...)
				expected = append(expected, input[pos+relative:]...)
				switch posText {
				case "start":
					matcher = regexp.MustCompile(`^first`)
				case "middle":
					matcher = regexp.MustCompile(`^second`)
				case "end":
					matcher = regexp.MustCompile(`^third`)
				default:
					t.Fatalf("unexpected position %s", posText)
				}
				actual := utils.InsertLines(input, additions, matcher, isBefore)
				assert.Equal(t, expected, actual)
			})
		}
	}
}
