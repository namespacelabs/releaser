// Copyright 2022 Namespace Labs Inc; All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"namespacelabs.dev/releaser/manifest"
)

func binaryUploads(distDir, version string, checksums map[string]string, manifests map[string]manifest.Manifest) ([]artifactUpload, error) {
	data, err := os.ReadFile(filepath.Join(distDir, "artifacts.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var artifacts []struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		Type   string `json:"type"`
		Goos   string `json:"goos"`
		Goarch string `json:"goarch"`
		Extra  struct {
			Binary string
			Format string
		} `json:"extra"`
	}
	if err := json.Unmarshal(data, &artifacts); err != nil {
		return nil, fmt.Errorf("read GoReleaser artifacts: %w", err)
	}

	// Metadata paths are relative to GoReleaser's workdir, which may differ from ours.
	sourceDist := filepath.Base(distDir)
	for _, artifact := range artifacts {
		if artifact.Type == "Checksum" && artifact.Name == "checksums.txt" {
			sourceDist = filepath.Dir(filepath.FromSlash(artifact.Path))
		}
	}

	var uploads []artifactUpload
	for _, artifact := range artifacts {
		// Build artifacts also have type Binary; only the binary archive format is uploadable.
		if artifact.Type != "Binary" || artifact.Extra.Format != "binary" {
			continue
		}
		tool := artifact.Extra.Binary
		if artifact.Goos == "windows" {
			tool = strings.TrimSuffix(tool, ".exe")
		}
		// Binaries supplement the selected packages without changing installer manifests.
		if !slices.ContainsFunc(manifests[tool].Artifacts, func(a manifest.Artifact) bool {
			return a.OS == strings.ToUpper(artifact.Goos) && a.Arch == strings.ToUpper(artifact.Goarch)
		}) {
			continue
		}
		name := fmt.Sprintf("%s_%s_%s_%s", tool, version, artifact.Goos, artifact.Goarch)
		if artifact.Goos == "windows" {
			name += ".exe"
		}
		if artifact.Name != name {
			return nil, fmt.Errorf("binary filename %q does not match expected %q", artifact.Name, name)
		}
		if checksums[name] == "" {
			return nil, fmt.Errorf("missing checksum for binary %s", name)
		}

		source := filepath.FromSlash(artifact.Path)
		if !filepath.IsAbs(source) {
			rel, err := filepath.Rel(sourceDist, source)
			if err != nil {
				return nil, fmt.Errorf("resolve binary %s: %w", name, err)
			}
			source = filepath.Join(distDir, rel)
		}
		if _, err := os.Stat(source); err != nil {
			return nil, fmt.Errorf("read binary %s: %w", name, err)
		}
		uploads = append(uploads, artifactUpload{
			Filename:    name,
			SourcePath:  source,
			ContentType: "application/octet-stream",
		})
	}
	return uploads, nil
}
