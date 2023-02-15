/*
 * Copyright (c) 2022. Nydus Developers. All rights reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package converter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/containerd/containerd/content"
	"github.com/containerd/nydus-snapshotter/pkg/converter/tool"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type Compressor = uint32

const (
	CompressorNone Compressor = 0x0001
	CompressorZstd Compressor = 0x0002
)

var (
	ErrNotFound = errors.New("data not found")
)

type Layer struct {
	// Digest represents the hash of whole tar blob.
	Digest digest.Digest
	// Digest represents the original OCI tar(.gz) blob.
	OriginalDigest *digest.Digest
	// ReaderAt holds the reader of whole tar blob.
	ReaderAt content.ReaderAt
}

// Backend uploads blobs generated by nydus-image builder to a backend storage.
type Backend interface {
	// Push pushes specified blob file to remote storage backend.
	Push(ctx context.Context, cs content.Store, desc ocispec.Descriptor) error
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
	// values: `5`, `6` (EROFS-compatible), default is `6`.
	FsVersion string
	// ChunkDictPath holds the bootstrap path of chunk dict image.
	ChunkDictPath string
	// PrefetchPatterns holds file path pattern list want to prefetch.
	PrefetchPatterns string
	// Compressor specifies nydus blob compression algorithm.
	Compressor string
	// OCIRef enables converting OCI tar(.gz) blob to nydus referenced blob.
	OCIRef bool
	// AlignedChunk aligns uncompressed data chunks to 4K, only for RAFS V5.
	AlignedChunk bool
	// ChunkSize sets the size of data chunks, must be power of two and between 0x1000-0x1000000.
	ChunkSize string
	// Backend uploads blobs generated by nydus-image builder to a backend storage.
	Backend Backend
	// Timeout cancels execution once exceed the specified time.
	Timeout *time.Duration

	// Features keeps a feature list supported by newer version of builder,
	// It is detected automatically, so don't export it.
	features tool.Features
}

type MergeOption struct {
	// WorkDir is used as the work directory during layer merge.
	WorkDir string
	// BuilderPath holds the path of `nydus-image` binary tool.
	BuilderPath string
	// FsVersion specifies nydus RAFS format version, possible
	// values: `5`, `6` (EROFS-compatible), default is `6`.
	FsVersion string
	// ChunkDictPath holds the bootstrap path of chunk dict image.
	ChunkDictPath string
	// PrefetchPatterns holds file path pattern list want to prefetch.
	PrefetchPatterns string
	// WithTar puts bootstrap into a tar stream (no gzip).
	WithTar bool
	// OCIRef enables converting OCI tar(.gz) blob to nydus referenced blob.
	OCIRef bool
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
	// Stream enables streaming mode, which doesn't unpack the blob data to disk,
	// but setup a http server to serve the blob data.
	Stream bool
}

type TOCEntry struct {
	// Feature flags of entry
	Flags     uint32
	Reserved1 uint32
	// Name of entry data
	Name [16]byte
	// Sha256 of uncompressed entry data
	UncompressedDigest [32]byte
	// Offset of compressed entry data
	CompressedOffset uint64
	// Size of compressed entry data
	CompressedSize uint64
	// Size of uncompressed entry data
	UncompressedSize uint64
	Reserved2        [44]byte
}

func (entry *TOCEntry) GetCompressor() (Compressor, error) {
	switch {
	case entry.Flags&CompressorNone == CompressorNone:
		return CompressorNone, nil
	case entry.Flags&CompressorZstd == CompressorZstd:
		return CompressorZstd, nil
	}
	return 0, fmt.Errorf("unsupported compressor, entry flags %x", entry.Flags)
}

func (entry *TOCEntry) GetName() string {
	var name strings.Builder
	name.Grow(16)
	for _, c := range entry.Name {
		if c == 0 {
			break
		}
		fmt.Fprintf(&name, "%c", c)
	}
	return name.String()
}

func (entry *TOCEntry) GetUncompressedDigest() string {
	return fmt.Sprintf("%x", entry.UncompressedDigest)
}

func (entry *TOCEntry) GetCompressedOffset() uint64 {
	return entry.CompressedOffset
}

func (entry *TOCEntry) GetCompressedSize() uint64 {
	return entry.CompressedSize
}

func (entry *TOCEntry) GetUncompressedSize() uint64 {
	return entry.UncompressedSize
}
