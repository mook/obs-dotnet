// Package versions determines the package versions to download
package versions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

const (
	releaseURL = "https://github.com/dotnet/core/raw/refs/heads/main/release-notes/%s/%s/release.json"
)

var (
	channelRegexp = regexp.MustCompile(`^\d+\.\d+`)
)

type versions struct {
	Releases []struct {
		SDK struct {
			Version string `json:"version"`
		} `json:"sdk"`
	} `json:"releases"`
}

// Fetch the SDK version for the given runtime version.
func FetchSDKVersion(ctx context.Context, version string) (string, error) {
	channel := channelRegexp.FindString(version)
	if channel == "" {
		return "", fmt.Errorf("failed to find channel version from version %q", version)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf(releaseURL, channel, version),
		http.NoBody)
	if err != nil {
		return "", fmt.Errorf("failed to create release manifest request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch release manifest: %w", err)
	}
	defer resp.Body.Close()
	var versions versions
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return "", fmt.Errorf("failed to unmarshal release manifest: %w", err)
	}
	for _, release := range versions.Releases {
		if release.SDK.Version != "" {
			return release.SDK.Version, nil
		}
	}
	return "", fmt.Errorf("failed to find SDK version for %q", version)
}
