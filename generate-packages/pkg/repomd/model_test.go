package repomd_test

import (
	"embed"
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/mook/obs-dotnet/generate-packages/pkg/repomd"
	"github.com/mook/obs-dotnet/generate-packages/pkg/rpm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata
var testdata embed.FS

type renamedFS struct {
	fs embed.FS
}

func (f *renamedFS) Open(name string) (fs.File, error) {
	renamed := path.Join("testdata", strings.TrimPrefix(name, "repodata/"))
	return f.fs.Open(renamed)
}

func TestParseRepoMetadata(t *testing.T) {
	result, err := repomd.ParseRepoMetadata(&renamedFS{testdata})
	require.NoError(t, err, "failed to parse repo")
	assert.NotNil(t, result)
	assert.True(t, slices.ContainsFunc(result.Data, func(data repomd.RepoMDData) bool {
		return data.Type == "filelists"
	}))
}

func TestParsePrimary(t *testing.T) {
	primary, err := repomd.ParsePrimary(&renamedFS{testdata})
	require.NoError(t, err)
	assert.True(t, slices.ContainsFunc(primary.Packages, func(pkg *repomd.PrimaryPackage) bool {
		if pkg.Name != "dotnet-sdk-9.0" {
			return false
		}
		if pkg.Version.Ver != "9.0.101" {
			return false
		}
		assert.Equal(t, pkg.Location.HRef, "Packages/d/dotnet-sdk-9.0-9.0.101-1.x86_64.rpm")
		assert.Equal(t, pkg.Format.License, "MIT")
		assert.Equal(t, pkg.Format.SourceRPM, "dotnet-sdk-9.0-9.0.101-1.src.rpm")
		assert.True(t, slices.ContainsFunc(pkg.Format.Requires, func(entry rpm.Entry) bool {
			return entry.Name == "dotnet-runtime-9.0"
		}))
		return true
	}))
}
