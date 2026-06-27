package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/runtime"
)

type tcpServer struct {
	entrypoint string
	address    string
	listener   net.Listener
	runtime    *runtime.Gateway
	logger     *slog.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	wg         sync.WaitGroup
	conns      *mapping.Map[net.Conn, struct{}]
}

func newTCPServer(ctx context.Context, entrypoint, address string, runtimeGateway *runtime.Gateway, logger *slog.Logger) (*tcpServer, error) {
	listener, err := listenTCP(ctx, address)
	if err != nil {
		return nil, err
	}
	serverCtx, cancel := context.WithCancel(ctx)
	return &tcpServer{
		entrypoint: entrypoint,
		address:    address,
		listener:   listener,
		runtime:    runtimeGateway,
		logger:     logger,
		ctx:        serverCtx,
		cancel:     cancel,
		conns:      mapping.NewMap[net.Conn, struct{}](),
	}, nil
}

func (s *tcpServer) Serve() {
	if s == nil || s.listener == nil {
		return
	}
	if s.logger != nil {
		s.logger.Info("tcp entrypoint started", "entrypoint", s.entrypoint, "addr", s.address)
	}
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || s.ctx.Err() != nil {
				return
			}
			if s.logger != nil {
				s.logger.Error("tcp entrypoint accept failed", "entrypoint", s.entrypoint, "addr", s.address, "error", err)
			}
			continue
		}
		s.track(conn)
		s.wg.Add(1)
		go s.proxy(conn)
	}
}

func (s *tcpServer) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	var err error
	if s.listener != nil {
		err = s.listener.Close()
	}
	s.closeActiveConnections()
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return err
	}
}

func (s *tcpServer) proxy(conn net.Conn) {
	defer s.wg.Done()
	defer s.untrack(conn)
	if s.runtime == nil {
		return
	}
	s.runtime.ProxyTCP(s.ctx, s.entrypoint, conn)
}

func (s *tcpServer) track(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns.Set(conn, struct{}{})
}

func (s *tcpServer) untrack(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns.Delete(conn)
}

func (s *tcpServer) closeActiveConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns.Range(func(conn net.Conn, _ struct{}) bool {
		_ = conn.Close()
		return true
	})
}
