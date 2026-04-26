// Copyright 2022 Namespace Labs Inc; All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Command release publishes a parameterized release manifest and the
// associated tarballs to a Tigris bucket.
//
// Usage:
//
//	release release  --bucket <bucket> --tag v0.0.123 --tools ns,nsc \
//	    [--key-prefix foundation] [--dist-dir dist] [--endpoint https://t3.storage.dev]
//
//	release installers --bucket <bucket> --key-prefix foundation \
//	    --installer install/install.sh=install/install.sh \
//	    --installer install/install_nsc.sh=install/install_nsc.sh
//
// The "release" subcommand uploads all <tool>_<version>_<os>_<arch>.tar.gz files
// from the dist directory along with one manifest JSON per tool. The same set
// of flags works for ns, nsc, devbox or any other GoReleaser-style tool by
// passing the desired tool list via --tools.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"namespacelabs.dev/releaser/publish"
	"namespacelabs.dev/releaser/tigris"
)

func main() {
	ctx := context.Background()

	if len(os.Args) < 2 {
		fatalf("usage: %s <release|installers> [flags]", os.Args[0])
	}

	switch os.Args[1] {
	case "release":
		if err := runRelease(ctx, os.Args[2:]); err != nil {
			fatalf("release upload failed: %v", err)
		}
	case "installers":
		if err := runInstallers(ctx, os.Args[2:]); err != nil {
			fatalf("installer upload failed: %v", err)
		}
	default:
		fatalf("unknown command %q", os.Args[1])
	}
}

func runRelease(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("release", flag.ExitOnError)
	bucket := fs.String("bucket", "", "Tigris bucket name (required)")
	endpoint := fs.String("endpoint", tigris.DefaultEndpoint, "Tigris S3 endpoint")
	distDir := fs.String("dist-dir", "dist", "GoReleaser dist directory")
	tag := fs.String("tag", "", "Release tag, for example v0.0.123 (required)")
	keyPrefix := fs.String("key-prefix", "foundation", "Bucket key prefix (e.g. foundation, devbox)")
	tools := fs.String("tools", "ns,nsc", "Comma-separated list of tools to publish (e.g. ns,nsc or devbox)")
	oses := fs.String("os", "darwin,linux", "Comma-separated list of operating systems to accept")
	arches := fs.String("arch", "amd64,arm64", "Comma-separated list of architectures to accept")
	fs.Parse(args)

	return publish.Release(ctx, publish.ReleaseOptions{
		Bucket:    *bucket,
		Endpoint:  *endpoint,
		DistDir:   *distDir,
		Tag:       *tag,
		KeyPrefix: *keyPrefix,
		Tools:     splitList(*tools),
		OSes:      splitList(*oses),
		Arches:    splitList(*arches),
	})
}

type installerFlag map[string]string

func (i installerFlag) String() string {
	parts := make([]string, 0, len(i))
	for k, v := range i {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (i installerFlag) Set(value string) error {
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			return fmt.Errorf("expected src=dest, got %q", entry)
		}
		i[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return nil
}

func runInstallers(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("installers", flag.ExitOnError)
	bucket := fs.String("bucket", "", "Tigris bucket name (required)")
	endpoint := fs.String("endpoint", tigris.DefaultEndpoint, "Tigris S3 endpoint")
	keyPrefix := fs.String("key-prefix", "foundation", "Bucket key prefix (e.g. foundation, devbox)")
	installers := installerFlag{}
	fs.Var(installers, "installer", "Installer mapping in the form src=dest. May be repeated.")
	fs.Parse(args)

	return publish.Installers(ctx, publish.InstallerOptions{
		Bucket:     *bucket,
		Endpoint:   *endpoint,
		KeyPrefix:  *keyPrefix,
		Installers: installers,
	})
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
