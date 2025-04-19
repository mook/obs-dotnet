package rpm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mook/obs-dotnet/generate-packages/pkg/utils"
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

// Set a version from an input string, implementing [flag.Value].
// The version may hav nil values for epoch and release.
func (v *Version) Set(input string) error {
	if epochIndex := strings.Index(input, ":"); epochIndex >= 0 {
		epoch, err := strconv.ParseUint(input[:epochIndex], 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse epoch from %s: %w", input, err)
		}
		v.Epoch = &epoch
		input = input[epochIndex+1:]
	}
	if relIndex := strings.Index(input, "-"); relIndex >= 0 {
		v.Ver = input[:relIndex]
		v.Rel = utils.Ptr(input[relIndex+1:])
	} else {
		v.Ver = input
	}
	return nil
}

// Parse a version string.  The version always contains epoch and release,
// defaulting to zero and empty respectively.
func ParseVersion(input string) (*Version, error) {
	v := &Version{}
	if err := v.Set(input); err != nil {
		return nil, err
	}
	if v.Epoch == nil {
		v.Epoch = utils.Ptr(uint64(0))
	}
	if v.Rel == nil {
		v.Rel = utils.Ptr("")
	}
	return v, nil
}
