package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/mook/obs-dotnet/generate-packages/pkg/httpfs"
	"github.com/mook/obs-dotnet/generate-packages/pkg/repomd"
	"github.com/mook/obs-dotnet/generate-packages/pkg/rpm"
	"github.com/mook/obs-dotnet/generate-packages/pkg/utils"
)

const (
	repository     = "https://packages.microsoft.com/opensuse/15/prod/"
	repoMeta       = "prod.repo"
	initialPackage = "dotnet-sdk-9.0"
)

var (
	options struct {
		verbose bool
		version rpm.Version
	}

	packages struct {
		sync.Mutex
		mapping map[string]*packageWriter
	}
)

func parseFlags() {
	flag.BoolVar(&options.verbose, "verbose", false, "enable extra logging")
	flag.Var(&options.version, "version", "override sdk version")
	flag.Parse()
	if options.version.Epoch != nil && *options.version.Epoch == 0 {
		options.version.Epoch = nil
	}
	if options.version.Rel != nil && *options.version.Rel == "" {
		options.version.Rel = nil
	}
}

func findPackage(pkgs []*repomd.PrimaryPackage, entry rpm.Entry) *repomd.PrimaryPackage {
	pkgsSeq := utils.Filter(pkgs, func(pkg *repomd.PrimaryPackage) bool {
		return entry.Match(pkg)
	})
	if len(pkgsSeq) < 1 {
		slog.Warn("could not find matching package", "package", entry.String())
		return nil
	}
	maxPkg := slices.MaxFunc(pkgsSeq, func(a, b *repomd.PrimaryPackage) int {
		return rpm.Compare(a.Version, b.Version)
	})
	return maxPkg
}

func run(ctx context.Context) error {
	parseFlags()
	logOptions := &slog.HandlerOptions{}
	if options.verbose {
		logOptions.Level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, logOptions)))
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable: %w", err)
	}
	if strings.Contains(exe, "go-build") {
		// Assume this is `go run`
		os.Chdir("..")
	}
	fs, err := httpfs.NewHttpFs(repository)
	if err != nil {
		return fmt.Errorf("error creating fs: %w", err)
	}
	primary, err := repomd.ParsePrimary(fs)
	if err != nil {
		return fmt.Errorf("error parsing repo: %w", err)
	}
	initalPkg := findPackage(primary.Packages, rpm.Entry{
		Name: initialPackage,
	})
	if initalPkg == nil {
		return fmt.Errorf("failed to get initial package %s", initialPackage)
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
