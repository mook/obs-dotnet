package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/mook/obs-dotnet/generate-packages/pkg/httpfs"
	"github.com/mook/obs-dotnet/generate-packages/pkg/repomd"
	"github.com/mook/obs-dotnet/generate-packages/pkg/rpm"
	"github.com/mook/obs-dotnet/generate-packages/pkg/utils"
	"golang.org/x/sync/errgroup"
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

			return nil
		})
		var wantedPackages []rpm.Entry
		for _, list := range [][]rpm.Entry{
			w.pkg.Format.Requires,
			w.pkg.Format.Suggests,
			w.pkg.Format.Recommends,
			w.pkg.Format.Supplements,
			w.pkg.Format.Enhances,
		} {
			wantedPackages = append(wantedPackages, list...)
		}
		for _, nextEntry := range wantedPackages {
			pkg := findPackage(pkgs, nextEntry)
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
	cmd.Stdout = os.Stdout
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate RPM spec file: %w", err)
	}
	buf, err := os.ReadFile(tempSpec.Name())
	if err != nil {
		return fmt.Errorf("failed to read generated RPM spec: %w", err)
	}

	rpmName := filepath.Base(rpmPath)
	lines := strings.Split(string(buf), "\n")

	// Write the changelog to a separate file
	changelogIndex := slices.Index(lines, "%changelog")
	if changelogIndex >= 0 {
		if err = w.writeChangelog(ctx, pkgDir, lines[changelogIndex+1:]); err != nil {
			return fmt.Errorf("failed to write changelog: %w", err)
		}
		lines = lines[:changelogIndex+1]
	}

	lines = utils.InsertLines(lines, []string{
		"BuildRequires: rpm",
		fmt.Sprintf("Source: %s", rpmName),
	}, regexp.MustCompile(`^%description`), true)

	lines = utils.InsertLines(lines, []string{
		"%install",
		"set -x",
		fmt.Sprintf("rpm2cpio %%{_sourcedir}/%s | cpio --extract --make-directories --preserve-modification-time --verbose --directory %%{buildroot}", rpmName),
	}, regexp.MustCompile(`^%files`), true)

	if err := os.WriteFile(specPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return fmt.Errorf("failed to write RPM spec file: %w", err)
	}
	return nil
}

func (w *packageWriter) writeChangelog(ctx context.Context, pkgDir string, lines []string) error {
	changelogPath := filepath.Join(pkgDir, fmt.Sprintf("%s.changes", w.pkg.Name))
	return os.WriteFile(changelogPath, []byte(strings.Join(lines, "\n")), 0o644)
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
			{
				Name: "download_url",
				Params: []serviceParam{
					{Name: "url", Value: w.fs.BuildURL(w.pkg.Location.HRef).String()},
				},
			},
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
