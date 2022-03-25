/*
 * Copyright (c) 2022. Nydus Developers. All rights reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package converter

import (
	"context"
	"io"
)

type readCloser struct {
	io.ReadCloser
	action func() error
}

func (c readCloser) Close() error {
	if err := c.ReadCloser.Close(); err != nil {
		return err
	}
	return c.action()
}

func newReadCloser(reader io.ReadCloser, action func() error) readCloser {
	return readCloser{
		ReadCloser: reader,
		action:     action,
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
