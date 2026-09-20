// Copyright 2022 Namespace Labs Inc; All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildRelease(t *testing.T) {
	for _, test := range []struct {
		name, sourceDist, failure string
		packagesOnly, filtered    bool
		disableBinaries           bool
	}{
		{name: "binaries disabled by default", sourceDist: "dist", disableBinaries: true},
		{name: "disabled ignores malformed metadata", failure: "read GoReleaser artifacts", disableBinaries: true},
		{name: "packages only", packagesOnly: true},
		{name: "raw binaries", sourceDist: "dist"},
		{name: "nested dist", sourceDist: "out/releases"},
		{name: "platform filter", sourceDist: "dist", filtered: true},
		{name: "missing checksum", sourceDist: "dist", failure: "missing checksum"},
		{name: "missing binary", sourceDist: "dist", failure: "read binary"},
		{name: "wrong version", sourceDist: "dist", failure: "does not match expected"},
		{name: "malformed metadata", failure: "read GoReleaser artifacts"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dist := filepath.Join(t.TempDir(), "downloaded", "dist")
			write := func(name, contents string) {
				t.Helper()
				file := filepath.Join(dist, name)
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			publishedAt := time.Unix(1714000000, 0).UTC()
			opts := ReleaseOptions{DistDir: dist, Tag: "v1.2.3", Tools: []string{"ns", "nsc"}, PublishedAt: publishedAt}
			if !test.disableBinaries {
				opts.PublishBinaries = true
			}
			if test.filtered {
				opts.Tools = []string{"ns"}
				opts.OSes = []string{"darwin"}
				opts.Arches = []string{"arm64"}
			}
			var expected []artifactUpload
			checksums := ""
			metadata := []map[string]any{{"name": "checksums.txt", "path": test.sourceDist + "/checksums.txt", "type": "Checksum"}}
			for _, target := range []struct{ tool, os, arch, ext string }{
				{"ns", "darwin", "arm64", ".tar.gz"},
				{"ns", "linux", "amd64", ".tar.gz"},
				{"nsc", "windows", "arm64", ".zip"},
			} {
				base := fmt.Sprintf("%s_1.2.3_%s_%s", target.tool, target.os, target.arch)
				binary := target.tool
				name := base
				if target.os == "windows" {
					binary += ".exe"
					name += ".exe"
				}
				rel := target.tool + "_" + target.os + "_" + target.arch + "/" + binary
				if test.failure != "read binary" {
					write(rel, "raw "+target.os)
				}
				write(base+target.ext, "package "+target.os)
				checksums += "archive-" + target.os + "  " + base + target.ext + "\n"
				if test.failure != "missing checksum" {
					checksums += "raw-" + target.os + "  " + name + "\n"
				}
				metadataName := name
				if test.failure == "does not match expected" {
					metadataName = strings.Replace(name, "1.2.3", "9.9.9", 1)
				}
				source := test.sourceDist + "/" + rel
				if target.os == "darwin" {
					source = filepath.Join(dist, rel)
				}
				metadata = append(metadata, map[string]any{
					"name": metadataName, "path": source, "type": "Binary", "goos": target.os, "goarch": target.arch,
					"extra": map[string]string{"Binary": binary, "Format": "binary"},
				})
				if test.filtered && target.os != "darwin" {
					continue
				}
				if !test.packagesOnly && !test.disableBinaries {
					expected = append(expected, artifactUpload{Filename: name, SourcePath: filepath.Join(dist, rel), ContentType: "application/octet-stream"})
				}
				contentType := "application/gzip"
				if target.os == "windows" {
					contentType = "application/zip"
				}
				expected = append(expected, artifactUpload{Filename: base + target.ext, SourcePath: filepath.Join(dist, base+target.ext), ContentType: contentType})
			}
			// These entries must not be treated as selected uploadable binaries.
			metadata = append(metadata,
				map[string]any{"type": "Binary", "goos": "darwin", "goarch": "arm64", "extra": map[string]string{"Binary": "ns"}},
				map[string]any{"type": "Binary", "goos": "linux", "goarch": "amd64", "extra": map[string]string{"Binary": "other", "Format": "binary"}},
				map[string]any{"type": "Binary", "goos": "linux", "goarch": "386", "extra": map[string]string{"Binary": "ns", "Format": "binary"}},
			)
			if !test.packagesOnly {
				data, err := json.Marshal(metadata)
				if err != nil {
					t.Fatal(err)
				}
				if test.failure == "read GoReleaser artifacts" {
					data = []byte("[")
				}
				write("artifacts.json", string(data))
			}
			write("checksums.txt", checksums)

			manifests, uploads, checksumPath, err := buildRelease(opts)
			if test.failure != "" && !test.disableBinaries {
				if err == nil || !strings.Contains(err.Error(), test.failure) {
					t.Fatalf("error = %v, want %q", err, test.failure)
				}
				if len(uploads) != 0 {
					t.Fatalf("failed release returned uploads: %+v", uploads)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(uploads, expected) {
				t.Fatalf("uploads = %+v, want %+v", uploads, expected)
			}
			for _, tool := range opts.Tools {
				m := manifests[tool]
				wantCount := 1
				if tool == "ns" && !test.filtered {
					wantCount = 2
				}
				if len(m.Artifacts) != wantCount || m.Version != opts.Tag || !m.PublishedAt.Equal(publishedAt) {
					t.Fatalf("unexpected manifest: %+v", m)
				}
				for _, a := range m.Artifacts {
					if !strings.HasSuffix(a.Filename, ".tar.gz") && !strings.HasSuffix(a.Filename, ".zip") {
						t.Fatalf("raw binary in installer manifest: %+v", a)
					}
					if a.SHA256 != "archive-"+strings.ToLower(a.OS) {
						t.Fatalf("wrong package checksum: %+v", a)
					}
				}
			}
			data, err := os.ReadFile(checksumPath)
			if err != nil || string(data) != checksums {
				t.Fatalf("checksums changed: %q, %v", data, err)
			}
		})
	}
}
