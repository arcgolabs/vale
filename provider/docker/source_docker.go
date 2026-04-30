package docker

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

type DockerSource struct {
	client *client.Client
}

func NewDockerSource(cli *client.Client) *DockerSource {
	return &DockerSource{client: cli}
}

func NewDockerSourceFromEnv() (*DockerSource, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerSource{client: cli}, nil
}

func (s *DockerSource) ListContainers(ctx context.Context) ([]Container, error) {
	list, err := s.client.ContainerList(ctx, container.ListOptions{
		All: false,
	})
	if err != nil {
		return nil, err
	}

	result := make([]Container, 0, len(list))
	for _, item := range list {
		address, port := dockerAddressPort(item)
		if address == "" || port == 0 {
			continue
		}
		result = append(result, Container{
			Name:    sanitizeContainerName(item.Names),
			Address: address,
			Port:    port,
			Labels:  item.Labels,
		})
	}
	return result, nil
}

func (s *DockerSource) Watch(ctx context.Context, onReload func(), onError func(error)) (io.Closer, error) {
	watchCtx, cancelWatch := context.WithCancel(ctx)
	filter := filters.NewArgs()
	filter.Add("type", "container")
	filter.Add("event", "start")
	filter.Add("event", "die")
	filter.Add("event", "stop")
	filter.Add("event", "destroy")
	filter.Add("event", "health_status")

	eventsCh, errCh := s.client.Events(watchCtx, events.ListOptions{Filters: filter})
	done := make(chan struct{})
	var once sync.Once

	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-eventsCh:
				if !ok {
					return
				}
				onReload()
			case err, ok := <-errCh:
				if !ok {
					return
				}
				if err != nil && !strings.Contains(err.Error(), "context canceled") {
					onError(err)
				}
				return
			}
		}
	}()

	return &watchCloser{
		closeFn: func() {
			once.Do(func() {
				cancelWatch()
				<-done
			})
		},
	}, nil
}

func dockerAddressPort(item container.Summary) (string, int) {
	for _, p := range item.Ports {
		if p.PublicPort > 0 {
			return "127.0.0.1", int(p.PublicPort)
		}
		if p.PrivatePort > 0 {
			// Keep simple default mapping for local docker source.
			return "127.0.0.1", int(p.PrivatePort)
		}
	}
	return "", 0
}

func sanitizeContainerName(names []string) string {
	if len(names) == 0 {
		return "container"
	}
	name := names[0]
	name = strings.TrimPrefix(name, "/")
	if strings.TrimSpace(name) == "" {
		return "container"
	}
	return name
}

type watchCloser struct {
	once    sync.Once
	closeFn func()
}

func (c *watchCloser) Close() error {
	c.once.Do(func() {
		if c.closeFn != nil {
			c.closeFn()
		}
	})
	return nil
}
