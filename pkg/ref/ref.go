/*
 * Copyright (c) 2022. Nydus Developers. All rights reserved.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package ref

import (
	"fmt"
	"io"
	"os"
	"path"

	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"

	"github.com/containerd/containerd/content/local"
	"github.com/containerd/nydus-snapshotter/pkg/converter"
	"github.com/containerd/nydus-snapshotter/pkg/label"
	"github.com/containerd/nydus-snapshotter/pkg/resolve"
	"github.com/containerd/nydus-snapshotter/pkg/utils/registry"
)

type Manager struct {
	blobCacheDir string
	resolver     *resolve.Resolver
}

func NewManager(blobCacheDir string, resolver *resolve.Resolver) *Manager {
	return &Manager{
		blobCacheDir: blobCacheDir,
		resolver:     resolver,
	}
}

func (m *Manager) PrepareRefLayer(labels map[string]string) error {
	ref, layerDigest := registry.ParseLabels(labels)
	if ref == "" || layerDigest == "" {
		return fmt.Errorf("can't find ref and digest from label %+v", labels)
	}

	rc, err := m.resolver.Resolve(ref, layerDigest, labels)
	if err != nil {
		return errors.Wrapf(err, "resolve from ref %s, digest %s", ref, layerDigest)
	}
	defer rc.Close()

	blobFile, err := os.CreateTemp(m.blobCacheDir, "downloading-")
	if err != nil {
		return errors.Wrap(err, "create temp file for downloading blob")
	}
	defer func() {
		blobFile.Close()
		os.Remove(blobFile.Name())
	}()

	_, err = io.Copy(blobFile, rc)
	if err != nil {
		return errors.Wrap(err, "write blob to local file")
	}

	ra, err := local.OpenReader(blobFile.Name())
	if err != nil {
		return errors.Wrap(err, "open blob as reader")
	}

	digestStr, ok := labels[label.NydusRefLayer]
	if !ok {
		return fmt.Errorf("not found label %s", label.NydusRefLayer)
	}

	originalBlobDigest, err := digest.Parse(digestStr)
	if err != nil {
		return errors.Wrapf(err, "invalid ref label %s=%s", label.NydusRefLayer, digestStr)
	}

	blobMetaPath := path.Join(m.blobCacheDir, fmt.Sprintf("%s.blob.meta", originalBlobDigest.Hex()))
	blobMetaFile, err := os.Create(blobMetaPath)
	if err != nil {
		return errors.Wrap(err, "create blob meta file")
	}
	defer blobMetaFile.Close()

	if err := converter.UnpackBlobMetaFromNydusTar(ra, blobMetaFile); err != nil {
		return errors.Wrap(err, "unpack blob meta from nydus blob")
	}

	fmt.Println("FETCHED", blobMetaPath)

	return nil
}
