package raftnode

import (
	"context"
	"log/slog"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/storx/bboltx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/samber/oops"
)

const (
	stateBucketName = "state"
	routeBucketName = "routes"
	routeKeySep     = "\x00"
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

func (s *stateStore) LoadState(ctx context.Context, group string) (State, bool, error) {
	if s == nil || s.state == nil {
		return State{}, false, nil
	}
	state, ok, err := s.state.Get(ctx, normalizeGroupName(group))
	if err != nil {
		return State{}, false, oops.
			In("raftnode").
			With("bucket", stateBucketName, "group", group).
			Wrapf(err, "load raft state")
	}
	return cloneState(state), ok, nil
}

func (s *stateStore) SaveState(ctx context.Context, group string, state State) error {
	if s == nil {
		return nil
	}
	group = normalizeGroupName(group)
	state = cloneState(state)
	if err := s.state.Put(ctx, group, state); err != nil {
		return oops.
			In("raftnode").
			With("bucket", stateBucketName, "group", group, "version", state.Version).
			Wrapf(err, "save raft state")
	}
	return s.replaceRoutes(ctx, group, state.Routes)
}

func (s *stateStore) LoadRoutes(ctx context.Context, group string) (*collectionlist.List[RouteRecord], error) {
	if s == nil || s.routes == nil {
		return collectionlist.NewList[RouteRecord](), nil
	}
	group = normalizeGroupName(group)
	prefix := routeGroupPrefix(group)
	routes := collectionlist.NewList[RouteRecord]()
	err := s.routes.View(ctx, func(tx bboltx.ViewTx[string, RouteRecord]) error {
		return tx.ForEach(func(key string, route RouteRecord) error {
			if strings.HasPrefix(key, prefix) {
				routes.Add(route)
			}
			return nil
		})
	})
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("bucket", routeBucketName, "group", group).
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

func (s *stateStore) replaceRoutes(ctx context.Context, group string, routes *collectionlist.List[RouteRecord]) error {
	if s == nil || s.routes == nil {
		return nil
	}
	if err := s.routes.Update(ctx, func(tx bboltx.UpdateTx[string, RouteRecord]) error {
		return replaceRouteRecords(group, tx, routes)
	}); err != nil {
		routeCount := 0
		if routes != nil {
			routeCount = routes.Len()
		}
		return oops.
			In("raftnode").
			With("bucket", routeBucketName, "group", group, "routes", routeCount).
			Wrapf(err, "replace raft routes")
	}
	return nil
}

func replaceRouteRecords(group string, tx bboltx.UpdateTx[string, RouteRecord], routes *collectionlist.List[RouteRecord]) error {
	existingKeys, err := routeRecordKeys(group, tx)
	if err != nil {
		return err
	}
	if err := tx.DeleteMany(existingKeys.Values()...); err != nil {
		return oops.
			In("raftnode").
			With("bucket", routeBucketName, "group", group, "keys", existingKeys.Len()).
			Wrapf(err, "delete existing raft routes")
	}
	return putRouteRecords(group, tx, routes)
}

func routeRecordKeys(group string, tx bboltx.UpdateTx[string, RouteRecord]) (*collectionlist.List[string], error) {
	existingKeys := collectionlist.NewList[string]()
	prefix := routeGroupPrefix(group)
	if err := tx.ForEach(func(key string, _ RouteRecord) error {
		if strings.HasPrefix(key, prefix) {
			existingKeys.Add(key)
		}
		return nil
	}); err != nil {
		return nil, oops.
			In("raftnode").
			With("bucket", routeBucketName, "group", group).
			Wrapf(err, "scan existing raft route keys")
	}
	return existingKeys, nil
}

func putRouteRecords(group string, tx bboltx.UpdateTx[string, RouteRecord], routes *collectionlist.List[RouteRecord]) error {
	if routes == nil {
		return nil
	}
	var putErr error
	routes.Range(func(_ int, route RouteRecord) bool {
		if route.Name == "" {
			return true
		}
		if err := tx.Put(routeKey(group, route.Name), route); err != nil {
			putErr = oops.
				In("raftnode").
				With("bucket", routeBucketName, "group", group, "route", route.Name).
				Wrapf(err, "save raft route")
			return false
		}
		return true
	})
	return putErr
}

func normalizeGroupName(group string) string {
	group = strings.TrimSpace(group)
	if group == "" {
		return DefaultGroupName
	}
	return group
}

func routeGroupPrefix(group string) string {
	return normalizeGroupName(group) + routeKeySep
}

func routeKey(group, routeName string) string {
	return routeGroupPrefix(group) + routeName
}
