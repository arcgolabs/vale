package raftnode

import (
	"context"
	"log/slog"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/storx/bboltx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/samber/oops"
)

const (
	stateBucketName = "state"
	routeBucketName = "routes"
	currentStateKey = "current"
)

type stateStore struct {
	db     *bboltx.DB
	state  *bboltx.Bucket[string, State]
	routes *bboltx.Bucket[string, RouteRecord]
}

func openStateStore(path string, logger *slog.Logger) (*stateStore, error) {
	db, err := bboltx.Open(path, 0o600, nil, bboltx.WithDBLogger(logger))
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("path", path).
			Wrapf(err, "open raft state store")
	}
	return newStateStore(db), nil
}

func newStateStore(db *bboltx.DB) *stateStore {
	return &stateStore{
		db: db,
		state: bboltx.NewBucketWithDB(
			db,
			stateBucketName,
			keycodec.String(),
			codec.JSON[State](),
		),
		routes: bboltx.NewBucketWithDB(
			db,
			routeBucketName,
			keycodec.String(),
			codec.JSON[RouteRecord](),
		),
	}
}

func (s *stateStore) LoadState(ctx context.Context) (State, bool, error) {
	if s == nil || s.state == nil {
		return State{}, false, nil
	}
	state, ok, err := s.state.Get(ctx, currentStateKey)
	if err != nil {
		return State{}, false, oops.
			In("raftnode").
			With("bucket", stateBucketName, "key", currentStateKey).
			Wrapf(err, "load raft state")
	}
	return cloneState(state), ok, nil
}

func (s *stateStore) SaveState(ctx context.Context, state State) error {
	if s == nil {
		return nil
	}
	state = cloneState(state)
	if err := s.state.Put(ctx, currentStateKey, state); err != nil {
		return oops.
			In("raftnode").
			With("bucket", stateBucketName, "key", currentStateKey, "version", state.Version).
			Wrapf(err, "save raft state")
	}
	return s.replaceRoutes(ctx, state.Routes)
}

func (s *stateStore) LoadRoutes(ctx context.Context) (*collectionlist.List[RouteRecord], error) {
	if s == nil || s.routes == nil {
		return collectionlist.NewList[RouteRecord](), nil
	}
	routes := collectionlist.NewList[RouteRecord]()
	err := s.routes.View(ctx, func(tx bboltx.ViewTx[string, RouteRecord]) error {
		return tx.ForEach(func(_ string, route RouteRecord) error {
			routes.Add(route)
			return nil
		})
	})
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("bucket", routeBucketName).
			Wrapf(err, "load raft routes")
	}
	return routes, nil
}

func (s *stateStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	if err := s.db.Close(); err != nil {
		return oops.
			In("raftnode").
			Wrapf(err, "close raft state store")
	}
	return nil
}

func (s *stateStore) replaceRoutes(ctx context.Context, routes *collectionlist.List[RouteRecord]) error {
	if s == nil || s.routes == nil {
		return nil
	}
	if err := s.routes.Update(ctx, func(tx bboltx.UpdateTx[string, RouteRecord]) error {
		return replaceRouteRecords(tx, routes)
	}); err != nil {
		return oops.
			In("raftnode").
			With("bucket", routeBucketName, "routes", routes.Len()).
			Wrapf(err, "replace raft routes")
	}
	return nil
}

func replaceRouteRecords(tx bboltx.UpdateTx[string, RouteRecord], routes *collectionlist.List[RouteRecord]) error {
	existingKeys, err := routeRecordKeys(tx)
	if err != nil {
		return err
	}
	if err := tx.DeleteMany(existingKeys.Values()...); err != nil {
		return oops.
			In("raftnode").
			With("bucket", routeBucketName, "keys", existingKeys.Len()).
			Wrapf(err, "delete existing raft routes")
	}
	return putRouteRecords(tx, routes)
}

func routeRecordKeys(tx bboltx.UpdateTx[string, RouteRecord]) (*collectionlist.List[string], error) {
	existingKeys := collectionlist.NewList[string]()
	if err := tx.ForEach(func(key string, _ RouteRecord) error {
		existingKeys.Add(key)
		return nil
	}); err != nil {
		return nil, oops.
			In("raftnode").
			With("bucket", routeBucketName).
			Wrapf(err, "scan existing raft route keys")
	}
	return existingKeys, nil
}

func putRouteRecords(tx bboltx.UpdateTx[string, RouteRecord], routes *collectionlist.List[RouteRecord]) error {
	var putErr error
	routes.Range(func(_ int, route RouteRecord) bool {
		if route.Name == "" {
			return true
		}
		if err := tx.Put(route.Name, route); err != nil {
			putErr = oops.
				In("raftnode").
				With("bucket", routeBucketName, "route", route.Name).
				Wrapf(err, "save raft route")
			return false
		}
		return true
	})
	return putErr
}
