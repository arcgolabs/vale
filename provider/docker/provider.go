package docker

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
	"github.com/samber/mo"
)

type Container struct {
	Name    string
	Address string
	Port    int
	Labels  map[string]string
}

type Source interface {
	ListContainers(context.Context) ([]Container, error)
	Watch(context.Context, func(), func(error)) (io.Closer, error)
}

type Provider struct {
	name    string
	source  Source
	options Options
}

type Options struct {
	DefaultEntrypointName string
	DefaultEntrypointAddr string
}

func DefaultOptions() Options {
	return Options{
		DefaultEntrypointName: "web",
		DefaultEntrypointAddr: ":8080",
	}
}

func New(name string, source Source, options Options) *Provider {
	if name == "" {
		name = "docker"
	}
	defaults := DefaultOptions()
	if options.DefaultEntrypointName == "" {
		options.DefaultEntrypointName = defaults.DefaultEntrypointName
	}
	if options.DefaultEntrypointAddr == "" {
		options.DefaultEntrypointAddr = defaults.DefaultEntrypointAddr
	}
	return &Provider{
		name:    name,
		source:  source,
		options: options,
	}
}

func NewFromEnv(name string, options Options) (*Provider, error) {
	source, err := NewDockerSourceFromEnv()
	if err != nil {
		return nil, err
	}
	return New(name, source, options), nil
}

func (p *Provider) Name() string {
	return p.name
}

func (p *Provider) Load(ctx context.Context) (*config.Config, error) {
	if p.source == nil {
		return nil, fmt.Errorf("docker provider source is nil")
	}
	containers, err := p.source.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	cfg := &config.Config{
		Entrypoints: []config.Entrypoint{
			{
				Name:    p.options.DefaultEntrypointName,
				Address: p.options.DefaultEntrypointAddr,
			},
		},
		Services: make([]config.Service, 0),
		Routes:   make([]config.Route, 0),
	}

	serviceMap := mapping.NewMap[string, *config.Service]()
	routeMap := mapping.NewMap[string, config.Route]()

	for _, container := range containers {
		labels := container.Labels
		if !parseBool(labels["vela.enable"], false) {
			continue
		}
		if container.Address == "" || container.Port <= 0 {
			continue
		}

		serviceName := valueOr(labels["vela.service"], sanitizeName(container.Name, "service"))
		routeName := valueOr(labels["vela.route"], serviceName+"-route")
		entrypoint := valueOr(labels["vela.entrypoint"], p.options.DefaultEntrypointName)
		host := strings.TrimSpace(labels["vela.rule.host"])
		pathPrefix := strings.TrimSpace(labels["vela.rule.pathprefix"])
		method := strings.TrimSpace(labels["vela.rule.method"])
		scheme := valueOr(labels["vela.scheme"], "http")
		weight := parseInt(labels["vela.weight"], 1)

		service, _ := serviceMap.GetOrCompute(serviceName, func() *config.Service {
			return &config.Service{
				Name:      serviceName,
				Strategy:  "round_robin",
				Endpoints: nil,
			}
		})
		service.Endpoints = append(service.Endpoints, config.Endpoint{
			URL:    fmt.Sprintf("%s://%s:%d", scheme, container.Address, container.Port),
			Weight: weight,
		})

		if _, exists := routeMap.Get(routeName); !exists {
			routeMap.Set(routeName, config.Route{
				Name:       routeName,
				Entrypoint: entrypoint,
				Service:    serviceName,
				Host:       host,
				PathPrefix: pathPrefix,
				Method:     method,
				Headers:    map[string]string{},
			})
		}
	}

	for _, serviceName := range provider.SortedStrings(serviceMap.Keys()) {
		service, _ := serviceMap.Get(serviceName)
		cfg.Services = append(cfg.Services, *service)
	}
	for _, routeName := range provider.SortedStrings(routeMap.Keys()) {
		route, _ := routeMap.Get(routeName)
		cfg.Routes = append(cfg.Routes, route)
	}

	if err := config.Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (p *Provider) Watch(ctx context.Context, onReload func(), onError func(error)) (io.Closer, error) {
	if p.source == nil {
		return nil, fmt.Errorf("docker provider source is nil")
	}
	return p.source.Watch(ctx, onReload, onError)
}

type MemorySource struct {
	mu         sync.RWMutex
	containers []Container
	listeners  *mapping.Map[int, func()]
	nextID     int
}

func NewMemorySource(containers ...Container) *MemorySource {
	return &MemorySource{
		containers: containers,
		listeners:  mapping.NewMap[int, func()](),
	}
}

func (s *MemorySource) ListContainers(_ context.Context) ([]Container, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Container, len(s.containers))
	copy(out, s.containers)
	return out, nil
}

func (s *MemorySource) Watch(_ context.Context, onReload func(), _ func(error)) (io.Closer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	s.listeners.Set(id, onReload)
	return provider.NewOnceCloser(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.listeners.Delete(id)
	}), nil
}

func (s *MemorySource) Update(containers ...Container) {
	s.mu.Lock()
	s.containers = containers
	listeners := s.listeners.Values()
	s.mu.Unlock()

	for _, listener := range listeners {
		if listener != nil {
			listener()
		}
	}
}

func valueOr(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func parseBool(value string, fallback bool) bool {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	return mo.TupleToOption(parsed, err == nil).OrElse(fallback)
}

func parseInt(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	return mo.TupleToOption(parsed, err == nil).OrElse(fallback)
}

func sanitizeName(input string, fallback string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return fallback
	}
	replacer := strings.NewReplacer("/", "-", "_", "-", " ", "-")
	return replacer.Replace(input)
}
