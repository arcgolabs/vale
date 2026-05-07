package docker

import (
	"context"
	"io"
	"strings"

	"github.com/arcgolabs/vela/provider"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/samber/oops"
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
		return nil, oops.In("docker_source").Wrapf(err, "create docker client")
	}
	return &DockerSource{client: cli}, nil
}

func (s *DockerSource) ListContainers(ctx context.Context) ([]Container, error) {
	list, err := s.client.ContainerList(ctx, container.ListOptions{
		All: false,
	})
	if err != nil {
		return nil, oops.In("docker_source").Wrapf(err, "list docker containers")
	}

	result := make([]Container, 0, len(list))
	for index := range list {
		item := &list[index]
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

	go watchDockerEvents(ctx, eventsCh, errCh, onReload, onError, done)

	return provider.NewOnceCloser(func() {
		cancelWatch()
		<-done
	}), nil
}

func watchDockerEvents(
	ctx context.Context,
	eventsCh <-chan events.Message,
	errCh <-chan error,
	onReload func(),
	onError func(error),
	done chan<- struct{},
) {
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
			handleDockerWatchError(err, ok, onError)
			return
		}
	}
}

func handleDockerWatchError(err error, ok bool, onError func(error)) {
	if !ok || err == nil || strings.Contains(err.Error(), "context canceled") {
		return
	}
	onError(err)
}

func dockerAddressPort(item *container.Summary) (string, int) {
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
