package rpm_test

import (
	"strings"
	"testing"
	"unicode"

	"github.com/mook/obs-dotnet/generate-packages/pkg/rpm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type SimpleNamedVersion struct {
	Name    string
	Version rpm.Version
}

func (v *SimpleNamedVersion) ToName() string {
	return v.Name
}
func (v *SimpleNamedVersion) ToVersion() rpm.Version {
	return v.Version
}

func TestEntryMatch(t *testing.T) {
	// PkgName PkgVersion OP EntryName EntryVersion
	testCases := map[string]bool{
		"pkg 0:1.0.0 EQ pkg 1.0.0": true,
		"pkg 2.1.0-1 GE pkg 2.1.0": true,
		"pkg 1.0.1.2 GT bob 1.0.0": false, // Name mismatch
		"pkg 2.0.0   GT pkg 1.0.0": true,
	}
	for input, expected := range testCases {
		t.Run(input, func(t *testing.T) {
			parts := strings.FieldsFunc(input, unicode.IsSpace)
			require.Len(t, parts, 5, "invalid test input")
			entryVersion, err := rpm.ParseVersion(parts[4])
			require.NoError(t, err, "failed to parse entry version")
			pkgVersion, err := rpm.ParseVersion(parts[1])
			require.NoError(t, err, "failed to parse package version")
			entry := rpm.Entry{
				Name:    parts[3],
				Version: *entryVersion,
				Flags:   rpm.CompareOp(parts[2]),
			}
			namedVersion := SimpleNamedVersion{
				Name:    parts[0],
				Version: *pkgVersion,
			}
			assert.Equal(t, expected, entry.Match(&namedVersion))
		})
	}
}
