package raftnode

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/arcgolabs/storx/bboltx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
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
		return nil, fmt.Errorf("open raft state store: %w", err)
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
	return cloneState(state), ok, err
}

func (s *stateStore) SaveState(ctx context.Context, state State) error {
	if s == nil {
		return nil
	}
	state = cloneState(state)
	if err := s.state.Put(ctx, currentStateKey, state); err != nil {
		return err
	}
	return s.replaceRoutes(ctx, state.Routes)
}

func (s *stateStore) LoadRoutes(ctx context.Context) ([]RouteRecord, error) {
	if s == nil || s.routes == nil {
		return nil, nil
	}
	routes := make([]RouteRecord, 0)
	err := s.routes.View(ctx, func(tx bboltx.ViewTx[string, RouteRecord]) error {
		return tx.ForEach(func(_ string, route RouteRecord) error {
			routes = append(routes, route)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return routes, nil
}

func (s *stateStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *stateStore) replaceRoutes(ctx context.Context, routes []RouteRecord) error {
	if s == nil || s.routes == nil {
		return nil
	}
	return s.routes.Update(ctx, func(tx bboltx.UpdateTx[string, RouteRecord]) error {
		existingKeys := make([]string, 0)
		if err := tx.ForEach(func(key string, _ RouteRecord) error {
			existingKeys = append(existingKeys, key)
			return nil
		}); err != nil {
			return err
		}
		if err := tx.DeleteMany(existingKeys...); err != nil {
			return err
		}
		for _, route := range routes {
			if route.Name == "" {
				continue
			}
			if err := tx.Put(route.Name, route); err != nil {
				return err
			}
		}
		return nil
	})
}
