package backend

import (
	"context"
	"io/fs"
	"path/filepath"

	"github.com/pkg/errors"
)

type Walker struct {
}

func NewWalker() *Walker {
	return &Walker{}
}

func (walker *Walker) Walk(ctx context.Context, root string, handler Handler) (*Result, error) {
	chunks := []Chunk{}
	files := []string{}

	addFile := func(size int64, relativeTarget string) error {
		target := filepath.Join("/", relativeTarget)
		_chunks, err := handler.Handle(ctx, File{
			RelativePath: relativeTarget,
			Size:         size,
		})
		if err != nil {
			return err
		}
		chunks = append(chunks, _chunks...)
		files = append(files, target)
		return nil
	}

	walkFiles := []func() error{}

	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}

		if info.Size() < DefaultThrottleFileSize {
			return nil
		}

		target, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		walkFiles = append(walkFiles, func() error {
			return addFile(info.Size(), target)
		})

		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "walk directory")
	}

	for i := len(walkFiles) - 1; i >= 0; i-- {
		if err := walkFiles[i](); err != nil {
			return nil, errors.Wrap(err, "handle files")
		}
	}

	bkd, err := handler.Backend(ctx)
	if err != nil {
		return nil, err
	}

	return &Result{
		Chunks:  chunks,
		Files:   files,
		Backend: *bkd,
	}, nil
}
