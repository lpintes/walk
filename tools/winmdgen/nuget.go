// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	nugetPackage = "Microsoft.Windows.SDK.Win32Metadata"
	winmdFile    = "Windows.Win32.winmd"
)

// fetchWinmd returns the contents of Windows.Win32.winmd from the given
// version of the NuGet package. The package is downloaded once into the
// user cache directory and its SHA-256 hash is checked on every use.
func fetchWinmd(version, sum string) ([]byte, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	id := strings.ToLower(nugetPackage)
	ver := strings.ToLower(version)
	path := filepath.Join(cache, "walk-winmdgen", id+"."+ver+".nupkg")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		url := fmt.Sprintf("https://api.nuget.org/v3-flatcontainer/%s/%s/%s.%s.nupkg", id, ver, id, ver)
		fmt.Fprintf(os.Stderr, "winmdgen: downloading %s\n", url)
		data, err = download(url)
		if err != nil {
			return nil, err
		}
		if err := checkSum(data, sum); err != nil {
			return nil, fmt.Errorf("%s: %w", url, err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if err := checkSum(data, sum); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name != winmdFile {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("%s: %s not found", path, winmdFile)
}

func download(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func checkSum(data []byte, want string) error {
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("SHA-256 mismatch: got %x, want %s", got, want)
	}
	return nil
}
