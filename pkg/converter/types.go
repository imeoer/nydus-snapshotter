/*
 * Copyright (c) 2022. Nydus Developers. All rights reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package converter

import (
	"context"
	"time"

	"github.com/containerd/containerd/content"
	"github.com/opencontainers/go-digest"
)

type Layer struct {
	// Digest represents the hash of whole tar blob.
	Digest digest.Digest
	// Digest represents the original OCI tar(.gz) blob.
	OriginalDigest *digest.Digest
	// ReaderAt holds the reader of whole tar blob.
	ReaderAt content.ReaderAt
}

// Backend uploads blobs generated by nydus-image builder to a backend storage such as:
// - oss: A object storage backend, which uses its SDK to upload blob file.
type Backend interface {
	// Push pushes specified blob file to remote storage backend.
	Push(ctx context.Context, ra content.ReaderAt, blobDigest digest.Digest) error
	// Check checks whether a blob exists in remote storage backend,
	// blob exists -> return (blobPath, nil)
	// blob not exists -> return ("", err)
	Check(blobDigest digest.Digest) (string, error)
	// Type returns backend type name.
	Type() string
}

type PackOption struct {
	// WorkDir is used as the work directory during layer pack.
	WorkDir string
	// BuilderPath holds the path of `nydus-image` binary tool.
	BuilderPath string
	// FsVersion specifies nydus RAFS format version, possible
	// values: `5`, `6` (EROFS-compatible), default is `5`.
	FsVersion string
	// ChunkDictPath holds the bootstrap path of chunk dict image.
	ChunkDictPath string
	// PrefetchPatterns holds file path pattern list want to prefetch.
	PrefetchPatterns string
	// Compressor specifies nydus blob compression algorithm.
	Compressor string
	// OCIRef enables to convert OCI tar(.gz) blob to nydus referenced blob.
	OCIRef bool
	// Backend uploads blobs generated by nydus-image builder to a backend storage.
	Backend Backend
	// Timeout cancels execution once exceed the specified time.
	Timeout *time.Duration
}

type MergeOption struct {
	// WorkDir is used as the work directory during layer merge.
	WorkDir string
	// BuilderPath holds the path of `nydus-image` binary tool.
	BuilderPath string
	// FsVersion specifies nydus RAFS format version, possible
	// values: `5`, `6` (EROFS-compatible), default is `5`.
	FsVersion string
	// ChunkDictPath holds the bootstrap path of chunk dict image.
	ChunkDictPath string
	// PrefetchPatterns holds file path pattern list want to prefetch.
	PrefetchPatterns string
	// WithTar puts bootstrap into a tar stream (no gzip).
	WithTar bool
	// Backend uploads blobs generated by nydus-image builder to a backend storage.
	Backend Backend
	// Timeout cancels execution once exceed the specified time.
	Timeout *time.Duration
}

type UnpackOption struct {
	// WorkDir is used as the work directory during layer unpack.
	WorkDir string
	// BuilderPath holds the path of `nydus-image` binary tool.
	BuilderPath string
	// Timeout cancels execution once exceed the specified time.
	Timeout *time.Duration
}
