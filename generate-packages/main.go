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
	"slices"
	"sync"

	"github.com/mook/obs-dotnet/generate-packages/pkg/httpfs"
	"github.com/mook/obs-dotnet/generate-packages/pkg/repomd"
	"github.com/mook/obs-dotnet/generate-packages/pkg/rpm"
	"github.com/mook/obs-dotnet/generate-packages/pkg/utils"
	"golang.org/x/sync/errgroup"
)

const (
	repository = "https://packages.microsoft.com/opensuse/15/prod/"
	repoMeta   = "prod.repo"
)

var packages struct {
	sync.Mutex
	mapping map[string]*packageWriter
}

type packageWriter struct {
	sync.Once
	fs  *httpfs.HttpFs
	pkg *repomd.PrimaryPackage
}

func (w *packageWriter) write(ctx context.Context, pkgs []*repomd.PrimaryPackage) error {
	group, ctx := errgroup.WithContext(ctx)
	w.Do(func() {
		slog.Debug("Download", "pkg", w.pkg)
		group.Go(func() error {
			pkgDir, err := filepath.Abs(filepath.Join("..", w.pkg.Name))
			if err != nil {
				return err
			}
			if err = os.MkdirAll(pkgDir, 0o755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", pkgDir, err)
			}

			// Download the RPM file and write the spec file
			group.Go(func() error {
				rpmPath, err := w.Download(pkgDir)
				if err != nil {
					return err
				}
				return w.WriteSpec(ctx, pkgDir, rpmPath)
			})

			// Write the _service file
			group.Go(func() error {
				return w.WriteService(pkgDir)
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

// Download the package, writing the file to disk.  Returns the path to the
// written RPM file.
func (w *packageWriter) Download(pkgDir string) (string, error) {
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

func (w *packageWriter) WriteSpec(ctx context.Context, pkgDir, rpmPath string) error {
	cmd := exec.CommandContext(ctx, "rpmrebuild",
		"--spec-only="+filepath.Join(pkgDir, w.pkg.Name+".spec"),
		"--package", rpmPath)
	cmd.Stdout = os.Stdout
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate RPM spec file: %w", err)
	}
	return nil
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
func (w *packageWriter) WriteService(pkgDir string) error {
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

func findPackage(pkgs []*repomd.PrimaryPackage, entry rpm.Entry) *repomd.PrimaryPackage {
	pkgsSeq := utils.Filter(pkgs, func(pkg *repomd.PrimaryPackage) bool {
		return entry.Match(pkg)
	})
	if len(pkgsSeq) < 1 {
		slog.Debug("could not find matching package", "package", entry.String())
		return nil
	}
	maxPkg := slices.MaxFunc(pkgsSeq, func(a, b *repomd.PrimaryPackage) int {
		return rpm.Compare(a.Version, b.Version)
	})
	return maxPkg
}

func run(ctx context.Context) error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	fs, err := httpfs.NewHttpFs(repository)
	if err != nil {
		return fmt.Errorf("error creating fs: %w", err)
	}
	primary, err := repomd.ParsePrimary(fs)
	if err != nil {
		return fmt.Errorf("error parsing repo: %w", err)
	}
	initalPkg := findPackage(primary.Packages, rpm.Entry{
		Name: "dotnet-sdk-9.0",
	})
	if err != nil {
		return fmt.Errorf("failed to get dotnet-sdk package: %w", err)
	}
	writer := &packageWriter{pkg: initalPkg, fs: fs}
	packages.Lock()
	packages.mapping = make(map[string]*packageWriter)
	packages.mapping[initalPkg.Name] = writer
	packages.Unlock()
	return writer.write(ctx, primary.Packages)
}

func main() {
	if err := run(context.Background()); err != nil {
		slog.Error("", "error", err)
		os.Exit(1)
	}
}
