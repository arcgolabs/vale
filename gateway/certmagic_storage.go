package gateway

import (
	"context"

	"github.com/arcgolabs/vale/certstore"
	"github.com/caddyserver/certmagic"
	"github.com/samber/oops"
)

type certMagicStorage struct {
	storage certstore.Storage
}

var _ certmagic.Storage = certMagicStorage{}

func newCertMagicStorage(storage certstore.Storage) certmagic.Storage {
	return certMagicStorage{storage: storage}
}

func (s certMagicStorage) Store(ctx context.Context, key string, value []byte) error {
	if err := s.storage.Store(ctx, key, value); err != nil {
		return oops.
			In("gateway").
			With("key", key).
			Wrapf(err, "store acme certificate data")
	}
	return nil
}

func (s certMagicStorage) Load(ctx context.Context, key string) ([]byte, error) {
	value, err := s.storage.Load(ctx, key)
	if err != nil {
		return nil, oops.
			In("gateway").
			With("key", key).
			Wrapf(err, "load acme certificate data")
	}
	return value, nil
}

func (s certMagicStorage) Delete(ctx context.Context, key string) error {
	if err := s.storage.Delete(ctx, key); err != nil {
		return oops.
			In("gateway").
			With("key", key).
			Wrapf(err, "delete acme certificate data")
	}
	return nil
}

func (s certMagicStorage) Exists(ctx context.Context, key string) bool {
	return s.storage.Exists(ctx, key)
}

func (s certMagicStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	keys, err := s.storage.List(ctx, prefix, recursive)
	if err != nil {
		return nil, oops.
			In("gateway").
			With("prefix", prefix, "recursive", recursive).
			Wrapf(err, "list acme certificate data")
	}
	return keys.Values(), nil
}

func (s certMagicStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	info, err := s.storage.Stat(ctx, key)
	if err != nil {
		return certmagic.KeyInfo{}, oops.
			In("gateway").
			With("key", key).
			Wrapf(err, "stat acme certificate data")
	}
	return certmagic.KeyInfo{
		Key:        info.Key,
		Modified:   info.Modified,
		Size:       info.Size,
		IsTerminal: info.IsTerminal,
	}, nil
}

func (s certMagicStorage) Lock(ctx context.Context, name string) error {
	if err := s.storage.Lock(ctx, name); err != nil {
		return oops.
			In("gateway").
			With("name", name).
			Wrapf(err, "lock acme certificate storage")
	}
	return nil
}

func (s certMagicStorage) Unlock(ctx context.Context, name string) error {
	if err := s.storage.Unlock(ctx, name); err != nil {
		return oops.
			In("gateway").
			With("name", name).
			Wrapf(err, "unlock acme certificate storage")
	}
	return nil
}
