package main

import (
	"context"
	_ "embed"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/mook/obs-dotnet/generate-packages/pkg/httpfs"
	"github.com/mook/obs-dotnet/generate-packages/pkg/repomd"
	"github.com/mook/obs-dotnet/generate-packages/pkg/rpm"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

type overrideEntry struct {
	Weight int    `yaml:"weight,omitempty"`
	Lines  string `yaml:"lines"`
}

var (
	//go:embed overrides.yaml
	overridesRaw  []byte
	loadOverrides = sync.OnceValues(func() (map[string]map[string]overrideEntry, error) {
		var result map[string]map[string]overrideEntry
		if err := yaml.Unmarshal(overridesRaw, &result); err != nil {
			return nil, err
		}
		return result, nil
	})
)

// packageWriter writes out a package definition
type packageWriter struct {
	sync.Once
	fs  *httpfs.HttpFs
	pkg *repomd.PrimaryPackage
}

// write the package definition.  This is the main entry point for packageWriter.
func (w *packageWriter) write(ctx context.Context, pkgs []*repomd.PrimaryPackage) error {
	group, ctx := errgroup.WithContext(ctx)
	w.Do(func() {
		slog.Debug("Download", "pkg", w.pkg)
		group.Go(func() error {
			pkgDir, err := filepath.Abs(w.pkg.Name)
			if err != nil {
				return err
			}
			if err = os.MkdirAll(pkgDir, 0o755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", pkgDir, err)
			}

			// Download the RPM file and write the spec file
			group.Go(func() error {
				rpmPath, err := w.download(pkgDir)
				if err != nil {
					return err
				}
				return w.writeSpec(ctx, pkgDir, rpmPath)
			})

			// Write the _service file
			group.Go(func() error {
				return w.writeService(pkgDir)
			})

			group.Go(func() error {
				return w.writeLintConfig(pkgDir)
			})

			return nil
		})
		wantedPackages := slices.Concat(
			w.pkg.Format.Requires,
			w.pkg.Format.Suggests,
			w.pkg.Format.Recommends,
			w.pkg.Format.Supplements,
			w.pkg.Format.Enhances,
		)
		for _, nextEntry := range wantedPackages {
			var pkg *repomd.PrimaryPackage
			if w.pkg.Name == initialPackage && options.version.Ver != "" {
				// If we're looking at the initial package, set the version of
				// its dependencies if possible.
				if nextEntry.Ver == "" {
					modifiedEntry := nextEntry
					modifiedEntry.Version = options.version
					modifiedEntry.Flags = rpm.EQ
					slog.DebugContext(ctx, "checking override", "override", modifiedEntry)
					pkg = findPackage(pkgs, modifiedEntry)
				}
				if pkg == nil {
					// If we can't find the override version, fallback to using
					// the default version.
					slog.DebugContext(ctx, "failed to find override", "fallback", nextEntry)
					pkg = findPackage(pkgs, nextEntry)
				}
			} else {
				pkg = findPackage(pkgs, nextEntry)
			}
			if pkg != nil {
				var ok bool
				newWriter := &packageWriter{pkg: pkg, fs: w.fs}
				packages.Lock()
				if _, ok = packages.mapping[pkg.Name]; !ok {
					packages.mapping[pkg.Name] = newWriter
				}
				packages.Unlock()
				if !ok {
					group.Go(func() error {
						return newWriter.write(ctx, pkgs)
					})
				}
			}
		}

	})
	return group.Wait()
}

// download the package, writing the file to disk.  Returns the path to the
// written RPM file.
func (w *packageWriter) download(pkgDir string) (string, error) {
	download, err := w.fs.Open(w.pkg.Location.HRef)
	if err != nil {
		return "", fmt.Errorf("failed to download %s: %w", w.pkg, err)
	}
	defer download.Close()
	stat, err := download.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to get download %s info: %w", w.pkg, err)
	}
	outPath := filepath.Join(pkgDir, stat.Name())
	outFile, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("failed to create %s file %s: %w", w.pkg, outPath, err)
	}
	defer outFile.Close()
	n, err := io.Copy(outFile, download)
	if err != nil {
		_ = outFile.Close()
		_ = os.Remove(outPath)
		return "", fmt.Errorf("failed to download %s: %w", w.pkg, err)
	}
	if stat.Size() > 0 && n != stat.Size() {
		_ = outFile.Close()
		_ = os.Remove(outPath)
		return "", fmt.Errorf("failed to download %s: got %d/%d bytes", w.pkg, n, stat.Size())
	}
	return outPath, nil
}

// rpmSectionHeaders contain the names of the RPM section headers (including the
// leading percent sign).  We use this list to detect when a section has ended
// so we can insert any lines we need into the end of the previous section.
// Note that %description is a sub-section, but we need to insert lines into the
// preamble before it, so we consider it a section.
const rpmSectionHeaders = `
	%description
	%prep
	%build
	%install
	%check
	%files
	%changelog
	%verify
	%pre %post %preun %postun
	%pretrans %posttrans %preuntrans %postuntrans
	%triggerprein %triggerin %triggerun %triggerpostun
	%filetriggerin %filetriggerun %filetriggerpostun
	%transfiletriggerin %transfiletriggerun %transfiletriggerpostun
	`

func (w *packageWriter) writeSpec(ctx context.Context, pkgDir, rpmPath string) error {
	specPath := filepath.Join(pkgDir, w.pkg.Name+".spec")
	// rpmrebuild emits log lines starting with `(GenRpmQf)` in stdout, so we
	// have to tell it to write to a file instead.
	tempSpec, err := os.CreateTemp(pkgDir, w.pkg.Name+"-*.spec")
	if err != nil {
		return fmt.Errorf("failed to create temporary RPM spec: %w", err)
	}
	defer os.Remove(tempSpec.Name())
	if err = tempSpec.Close(); err != nil {
		return fmt.Errorf("failed to close temporary RPM spec: %w", err)
	}
	cmd := exec.CommandContext(ctx, "rpmrebuild",
		"--spec-only="+tempSpec.Name(),
		"--package", rpmPath)
	cmd.Stderr = os.Stderr
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		cmd.Stdout = os.Stdout
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate RPM spec file: %w", err)
	}
	buf, err := os.ReadFile(tempSpec.Name())
	if err != nil {
		return fmt.Errorf("failed to read generated RPM spec: %w", err)
	}

	// Insert a %define rpm_url so we can reference it later.
	lines := []string{"%define rpm_url " + w.fs.BuildURL(w.pkg.Location.HRef).String()}
	lines = append(lines, strings.Split(string(buf), "\n")...)

	// Write the changelog to a separate file
	changelogIndex := slices.Index(lines, "%changelog")
	if changelogIndex >= 0 {
		if err = w.writeChangelog(pkgDir, lines[changelogIndex+1:]); err != nil {
			return fmt.Errorf("failed to write changelog: %w", err)
		}
		lines = lines[:changelogIndex+1]
	}

	if lines, err = w.overrideSpec(lines); err != nil {
		return err
	}

	if err := os.WriteFile(specPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return fmt.Errorf("failed to write RPM spec file: %w", err)
	}
	return nil
}

// overrideSpec modifies the lines of the spec file according to the configuration
// in overrides.yaml.
func (w *packageWriter) overrideSpec(lines []string) ([]string, error) {
	rpmSectionHeaderMap := map[string]struct{}{"%preamble": {}}
	for _, header := range strings.Fields(rpmSectionHeaders) {
		rpmSectionHeaderMap[header] = struct{}{}
	}

	allOverrides, err := loadOverrides()
	if err != nil {
		return nil, fmt.Errorf("failed to load overrides: %w", err)
	}

	// Load overrides matching this package
	overridesData := make(map[string][]overrideEntry)
	for packageGlob, entries := range allOverrides {
		match, err := path.Match(packageGlob, w.pkg.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to read override: %q is a bad glob", packageGlob)
		}
		if match {
			for section, entry := range entries {
				if _, ok := rpmSectionHeaderMap[section]; !ok {
					return nil, fmt.Errorf("override %s section %s is invalid", packageGlob, section)
				}
				overridesData[section] = append(overridesData[section], entry)
			}
		}
	}
	overrides := make(map[string][]string)
	for k, v := range overridesData {
		slices.SortFunc(v, func(a, b overrideEntry) int {
			return a.Weight - b.Weight
		})
		for _, entry := range v {
			overrides[k] = append(overrides[k], strings.Split(entry.Lines, "\n")...)
		}
	}

	// Go through each line and check for overrides
	section := "%preamble"
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		for word := range strings.FieldsSeq(line) {
			if _, ok := rpmSectionHeaderMap[word]; ok {
				// The line starts a new section; insert the lines and change
				// sections.
				result = append(result, overrides[section]...)
				section = word
			}
			break
		}
		result = append(result, line)
	}

	return append(result, overrides[section]...), nil
}

func (w *packageWriter) writeChangelog(pkgDir string, lines []string) error {
	changelogPath := filepath.Join(pkgDir, fmt.Sprintf("%s.changes", w.pkg.Name))
	return os.WriteFile(changelogPath, []byte(strings.Join(lines, "\n")), 0o644)
}

func (w *packageWriter) writeLintConfig(pkgDir string) error {
	var lines []string
	for _, checkName := range []string{
		"arch-dependent-file-in-usr-share",
	} {
		lines = append(lines, fmt.Sprintf("setBadness('%s', 0)", checkName))
	}
	configPath := filepath.Join(pkgDir, fmt.Sprintf("%s-rpmlintrc", w.pkg.Name))
	return os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0o644)
}

type serviceParam struct {
	XMLName string `xml:"param"`
	Name    string `xml:"name,attr"`
	Value   string `xml:",chardata"`
}
type serviceElement struct {
	XMLName string `xml:"service"`
	Name    string `xml:"name,attr"`
	Params  []serviceParam
}
type serviceTemplate struct {
	XMLName  string `xml:"services"`
	Services []serviceElement
}

// Write the _service file in the given directory.
func (w *packageWriter) writeService(pkgDir string) error {
	service := serviceTemplate{
		Services: []serviceElement{
			{Name: "format_spec_file"},
			{Name: "download_files"},
		},
	}
	buf, err := xml.MarshalIndent(service, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to build %s _service file: %w", w.pkg, err)
	}
	servicePath := filepath.Join(pkgDir, "_service")
	if err = os.WriteFile(servicePath, buf, 0o644); err != nil {
		_ = os.Remove(servicePath)
		return fmt.Errorf("failed to write %s _servie file: %w", w.pkg, err)
	}
	slog.Debug("Wrote service file", "package", w.pkg, "path", servicePath)
	return nil
}
