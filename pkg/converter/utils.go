/*
 * Copyright (c) 2022. Nydus Developers. All rights reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package converter

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type readCloser struct {
	io.ReadCloser
	action func() error
}

func (c *readCloser) Close() error {
	if err := c.ReadCloser.Close(); err != nil {
		return err
	}
	return c.action()
}

func newReadCloser(rc io.ReadCloser, action func() error) *readCloser {
	return &readCloser{
		ReadCloser: rc,
		action:     action,
	}
}

type writeCloser struct {
	io.WriteCloser
	action func() error
}

func (c *writeCloser) Close() error {
	if err := c.action(); err != nil {
		return err
	}
	return c.WriteCloser.Close()
}

func newWriteCloser(wc io.WriteCloser, action func() error) *writeCloser {
	return &writeCloser{
		WriteCloser: wc,
		action:      action,
	}
}

type readerAtToReader struct {
	io.ReaderAt
	pos int64
}

func (ra *readerAtToReader) Read(p []byte) (int, error) {
	n, err := ra.ReaderAt.ReadAt(p, ra.pos)
	ra.pos += int64(len(p))
	return n, err
}

func (ra *readerAtToReader) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekCurrent {
		return 0, fmt.Errorf("only support SeekCurrent whence")
	}
	ra.pos += offset
	return ra.pos, nil
}

func newReaderAtToReader(ra io.ReaderAt) *readerAtToReader {
	return &readerAtToReader{
		ReaderAt: ra,
		pos:      0,
	}
}

type ctxReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *ctxReader) Read(p []byte) (n int, err error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func newCtxReader(ctx context.Context, reader io.Reader) io.Reader {
	return &ctxReader{
		ctx:    ctx,
		reader: reader,
	}
}

// packToTar makes .tar(.gz) stream of file named `name` and return reader.
func packToTar(src string, name string, compress bool) (io.ReadCloser, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return nil, err
	}

	dirHdr := &tar.Header{
		Name:     filepath.Dir(name),
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}

	hdr := &tar.Header{
		Name: name,
		Mode: 0444,
		Size: fi.Size(),
	}

	reader, writer := io.Pipe()

	go func() {
		// Prepare targz writer
		var tw *tar.Writer
		var gw *gzip.Writer
		var err error
		var file *os.File

		if compress {
			gw = gzip.NewWriter(writer)
			tw = tar.NewWriter(gw)
		} else {
			tw = tar.NewWriter(writer)
		}

		defer func() {
			err1 := tw.Close()
			var err2 error
			if gw != nil {
				err2 = gw.Close()
			}

			var finalErr error

			// Return the first error encountered to the other end and ignore others.
			if err != nil {
				finalErr = err
			} else if err1 != nil {
				finalErr = err1
			} else if err2 != nil {
				finalErr = err2
			}

			writer.CloseWithError(finalErr)
		}()

		file, err = os.Open(src)
		if err != nil {
			return
		}
		defer file.Close()

		// Write targz stream
		if err = tw.WriteHeader(dirHdr); err != nil {
			return
		}

		if err = tw.WriteHeader(hdr); err != nil {
			return
		}

		if _, err = io.Copy(tw, file); err != nil {
			return
		}
	}()

	return reader, nil
}
