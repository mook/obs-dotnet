package rpm_test

import (
	"testing"

	"github.com/mook/obs-dotnet/generate-packages/pkg/rpm"
	"github.com/mook/obs-dotnet/generate-packages/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestParseVersion(t *testing.T) {
	testCases := map[string]rpm.Version{
		"1.0.0":         {Ver: "1.0.0"},
		"2:1.2.3":       {Epoch: utils.Ptr(uint64(2)), Ver: "1.2.3"},
		"1.2-3.4":       {Ver: "1.2", Rel: utils.Ptr("3.4")},
		"1:2.3.4-5.6.7": {Epoch: utils.Ptr(uint64(1)), Ver: "2.3.4", Rel: utils.Ptr("5.6.7")},
		"--":            {Rel: utils.Ptr("-")},
	}
	for input, expected := range testCases {
		t.Run(input, func(t *testing.T) {
			actual, err := rpm.ParseVersion(input)
			if assert.NoError(t, err) {
				assert.Equal(t, &expected, actual)
			}
		})
	}
}
