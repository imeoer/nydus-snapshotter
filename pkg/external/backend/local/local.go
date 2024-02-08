package local

import (
	"context"

	"github.com/containerd/nydus-snapshotter/pkg/external/backend"
)

type Object struct {
	Path string `msgpack:"p"`
}

type Handler struct {
	root string
}

func NewHandler(root string) *Handler {
	return &Handler{
		root: root,
	}
}

func (handler *Handler) Handle(_ context.Context, path string) (backend.Object, error) {
	return Object{
		Path: path,
	}, nil
}

func (handler *Handler) Backend(_ context.Context) (*backend.Backend, error) {
	return &backend.Backend{
		Type: "local",
		Config: map[string]string{
			"root": handler.root,
		},
	}, nil
}
