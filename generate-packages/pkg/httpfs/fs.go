package httpfs

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"time"
)

func NewHttpFs(baseURL string) (fs.FS, error) {
	parsedURL, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return nil, err
	}
	return &HttpFs{base: parsedURL}, nil
}

type HttpFs struct {
	base *url.URL
}

// Open implements fs.FS.
func (h *HttpFs) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	fileURL := h.base.JoinPath(name).String()
	slog.Debug("open httpfs", "url", fileURL)
	resp, err := http.Get(fileURL)
	if err != nil {
		slog.Error("failed to make GET request", "url", fileURL)
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  err,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, &fs.PathError{
				Op:   "open",
				Path: name,
				Err:  fs.ErrNotExist,
			}
		}
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  err,
		}
	}
	if resp.Body == nil {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fmt.Errorf("response had no body"),
		}
	}
	return &HttpFile{resp: resp, ReadCloser: resp.Body}, nil
}

type HttpFile struct {
	io.ReadCloser
	resp *http.Response
}

// Stat implements fs.File.
func (h *HttpFile) Stat() (fs.FileInfo, error) {
	return &HttpFileInfo{headResponse: h.resp}, nil
}

type HttpFileInfo struct {
	headResponse *http.Response
}

// IsDir implements fs.FileInfo.
func (h *HttpFileInfo) IsDir() bool {
	return false
}

// ModTime implements fs.FileInfo.
func (h *HttpFileInfo) ModTime() time.Time {
	for _, headerKey := range []string{"Last-Modified", "Date"} {
		value := h.headResponse.Header.Get(headerKey)
		if value != "" {
			t, err := time.Parse(time.RFC1123, value)
			if err == nil {
				return t
			}
		}
	}
	return time.Unix(0, 0).UTC()
}

// Mode implements fs.FileInfo.
func (h *HttpFileInfo) Mode() fs.FileMode {
	// HTTP doesn't really have file modes.
	return 0o644
}

// Name implements fs.FileInfo.
func (h *HttpFileInfo) Name() string {
	disposition := h.headResponse.Header.Get("Content-Disposition")
	if disposition != "" {
		_, params, err := mime.ParseMediaType(disposition)
		if err == nil && params["filename"] != "" {
			return params["filename"]
		}
	}
	return path.Base(h.headResponse.Request.URL.Path)
}

// Size implements fs.FileInfo.
func (h *HttpFileInfo) Size() int64 {
	return h.headResponse.ContentLength
}

// Sys implements fs.FileInfo.
func (h *HttpFileInfo) Sys() any {
	return nil
}
