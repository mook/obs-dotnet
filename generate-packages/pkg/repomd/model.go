package repomd

import (
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"

	"github.com/mook/obs-dotnet/generate-packages/pkg/rpm"
)

type RepoMD struct {
	XMLName  xml.Name     `xml:"http://linux.duke.edu/metadata/repo repomd"`
	Revision uint         `xml:"revision,omitempty"`
	Data     []RepoMDData `xml:"data"`
}
type RepoMDData struct {
	XMLName      xml.Name       `xml:"http://linux.duke.edu/metadata/repo data"`
	Type         RepoMDDataType `xml:"type,attr"`
	Location     YUMLocation    `xml:"location"`
	Checksum     string         `xml:"checksum,omitempty"`
	Size         uint           `xml:"size,omitempty"`
	OpenSize     uint           `xml:"open-size,omitempty"`
	OpenChecksum string         `xml:"open-checksum,omitempty"`
}
type RepoMDDataType string

const (
	RepoMDDataTypeDeltaInfo  = RepoMDDataType("deltainfo")
	RepoMDDataTypeFileLists  = RepoMDDataType("filelists")
	RepoMDDataTypeOther      = RepoMDDataType("other")
	RepoMDDataTypePrimary    = RepoMDDataType("primary")
	RepoMDDataTypeSUSEData   = RepoMDDataType("susedata")
	RepoMDDataTypeSUSEInfo   = RepoMDDataType("suseinfo")
	RepoMDDataTypeUpdateInfo = RepoMDDataType("updateinfo")
	RepoMDDataTypePatches    = RepoMDDataType("patches")
	RepoMDDataTypeProducts   = RepoMDDataType("products")
	RepoMDDataTypeProduct    = RepoMDDataType("product")
	RepoMDDataTypePatterns   = RepoMDDataType("patterns")
	RepoMDDataTypePattern    = RepoMDDataType("pattern")
)

type PrimaryMetadata struct {
	XMLName  xml.Name          `xml:"http://linux.duke.edu/metadata/common metadata"`
	Packages []*PrimaryPackage `xml:"package"`
}

type PrimaryPackage struct {
	Type        string      `xml:"type,attr"`
	Name        string      `xml:"name"`
	Arch        string      `xml:"arch"`
	Version     rpm.Version `xml:"version"`
	Checksum    RPMChecksum `xml:"checksum"`
	Summary     string      `xml:"summary"`     // Might have localization?
	Description string      `xml:"description"` // Might have localization?
	Packager    string      `xml:"packager,omitempty"`
	URL         string      `xml:"url,omitempty"`
	Time        YUMTime     `xml:"time"`
	Size        YUMSize     `xml:"size"`
	Location    YUMLocation `xml:"location"`
	Format      RPMFormat   `xml:"format"`
}

func (p *PrimaryPackage) ToName() string {
	return p.Name
}

func (p *PrimaryPackage) ToVersion() rpm.Version {
	return p.Version
}

func (p *PrimaryPackage) String() string {
	return fmt.Sprintf("%s %s", p.Name, &p.Version)
}

type RPMChecksum struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}
type RPMFormat struct {
	License     string `xml:"http://linux.duke.edu/metadata/rpm license"`
	Vendor      string `xml:"http://linux.duke.edu/metadata/rpm vendor"`
	Group       string `xml:"http://linux.duke.edu/metadata/rpm group"`
	BuildHost   string `xml:"http://linux.duke.edu/metadata/rpm buildhost"`
	SourceRPM   string `xml:"http://linux.duke.edu/metadata/rpm sourcerpm,omitempty"`
	HeaderRange struct {
		Start uint `xml:"start,attr"`
		End   uint `xml:"end,attr"`
	} `xml:"http://linux.duke.edu/metadata/rpm header-range"`
	Provides    []rpm.Entry `xml:"http://linux.duke.edu/metadata/rpm provides>entry"`
	Requires    []rpm.Entry `xml:"http://linux.duke.edu/metadata/rpm requires>entry"`
	Conflicts   []rpm.Entry `xml:"http://linux.duke.edu/metadata/rpm conflicts>entry"`
	Obsoletes   []rpm.Entry `xml:"http://linux.duke.edu/metadata/rpm obsoletes>entry"`
	Suggests    []rpm.Entry `xml:"http://linux.duke.edu/metadata/rpm suggests>entry"`
	Recommends  []rpm.Entry `xml:"http://linux.duke.edu/metadata/rpm recommends>entry"`
	Supplements []rpm.Entry `xml:"http://linux.duke.edu/metadata/rpm supplements>entry"`
	Enhances    []rpm.Entry `xml:"http://linux.duke.edu/metadata/rpm enhances>entry"`
	Files       []YUMFile   `xml:"file"`
}

type YUMTime struct {
	XMLName xml.Name `xml:"http://linux.duke.edu/metadata/common time"`
	File    uint     `xml:"file,attr"`
	Build   uint     `xml:"build,attr"`
}
type YUMSize struct {
	XMLName xml.Name `xml:"http://linux.duke.edu/metadata/common size"`
	Package uint     `xml:"package,attr"`
}
type YUMLocation struct {
	HRef string `xml:"href,attr"`
}
type YUMFile struct {
	XMLName xml.Name `xml:"http://linux.duke.edu/metadata/common file"`
	Type    string   `xml:"type,attr,omitempty"`
	Name    string   `xml:",chardata"`
}

func ParseRepoMetadata(fs fs.FS) (*RepoMD, error) {
	file, err := fs.Open("repodata/repomd.xml")
	if err != nil {
		return nil, fmt.Errorf("failed to open repo metadata: %w", err)
	}
	defer file.Close()
	var metadata RepoMD
	if err = xml.NewDecoder(file).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode repo metadata: %w", err)
	}
	return &metadata, nil
}

func ParsePrimary(fs fs.FS) (*PrimaryMetadata, error) {
	metadata, err := ParseRepoMetadata(fs)
	if err != nil {
		return nil, fmt.Errorf("error parsing repo metadata: %w", err)
	}
	primaryIndex := slices.IndexFunc(metadata.Data, func(data RepoMDData) bool {
		return data.Type == "primary"
	})
	if primaryIndex < 0 {
		return nil, fmt.Errorf("could not find primary data")
	}
	href := metadata.Data[primaryIndex].Location.HRef
	file, err := fs.Open(href)
	if err != nil {
		return nil, fmt.Errorf("failed to open primary index: %w", err)
	}
	defer file.Close()
	var reader io.Reader
	switch path.Ext(href) {
	case ".gz":
		reader, err = gzip.NewReader(file)
		if closer, ok := reader.(io.Closer); ok {
			defer closer.Close()
		}
	default:
		reader = file
	}
	if err != nil {
		return nil, err
	}
	var result PrimaryMetadata
	if err = xml.NewDecoder(reader).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
