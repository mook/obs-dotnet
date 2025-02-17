package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
	pkg *repomd.PrimaryPackage
}

func (w *packageWriter) write(ctx context.Context, pkgs []*repomd.PrimaryPackage) error {
	group, ctx := errgroup.WithContext(ctx)
	w.Do(func() {
		slog.Debug("Download", "pkg", w.pkg)
		group.Go(func() error {
			/*
				pkgDir, err := filepath.Abs(w.pkg.Name)
				if err != nil {
					return err
				}
				if err = os.MkdirAll(pkgDir, 0o755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", pkgDir, err)
				}
			*/
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
				newWriter := &packageWriter{pkg: pkg}
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
	writer := &packageWriter{pkg: initalPkg}
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
