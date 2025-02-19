package main

import (
	"context"
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
	repository = "https://packages.microsoft.com/opensuse/15/prod/"
	repoMeta   = "prod.repo"
)

var packages struct {
	sync.Mutex
	mapping map[string]*packageWriter
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
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
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
