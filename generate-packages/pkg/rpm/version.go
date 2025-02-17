package rpm

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Epoch uint   `xml:"epoch,attr,omitempty"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr,omitempty"`
}

func (v *Version) String() string {
	result := v.Ver
	if v.Epoch > 0 {
		result = fmt.Sprintf("%d:%s", v.Epoch, v.Ver)
	}
	if v.Rel != "" {
		result += "-" + v.Rel
	}
	return result
}

func ParseVersion(input string) (*Version, error) {
	var result Version
	if epochIndex := strings.Index(input, ":"); epochIndex >= 0 {
		epoch, err := strconv.Atoi(input[:epochIndex])
		if err != nil {
			return nil, fmt.Errorf("failed to parse epoch from %s: %w", input, err)
		}
		if epoch < 0 {
			return nil, fmt.Errorf("failed to parse epoch from %s: negative epoch", input)
		}
		result.Epoch = uint(epoch)
		input = input[epochIndex+1:]
	}
	if relIndex := strings.Index(input, "-"); relIndex >= 0 {
		result.Ver = input[:relIndex]
		result.Rel = input[relIndex+1:]
	} else {
		result.Ver = input
	}
	return &result, nil
}
