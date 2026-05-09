package certstore

import (
	"bytes"
	"cmp"
	"context"
	"io/fs"
	"strings"
	"sync"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/oops"
)

// MutationKind describes a committed certificate storage mutation.
type MutationKind string

const (
	MutationStore  MutationKind = "store"
	MutationDelete MutationKind = "delete"
)

// Object is a terminal certificate storage key.
type Object struct {
	Key      string
	Value    []byte
	Modified time.Time
}

// Mutation is the Raft-friendly write model for Projection.
type Mutation struct {
	Kind     MutationKind
	Key      string
	Value    []byte
	Modified time.Time
}

// Snapshot is a defensive copy of a Projection.
type Snapshot struct {
	Objects *collectionmapping.Map[string, Object]
	Keys    *collectionlist.List[string]
}

// Projection is a thread-safe in-memory certificate KV model.
// It is suitable as the hot read path for file-backed and future Raft-backed
// storage implementations.
type Projection struct {
	initOnce sync.Once
	objects  *collectionmapping.ConcurrentMap[string, Object]
	locker   *LocalLocker
	now      func() time.Time
}

func NewProjection(objects ...Object) *Projection {
	projection := &Projection{
		objects: collectionmapping.NewConcurrentMapWithCapacity[string, Object](len(objects)),
		locker:  NewLocalLocker(),
		now:     time.Now,
	}
	for _, object := range objects {
		object = normalizeObject(object, projection.now())
		if object.Key != "" {
			projection.objects.Set(object.Key, object)
		}
	}
	return projection
}

func (p *Projection) Store(ctx context.Context, key string, value []byte) error {
	return p.Apply(ctx, Mutation{
		Kind:  MutationStore,
		Key:   key,
		Value: value,
	})
}

func (p *Projection) Load(ctx context.Context, key string) ([]byte, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	key = cleanKey(key)
	if err := validateFileKey(key); err != nil {
		return nil, err
	}
	p.ensureInit()
	object, ok := p.objects.Get(key)
	if !ok {
		return nil, fs.ErrNotExist
	}
	return bytes.Clone(object.Value), nil
}

func (p *Projection) Delete(ctx context.Context, key string) error {
	return p.Apply(ctx, Mutation{
		Kind: MutationDelete,
		Key:  key,
	})
}

func (p *Projection) Exists(ctx context.Context, key string) bool {
	if err := contextErr(ctx); err != nil {
		return false
	}
	key = cleanKey(key)
	p.ensureInit()
	if _, ok := p.objects.Get(key); ok {
		return true
	}
	return p.hasDescendant(key)
}

func (p *Projection) List(ctx context.Context, prefix string, recursive bool) (*collectionlist.List[string], error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	prefix = cleanKey(prefix)
	p.ensureInit()

	seen := collectionmapping.NewMap[string, struct{}]()
	keys := collectionlist.NewListWithCapacity[string](p.objects.Len())
	p.objects.Range(func(key string, _ Object) bool {
		for _, listedKey := range listedKeys(prefix, key, recursive) {
			if _, ok := seen.Get(listedKey); ok {
				continue
			}
			seen.Set(listedKey, struct{}{})
			keys.Add(listedKey)
		}
		return true
	})
	keys.Sort(cmp.Compare[string])
	if keys.IsEmpty() && !p.Exists(ctx, prefix) {
		return keys, fs.ErrNotExist
	}
	return keys, nil
}

func (p *Projection) Stat(ctx context.Context, key string) (KeyInfo, error) {
	if err := contextErr(ctx); err != nil {
		return KeyInfo{}, err
	}
	key = cleanKey(key)
	p.ensureInit()
	if object, ok := p.objects.Get(key); ok {
		return KeyInfo{
			Key:        key,
			Modified:   object.Modified,
			Size:       int64(len(object.Value)),
			IsTerminal: true,
		}, nil
	}

	info, ok := p.directoryInfo(key)
	if !ok {
		return KeyInfo{}, fs.ErrNotExist
	}
	return info, nil
}

func (p *Projection) Lock(ctx context.Context, name string) error {
	p.ensureInit()
	return p.locker.Lock(ctx, name)
}

func (p *Projection) Unlock(ctx context.Context, name string) error {
	p.ensureInit()
	return p.locker.Unlock(ctx, name)
}

func (p *Projection) Apply(ctx context.Context, mutation Mutation) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	p.ensureInit()
	switch mutation.Kind {
	case MutationStore:
		return p.applyStore(mutation)
	case MutationDelete:
		p.applyDelete(mutation.Key)
		return nil
	default:
		return oops.
			In("certstore").
			With("kind", mutation.Kind).
			New("unknown certificate storage mutation")
	}
}

func (p *Projection) Snapshot(ctx context.Context) (*Snapshot, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	p.ensureInit()
	objects := collectionmapping.NewMapWithCapacity[string, Object](p.objects.Len())
	keys := collectionlist.NewListWithCapacity[string](p.objects.Len())
	p.objects.Range(func(key string, object Object) bool {
		objects.Set(key, cloneObject(object))
		keys.Add(key)
		return true
	})
	keys.Sort(cmp.Compare[string])
	return &Snapshot{
		Objects: objects,
		Keys:    keys,
	}, nil
}

func (p *Projection) applyStore(mutation Mutation) error {
	key := cleanKey(mutation.Key)
	if err := validateFileKey(key); err != nil {
		return err
	}
	modified := mutation.Modified
	if modified.IsZero() {
		modified = p.now()
	}
	p.objects.Set(key, Object{
		Key:      key,
		Value:    bytes.Clone(mutation.Value),
		Modified: modified,
	})
	return nil
}

func (p *Projection) applyDelete(key string) {
	key = cleanKey(key)
	if key == "" {
		p.objects.Clear()
		return
	}
	prefix := keyPrefix(key)
	keys := collectionlist.NewListWithCapacity[string](p.objects.Len())
	p.objects.Range(func(candidate string, _ Object) bool {
		if candidate == key || strings.HasPrefix(candidate, prefix) {
			keys.Add(candidate)
		}
		return true
	})
	keys.Range(func(_ int, candidate string) bool {
		p.objects.Delete(candidate)
		return true
	})
}

func (p *Projection) hasDescendant(key string) bool {
	prefix := keyPrefix(key)
	found := false
	p.objects.Range(func(candidate string, _ Object) bool {
		if key == "" || strings.HasPrefix(candidate, prefix) {
			found = true
		}
		return !found
	})
	return found
}

func (p *Projection) directoryInfo(key string) (KeyInfo, bool) {
	prefix := keyPrefix(key)
	var modified time.Time
	found := false
	p.objects.Range(func(candidate string, object Object) bool {
		if key != "" && !strings.HasPrefix(candidate, prefix) {
			return true
		}
		found = true
		if object.Modified.After(modified) {
			modified = object.Modified
		}
		return true
	})
	if !found {
		return KeyInfo{}, false
	}
	return KeyInfo{
		Key:        key,
		Modified:   modified,
		IsTerminal: false,
	}, true
}

func (p *Projection) ensureInit() {
	p.initOnce.Do(func() {
		if p.objects == nil {
			p.objects = collectionmapping.NewConcurrentMap[string, Object]()
		}
		if p.locker == nil {
			p.locker = NewLocalLocker()
		}
		if p.now == nil {
			p.now = time.Now
		}
	})
}
