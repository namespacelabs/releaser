// Copyright 2022 Namespace Labs Inc; All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package manifest

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuild(t *testing.T) {
	tempDir := t.TempDir()
	tag := "v1.2.3"
	version := "1.2.3"
	publishedAt := time.Unix(1714000000, 0).UTC()

	files := map[string]string{
		"ns_1.2.3_darwin_arm64.tar.gz":  "",
		"ns_1.2.3_linux_amd64.tar.gz":   "",
		"ns_1.2.3_windows_amd64.zip":    "",
		"nsc_1.2.3_darwin_arm64.tar.gz": "",
		"nsc_1.2.3_linux_amd64.tar.gz":  "",
		"nsc_1.2.3_windows_arm64.zip":   "",
		"checksums.txt": "aaa ns_1.2.3_darwin_arm64.tar.gz\n" +
			"bbb ns_1.2.3_linux_amd64.tar.gz\n" +
			"eee ns_1.2.3_windows_amd64.zip\n" +
			"ccc nsc_1.2.3_darwin_arm64.tar.gz\n" +
			"ddd nsc_1.2.3_linux_amd64.tar.gz\n" +
			"fff nsc_1.2.3_windows_arm64.zip\n",
	}

	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(tempDir, name), []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	checksums, err := ReadChecksums(filepath.Join(tempDir, "checksums.txt"))
	if err != nil {
		t.Fatalf("ReadChecksums: %v", err)
	}

	manifests, tarballs, err := Build(tempDir, version, tag, publishedAt, checksums, BuildOptions{
		Tools: []string{"ns", "nsc"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(tarballs) != 6 {
		t.Fatalf("got %d archives, want 6", len(tarballs))
	}

	if got := manifests["ns"].Version; got != tag {
		t.Fatalf("ns version = %q, want %q", got, tag)
	}

	if got := manifests["ns"].PublishedAt; !got.Equal(publishedAt) {
		t.Fatalf("published_at = %v, want %v", got, publishedAt)
	}

	if got := len(manifests["ns"].Artifacts); got != 3 {
		t.Fatalf("ns artifacts = %d, want 3", got)
	}

	if got := manifests["nsc"].Artifacts[1].SHA256; got != "ddd" {
		t.Fatalf("nsc linux checksum = %q, want %q", got, "ddd")
	}

	// The windows .zip artifact must be discovered and recorded.
	var win *Artifact
	for i := range manifests["nsc"].Artifacts {
		if a := &manifests["nsc"].Artifacts[i]; a.OS == "WINDOWS" {
			win = a
			break
		}
	}
	if win == nil {
		t.Fatalf("nsc windows artifact not found in %+v", manifests["nsc"].Artifacts)
	}
	if win.Filename != "nsc_1.2.3_windows_arm64.zip" {
		t.Fatalf("nsc windows filename = %q, want %q", win.Filename, "nsc_1.2.3_windows_arm64.zip")
	}
	if win.Arch != "ARM64" {
		t.Fatalf("nsc windows arch = %q, want %q", win.Arch, "ARM64")
	}
	if win.SHA256 != "fff" {
		t.Fatalf("nsc windows checksum = %q, want %q", win.SHA256, "fff")
	}
}

func TestBuildCustomTool(t *testing.T) {
	tempDir := t.TempDir()
	tag := "v0.1.0"
	version := "0.1.0"
	publishedAt := time.Unix(1714000000, 0).UTC()

	files := map[string]string{
		"devbox_0.1.0_darwin_arm64.tar.gz": "",
		"devbox_0.1.0_linux_amd64.tar.gz":  "",
		"checksums.txt": "111 devbox_0.1.0_darwin_arm64.tar.gz\n" +
			"222 devbox_0.1.0_linux_amd64.tar.gz\n",
	}

	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(tempDir, name), []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	checksums, err := ReadChecksums(filepath.Join(tempDir, "checksums.txt"))
	if err != nil {
		t.Fatalf("ReadChecksums: %v", err)
	}

	manifests, tarballs, err := Build(tempDir, version, tag, publishedAt, checksums, BuildOptions{
		Tools: []string{"devbox"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(tarballs) != 2 {
		t.Fatalf("got %d tarballs, want 2", len(tarballs))
	}

	if got := len(manifests["devbox"].Artifacts); got != 2 {
		t.Fatalf("devbox artifacts = %d, want 2", got)
	}
}
