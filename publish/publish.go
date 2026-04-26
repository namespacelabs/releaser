// Copyright 2022 Namespace Labs Inc; All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Package publish wires together manifest generation and Tigris uploads to
// publish a release.
package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"namespacelabs.dev/releaser/manifest"
	"namespacelabs.dev/releaser/tigris"
)

// ReleaseOptions configures a single release publish.
type ReleaseOptions struct {
	// Bucket is the destination Tigris bucket.
	Bucket string
	// Endpoint is the Tigris S3 endpoint. Defaults to tigris.DefaultEndpoint.
	Endpoint string
	// DistDir is the local GoReleaser dist directory containing the tarballs
	// and checksums.txt.
	DistDir string
	// Tag is the release tag, e.g. "v0.0.123".
	Tag string
	// KeyPrefix is the bucket key prefix under which artifacts are uploaded,
	// e.g. "foundation" or "devbox". Resulting keys look like:
	//   <KeyPrefix>/releases/<Tag>/<file>
	//   <KeyPrefix>/releases/latest/<tool>.json
	KeyPrefix string
	// Tools is the list of tool names to include in the release. Each tool
	// gets its own manifest under <KeyPrefix>/releases/<Tag>/<tool>.json and
	// <KeyPrefix>/releases/latest/<tool>.json.
	Tools []string
	// PublishedAt overrides the published_at timestamp written to manifests.
	// Defaults to time.Now().UTC().
	PublishedAt time.Time
	// OSes / Arches optionally restrict the artifacts considered. See
	// manifest.BuildOptions.
	OSes   []string
	Arches []string
}

// Release uploads all release artifacts and manifests for the given options.
func Release(ctx context.Context, opts ReleaseOptions) error {
	if opts.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if opts.Tag == "" {
		return fmt.Errorf("tag is required")
	}
	if opts.KeyPrefix == "" {
		return fmt.Errorf("key prefix is required")
	}
	if len(opts.Tools) == 0 {
		return fmt.Errorf("at least one tool is required")
	}
	if opts.DistDir == "" {
		opts.DistDir = "dist"
	}
	if opts.PublishedAt.IsZero() {
		opts.PublishedAt = time.Now().UTC()
	}

	version := strings.TrimPrefix(opts.Tag, "v")
	checksumsPath := filepath.Join(opts.DistDir, "checksums.txt")
	checksums, err := manifest.ReadChecksums(checksumsPath)
	if err != nil {
		return err
	}

	manifests, tarballs, err := manifest.Build(opts.DistDir, version, opts.Tag, opts.PublishedAt, checksums, manifest.BuildOptions{
		Tools:  opts.Tools,
		OSes:   opts.OSes,
		Arches: opts.Arches,
	})
	if err != nil {
		return err
	}

	client, err := tigris.NewClient(ctx, opts.Endpoint)
	if err != nil {
		return err
	}

	return uploadRelease(ctx, client, opts, manifests, tarballs, checksumsPath)
}

func uploadRelease(ctx context.Context, client *s3.Client, opts ReleaseOptions, manifests map[string]manifest.Manifest, tarballs []string, checksumsPath string) error {
	for _, file := range tarballs {
		key := path.Join(opts.KeyPrefix, "releases", opts.Tag, filepath.Base(file))
		if err := tigris.PutFile(ctx, client, opts.Bucket, key, file, "application/gzip"); err != nil {
			return err
		}
	}

	checksumsKey := path.Join(opts.KeyPrefix, "releases", opts.Tag, "checksums.txt")
	if err := tigris.PutFile(ctx, client, opts.Bucket, checksumsKey, checksumsPath, "text/plain; charset=utf-8"); err != nil {
		return err
	}

	for _, tool := range opts.Tools {
		payload, err := json.MarshalIndent(manifests[tool], "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s manifest: %w", tool, err)
		}

		versionKey := path.Join(opts.KeyPrefix, "releases", opts.Tag, tool+".json")
		latestKey := path.Join(opts.KeyPrefix, "releases", "latest", tool+".json")
		if err := tigris.PutBytes(ctx, client, opts.Bucket, versionKey, payload, "application/json"); err != nil {
			return err
		}
		if err := tigris.PutBytes(ctx, client, opts.Bucket, latestKey, payload, "application/json"); err != nil {
			return err
		}
	}

	return nil
}

// InstallerOptions configures an installer-script publish.
type InstallerOptions struct {
	Bucket    string
	Endpoint  string
	KeyPrefix string
	// Installers maps the source path of an installer script to the bucket key
	// it should be uploaded to (relative to KeyPrefix). Example:
	//   "install/install.sh" -> "install/install.sh"
	Installers map[string]string
}

// Installers uploads each installer script to <KeyPrefix>/<dest>.
func Installers(ctx context.Context, opts InstallerOptions) error {
	if opts.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if opts.KeyPrefix == "" {
		return fmt.Errorf("key prefix is required")
	}
	if len(opts.Installers) == 0 {
		return fmt.Errorf("at least one installer is required")
	}

	client, err := tigris.NewClient(ctx, opts.Endpoint)
	if err != nil {
		return err
	}

	for src, dest := range opts.Installers {
		key := path.Join(opts.KeyPrefix, dest)
		if err := tigris.PutFile(ctx, client, opts.Bucket, key, src, "text/plain; charset=utf-8"); err != nil {
			return err
		}
	}

	return nil
}
