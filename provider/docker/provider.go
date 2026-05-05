package docker

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/arcgolabs/vela/config"
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

	serviceMap := make(map[string]*config.Service)
	routeMap := make(map[string]config.Route)

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

		service, exists := serviceMap[serviceName]
		if !exists {
			service = &config.Service{
				Name:      serviceName,
				Strategy:  "round_robin",
				Endpoints: make([]config.Endpoint, 0),
			}
			serviceMap[serviceName] = service
		}
		service.Endpoints = append(service.Endpoints, config.Endpoint{
			URL:    fmt.Sprintf("%s://%s:%d", scheme, container.Address, container.Port),
			Weight: weight,
		})

		if _, exists := routeMap[routeName]; !exists {
			routeMap[routeName] = config.Route{
				Name:       routeName,
				Entrypoint: entrypoint,
				Service:    serviceName,
				Host:       host,
				PathPrefix: pathPrefix,
				Method:     method,
				Headers:    map[string]string{},
			}
		}
	}

	for _, serviceName := range sortedKeys(serviceMap) {
		cfg.Services = append(cfg.Services, *serviceMap[serviceName])
	}
	for _, routeName := range sortedRouteKeys(routeMap) {
		cfg.Routes = append(cfg.Routes, routeMap[routeName])
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
	listeners  map[int]func()
	nextID     int
}

func NewMemorySource(containers ...Container) *MemorySource {
	return &MemorySource{
		containers: containers,
		listeners:  make(map[int]func()),
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
	s.listeners[id] = onReload
	return &memoryWatchCloser{
		closeFn: func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(s.listeners, id)
		},
	}, nil
}

func (s *MemorySource) Update(containers ...Container) {
	s.mu.Lock()
	s.containers = containers
	listeners := make([]func(), 0, len(s.listeners))
	for _, listener := range s.listeners {
		listeners = append(listeners, listener)
	}
	s.mu.Unlock()

	for _, listener := range listeners {
		if listener != nil {
			listener()
		}
	}
}

type memoryWatchCloser struct {
	once    sync.Once
	closeFn func()
}

func (c *memoryWatchCloser) Close() error {
	c.once.Do(func() {
		if c.closeFn != nil {
			c.closeFn()
		}
	})
	return nil
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
	if err != nil {
		return fallback
	}
	return parsed
}

func parseInt(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func sanitizeName(input string, fallback string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return fallback
	}
	replacer := strings.NewReplacer("/", "-", "_", "-", " ", "-")
	return replacer.Replace(input)
}

func sortedKeys(source map[string]*config.Service) []string {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedRouteKeys(source map[string]config.Route) []string {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
