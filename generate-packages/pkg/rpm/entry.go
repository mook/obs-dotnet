package rpm

import (
	"encoding/xml"
	"fmt"
)

type CompareOp string

const (
	EQ = CompareOp("EQ")
	GE = CompareOp("GE")
	LE = CompareOp("LE")
	LT = CompareOp("LT")
	GT = CompareOp("GT")
)

type NamedVersion = interface {
	ToVersion() Version
	ToName() string
}

type Entry struct {
	XMLName xml.Name `xml:"http://linux.duke.edu/metadata/rpm entry"`
	Pre     bool     `xml:"pre,attr,omitempty"`
	Kind    string   `xml:"kind,attr,omitempty"`
	Name    string   `xml:"name,attr"`
	Version
	Flags CompareOp `xml:"flags,attr,omitempty"`
}

func (e *Entry) Match(pkg NamedVersion) bool {
	if pkg.ToName() != e.Name {
		return false
	}
	var compareResult = Compare(pkg.ToVersion(), e.Version)
	switch e.Flags {
	case EQ:
		return compareResult == 0
	case GE:
		return compareResult >= 0
	case LE:
		return compareResult <= 0
	case LT:
		return compareResult < 0
	case GT:
		return compareResult > 0
	default:
		return true
	}
}

func (e *Entry) String() string {
	if e.Flags == "" {
		return e.Name
	}
	return fmt.Sprintf("%s %s %s", e.Name, e.Flags, &e.Version)
}
