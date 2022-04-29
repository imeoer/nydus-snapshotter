/*
 * Copyright (c) 2022. Nydus Developers. All rights reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package converter

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/containerd/containerd/archive"
	"github.com/containerd/containerd/archive/compression"
	"github.com/containerd/containerd/content"
	"github.com/containerd/fifo"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/containerd/nydus-snapshotter/pkg/converter/tool"
)

const bootstrapNameInTar = "image.boot"

const envNydusBuilder = "NYDUS_BUILDER"
const envNydusWorkdir = "NYDUS_WORKDIR"

type Layer struct {
	// Digest represents the hash of whole tar blob.
	Digest digest.Digest
	// ReaderAt holds the reader of whole tar blob.
	ReaderAt content.ReaderAt
}

type ConvertOption struct {
	// RafsVersion specifies nydus format version, possible values:
	// `5`, `6` (EROFS-compatible), default is `5`.
	RafsVersion string
	// ChunkDictPath holds the bootstrap path of chunk dict image.
	ChunkDictPath string
	// PrefetchPatterns holds file path pattern list want to prefetch.
	PrefetchPatterns string
}

type MergeOption struct {
	// ChunkDictPath holds the bootstrap path of chunk dict image.
	ChunkDictPath string
	// PrefetchPatterns holds file path pattern list want to prefetch.
	PrefetchPatterns string
	// WithTar puts bootstrap into a tar stream (no gzip).
	WithTar bool
}

func getBuilder() string {
	builderPath := os.Getenv(envNydusBuilder)
	if builderPath == "" {
		builderPath = "nydus-image"
	}
	return builderPath
}

// Unpack a OCI formatted tar stream into a directory.
func unpackOciTar(ctx context.Context, dst string, reader io.Reader) error {
	ds, err := compression.DecompressStream(reader)
	if err != nil {
		return errors.Wrap(err, "decompress stream")
	}
	defer ds.Close()

	if _, err := archive.Apply(
		ctx,
		dst,
		ds,
		archive.WithConvertWhiteout(func(hdr *tar.Header, file string) (bool, error) {
			// Keep to extract all whiteout files.
			return true, nil
		}),
	); err != nil {
		return errors.Wrap(err, "apply with convert whiteout")
	}

	return nil
}

// Unpack the bootstrap from Nydus formatted tar stream (blob + bootstrap).
func unpackBootstrapFromNydusTar(ctx context.Context, ra content.ReaderAt, target io.Writer) error {
	cur := ra.Size()
	reader := newSeekReader(ra)

	const headerSize = 512

	for {
		var err error
		cur, err = reader.Seek(cur-headerSize, io.SeekCurrent)
		if err != nil {
			return errors.Wrapf(err, "seek to %d for header", cur-headerSize)
		}

		tr := tar.NewReader(reader)
		hdr, err := tr.Next()
		if err != nil {
			return errors.Wrap(err, "parse tar header")
		}

		if hdr.Name == bootstrapNameInTar {
			targetOffset := cur - hdr.Size
			_, err = reader.Seek(targetOffset, io.SeekStart)
			if err != nil {
				return errors.Wrap(err, "seek to target offset")
			}

			if _, err := io.CopyN(target, reader, hdr.Size); err != nil {
				return errors.Wrap(err, "copy target to reader")
			}

			return nil
		}

		if cur == hdr.Size {
			break
		}
	}

	return fmt.Errorf("can't find bootstrap in nydus tar")
}

// Convert converts a OCI formatted tar stream to a nydus formatted tar stream
//
// The nydus blob tar stream contains blob and bootstrap files with the following
// file tree structure:
//
// /image
// ├── image.blob
// ├── image.boot
//
// So for the chunk of files in the nydus boostreap, a blob compressed offset
// of 1024 (size_of(tar_header) * 2) is required.
//
// Important: the caller must check `io.WriteCloser.Close() == nil` to ensure
// the conversion workflow is finish.
func Convert(ctx context.Context, dest io.Writer, opt ConvertOption) (io.WriteCloser, error) {
	workDir, err := ioutil.TempDir(os.Getenv(envNydusWorkdir), "nydus-converter-")
	if err != nil {
		return nil, errors.Wrap(err, "create work directory")
	}
	defer func() {
		if err != nil {
			os.RemoveAll(workDir)
		}
	}()

	sourceDir := filepath.Join(workDir, "source")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		return nil, errors.Wrap(err, "create source directory")
	}

	pr, pw := io.Pipe()

	unpackDone := make(chan bool, 1)
	go func() {
		if err := unpackOciTar(ctx, sourceDir, pr); err != nil {
			pr.CloseWithError(errors.Wrapf(err, "unpack to %s", sourceDir))
			return
		}
		unpackDone <- true
	}()

	wc := newWriteCloser(pw, func() error {
		defer func() {
			os.RemoveAll(workDir)
		}()

		// Because PipeWriter#Close is called does not mean that the PipeReader
		// has finished reading all the data, and unpack may not be complete yet,
		// so we need to wait for that here.
		<-unpackDone

		blobPath := filepath.Join(workDir, "blob")
		blobRc, err := fifo.OpenFifo(ctx, blobPath, syscall.O_CREAT|syscall.O_RDONLY|syscall.O_NONBLOCK, 0644)
		if err != nil {
			return errors.Wrap(err, "create fifo file")
		}
		defer blobRc.Close()

		go func() {
			if err := tool.Convert(tool.ConvertOption{
				BuilderPath: getBuilder(),

				BlobPath:         blobPath,
				RafsVersion:      opt.RafsVersion,
				SourcePath:       sourceDir,
				ChunkDictPath:    opt.ChunkDictPath,
				PrefetchPatterns: opt.PrefetchPatterns,
			}); err != nil {
				pr.CloseWithError(errors.Wrapf(err, "convert blob for %s", sourceDir))
				blobRc.Close()
			}
		}()

		if _, err := io.Copy(dest, blobRc); err != nil {
			return errors.Wrap(err, "pack nydus tar")
		}

		return nil
	})

	return wc, nil
}

// Merge multiple nydus boostraps (from every layer of image) to a final boostrap.
func Merge(ctx context.Context, layers []Layer, dest io.Writer, opt MergeOption) error {
	workDir, err := ioutil.TempDir(os.Getenv(envNydusWorkdir), "nydus-converter-")
	if err != nil {
		return errors.Wrap(err, "create work directory")
	}
	defer os.RemoveAll(workDir)

	eg, ctx := errgroup.WithContext(ctx)
	sourceBootstrapPaths := []string{}
	for idx := range layers {
		sourceBootstrapPaths = append(sourceBootstrapPaths, filepath.Join(workDir, layers[idx].Digest.Hex()))
		eg.Go(func(idx int) func() error {
			return func() error {
				layer := layers[idx]

				// Use the hex hash string of whole tar blob as the boostrap name.
				bootstrap, err := os.Create(filepath.Join(workDir, layer.Digest.Hex()))
				if err != nil {
					return errors.Wrap(err, "create source bootstrap")
				}
				defer bootstrap.Close()

				if err := unpackBootstrapFromNydusTar(ctx, layer.ReaderAt, bootstrap); err != nil {
					return errors.Wrap(err, "unpack nydus tar")
				}

				return nil
			}
		}(idx))
	}

	if err := eg.Wait(); err != nil {
		return errors.Wrap(err, "unpack all bootstraps")
	}

	targetBootstrapPath := filepath.Join(workDir, "bootstrap")

	if err := tool.Merge(tool.MergeOption{
		BuilderPath: getBuilder(),

		SourceBootstrapPaths: sourceBootstrapPaths,
		TargetBootstrapPath:  targetBootstrapPath,
		ChunkDictPath:        opt.ChunkDictPath,
		PrefetchPatterns:     opt.PrefetchPatterns,
	}); err != nil {
		return errors.Wrap(err, "merge bootstrap")
	}

	var rc io.ReadCloser

	if opt.WithTar {
		rc, err = packToTar(targetBootstrapPath, fmt.Sprintf("image/%s", bootstrapNameInTar), false)
		if err != nil {
			return errors.Wrap(err, "pack bootstrap to tar")
		}
	} else {
		rc, err = os.Open(targetBootstrapPath)
		if err != nil {
			return errors.Wrap(err, "open targe bootstrap")
		}
	}
	defer rc.Close()

	if _, err = io.Copy(dest, rc); err != nil {
		return errors.Wrap(err, "copy merged bootstrap")
	}

	return nil
}
