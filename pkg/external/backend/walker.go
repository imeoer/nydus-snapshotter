package backend

import (
	"context"
	"io/fs"
	"path/filepath"

	"github.com/pkg/errors"
)

const throttleFileSize = 1024 * 1024 * 2 // 2 MB
const fileChunkSize = 1024 * 1024 * 1    // 1 MB

type Walker struct {
}

func NewWalker() *Walker {
	return &Walker{}
}

func (walker *Walker) Walk(ctx context.Context, root string, handler Handler) (*Result, error) {
	objects := []Object{}
	chunks := []Chunk{}
	files := []string{}

	addFile := func(size int64, relativeTarget string) error {
		target := filepath.Join("/", relativeTarget)
		objectIndex := uint32(len(objects))
		object, err := handler.Handle(ctx, relativeTarget)
		if err != nil {
			return err
		}
		objects = append(objects, object)
		fileChunks := SplitChunks(size, fileChunkSize, objectIndex)
		chunks = append(chunks, fileChunks...)
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

		if info.Size() < throttleFileSize {
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
		Objects: objects,
		Files:   files,
		Backend: *bkd,
	}, nil
}
