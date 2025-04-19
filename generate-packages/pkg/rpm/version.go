package rpm

import (
	"fmt"
	"strconv"
	"strings"
)

// Versions in RPM; epoch and release are optional for matching purposes.
type Version struct {
	Epoch *uint64 `xml:"epoch,attr,omitempty"`
	Ver   string  `xml:"ver,attr"`
	Rel   *string `xml:"rel,attr,omitempty"`
}

func (v *Version) String() string {
	result := v.Ver
	if v.Epoch != nil && *v.Epoch > 0 {
		result = fmt.Sprintf("%d:%s", *v.Epoch, v.Ver)
	}
	if v.Rel != nil && *v.Rel != "" {
		result += "-" + *v.Rel
	}
	return result
}

func (v *Version) Set(input string) error {
	if epochIndex := strings.Index(input, ":"); epochIndex >= 0 {
		epoch, err := strconv.ParseUint(input[:epochIndex], 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse epoch from %s: %w", input, err)
		}
		if epoch < 0 {
			return fmt.Errorf("failed to parse epoch from %s: negative epoch", input)
		}
		v.Epoch = &epoch
		input = input[epochIndex+1:]
	}
	rel := ""
	if relIndex := strings.Index(input, "-"); relIndex >= 0 {
		v.Ver = input[:relIndex]
		rel = input[relIndex+1:]
	} else {
		v.Ver = input
	}
	v.Rel = &rel
	return nil
}

// Parse a version string.  The version always contains epoch and release,
// defaulting to zero and empty respectively.
func ParseVersion(input string) (*Version, error) {
	v := &Version{}
	err := v.Set(input)
	return v, err
}
