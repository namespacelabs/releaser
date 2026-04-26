// Copyright 2022 Namespace Labs Inc; All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Package tigris contains helpers to interact with Tigris' S3-compatible API.
package tigris

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// DefaultEndpoint is the public Tigris S3 endpoint.
const DefaultEndpoint = "https://t3.storage.dev"

// NewClient builds an S3 client configured to talk to the given Tigris endpoint.
func NewClient(ctx context.Context, endpoint string) (*s3.Client, error) {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID {
			return aws.Endpoint{
				PartitionID:       "aws",
				URL:               endpoint,
				SigningRegion:     "auto",
				HostnameImmutable: true,
			}, nil
		}

		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("auto"),
		config.WithEndpointResolverWithOptions(resolver),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.UsePathStyle = true
	}), nil
}

// PutFile uploads the contents of filename to s3://bucket/key.
func PutFile(ctx context.Context, client *s3.Client, bucket, key, filename, contentType string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open %s: %w", filename, err)
	}
	defer file.Close()

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("upload %s to s3://%s/%s: %w", filename, bucket, key, err)
	}

	return nil
}

// PutBytes uploads body to s3://bucket/key.
func PutBytes(ctx context.Context, client *s3.Client, bucket, key string, body []byte, contentType string) error {
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("upload s3://%s/%s: %w", bucket, key, err)
	}

	return nil
}
