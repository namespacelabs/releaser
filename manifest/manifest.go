// Copyright 2022 Namespace Labs Inc; All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Package manifest defines the release manifest format published to Tigris and
// helpers to build manifests from a GoReleaser-style dist directory.
//
// The types defined here are intentionally exported so that other repositories
// (for example namespacelabs.dev/internal/service/versions) can decode the same
// manifest format without duplicating the schema.
package manifest

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Manifest is the JSON payload that describes a single tool release.
type Manifest struct {
	Tool        string     `json:"tool"`
	Version     string     `json:"version"`
	PublishedAt time.Time  `json:"published_at"`
	Artifacts   []Artifact `json:"artifacts"`
}

// Artifact describes a single uploaded tarball for a Manifest.
type Artifact struct {
	Filename string `json:"filename"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	SHA256   string `json:"sha256"`
}

// BuildOptions configure how Build derives manifests from a dist directory.
type BuildOptions struct {
	// Tools lists the tool names whose artifacts must be present in the dist
	// directory. The names must match the prefix used by GoReleaser, e.g. "ns",
	// "nsc", "devbox".
	Tools []string

	// OSes restricts the set of operating systems that are accepted. Defaults
	// to {"darwin", "linux"} when empty.
	OSes []string

	// Arches restricts the set of architectures that are accepted. Defaults to
	// {"amd64", "arm64"} when empty.
	Arches []string
}

var (
	defaultOSes   = []string{"darwin", "linux"}
	defaultArches = []string{"amd64", "arm64"}
)

// Build inspects distDir for tarballs of the form <tool>_<version>_<os>_<arch>.tar.gz
// and produces a Manifest per tool. It returns the manifests keyed by tool name
// and the absolute paths of the matched tarballs.
func Build(distDir, version, tag string, publishedAt time.Time, checksums map[string]string, opts BuildOptions) (map[string]Manifest, []string, error) {
	if len(opts.Tools) == 0 {
		return nil, nil, fmt.Errorf("at least one tool is required")
	}

	oses := opts.OSes
	if len(oses) == 0 {
		oses = defaultOSes
	}
	arches := opts.Arches
	if len(arches) == 0 {
		arches = defaultArches
	}

	re, err := compileTarballRE(opts.Tools, oses, arches)
	if err != nil {
		return nil, nil, err
	}

	entries, err := os.ReadDir(distDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read dist dir: %w", err)
	}

	manifests := make(map[string]Manifest, len(opts.Tools))
	for _, tool := range opts.Tools {
		manifests[tool] = Manifest{Tool: tool, Version: tag, PublishedAt: publishedAt}
	}

	var tarballs []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		match := re.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}

		tool, artifactVersion, osName, arch := match[1], match[2], strings.ToUpper(match[3]), strings.ToUpper(match[4])
		if artifactVersion != version {
			return nil, nil, fmt.Errorf("unexpected version in %s: got %s want %s", entry.Name(), artifactVersion, version)
		}

		sha256, ok := checksums[entry.Name()]
		if !ok {
			return nil, nil, fmt.Errorf("missing checksum for %s", entry.Name())
		}

		artifact := Artifact{
			Filename: entry.Name(),
			OS:       osName,
			Arch:     arch,
			SHA256:   sha256,
		}

		m := manifests[tool]
		m.Artifacts = append(m.Artifacts, artifact)
		manifests[tool] = m
		tarballs = append(tarballs, fmt.Sprintf("%s/%s", strings.TrimRight(distDir, "/"), entry.Name()))
	}

	for _, tool := range opts.Tools {
		m := manifests[tool]
		if len(m.Artifacts) == 0 {
			return nil, nil, fmt.Errorf("no artifacts found for %s", tool)
		}

		sort.Slice(m.Artifacts, func(i, j int) bool {
			return m.Artifacts[i].Filename < m.Artifacts[j].Filename
		})
		manifests[tool] = m
	}

	sort.Strings(tarballs)
	return manifests, tarballs, nil
}

func compileTarballRE(tools, oses, arches []string) (*regexp.Regexp, error) {
	for _, tool := range tools {
		if tool == "" {
			return nil, fmt.Errorf("tool name must not be empty")
		}
	}

	pattern := fmt.Sprintf(`^(%s)_([^_]+)_(%s)_(%s)\.tar\.gz$`,
		strings.Join(escapeAll(tools), "|"),
		strings.Join(escapeAll(oses), "|"),
		strings.Join(escapeAll(arches), "|"),
	)
	return regexp.Compile(pattern)
}

func escapeAll(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = regexp.QuoteMeta(v)
	}
	return out
}

// ReadChecksums parses a checksums.txt file as produced by GoReleaser and
// returns a map from filename to SHA-256 hex digest.
func ReadChecksums(checksumsPath string) (map[string]string, error) {
	file, err := os.Open(checksumsPath)
	if err != nil {
		return nil, fmt.Errorf("open checksums.txt: %w", err)
	}
	defer file.Close()

	checksums := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, fmt.Errorf("unexpected checksum line %q", line)
		}

		checksums[parts[1]] = parts[0]
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksums.txt: %w", err)
	}

	return checksums, nil
}
