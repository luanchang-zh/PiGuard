package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"piguard/go-backend/internal/protocol"
)

// Storage operations remain outside SQLite transactions and the Monitor lock.
type frameStore interface {
	Save(context.Context, string, []byte) (string, error)
	Open(string) (*os.File, error)
	Remove(string) error
}
type diskFrameStore struct{ dir string }

func (s diskFrameStore) Save(ctx context.Context, id string, data []byte) (path string, err error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	temp, final := ".pending-"+id, id+".jpg"
	f, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	defer root.Remove(temp)
	defer f.Close()
	if _, err = io.Copy(f, protocol.ContextReader{Context: ctx, Reader: bytes.NewReader(data)}); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	// Link publishes without overwriting an existing immutable file.
	if err = root.Link(temp, final); err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = root.Remove(final)
		}
	}()
	if err = root.Remove(temp); err != nil {
		return "", err
	}
	directory, err := root.Open(".")
	if err != nil {
		return "", err
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	return final, nil
}
func (s diskFrameStore) Open(path string) (*os.File, error) {
	if !filepath.IsLocal(path) || filepath.Base(path) != path {
		return nil, fmt.Errorf("invalid snapshot storage path")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("snapshot is not a regular file")
	}
	return f, nil
}
func (s diskFrameStore) Remove(path string) error {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(path)
}
