package provider

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/config"
	"github.com/samber/mo"
)

const (
	traefikRouterPrefix     = "traefik.http.routers."
	traefikServicePrefix    = "traefik.http.services."
	traefikMiddlewarePrefix = "traefik.http.middlewares."
)

// TraefikLabels is the normalized subset of Traefik HTTP dynamic labels Vale can project.
type TraefikLabels struct {
	Enabled     mo.Option[bool]
	Routers     *mapping.Map[string, TraefikRouter]
	Services    *mapping.Map[string, TraefikService]
	Middlewares *mapping.Map[string, config.Middleware]
}

// TraefikRouter contains router labels projected into Vale's HTTP route model.
type TraefikRouter struct {
	Name        string
	Rule        string
	Entrypoints *collectionlist.List[string]
	Middlewares *collectionlist.List[string]
	Service     string
	Host        string
	PathPrefix  string
	Method      string
	Headers     *mapping.Map[string, string]
}

// TraefikService contains load balancer labels projected into Vale services.
type TraefikService struct {
	Name   string
	Port   int
	Scheme string
}

// ParseTraefikLabels parses Traefik-compatible HTTP labels.
func ParseTraefikLabels(labels *mapping.Map[string, string]) TraefikLabels {
	result := NewTraefikLabels()
	normalizeTraefikLabels(labels).Range(func(key string, value string) bool {
		switch {
		case key == "traefik.enable":
			result.Enabled = mo.Some(parseTraefikBool(value))
		case strings.HasPrefix(key, traefikRouterPrefix):
			result.applyRouterLabel(strings.TrimPrefix(key, traefikRouterPrefix), value)
		case strings.HasPrefix(key, traefikServicePrefix):
			result.applyServiceLabel(strings.TrimPrefix(key, traefikServicePrefix), value)
		case strings.HasPrefix(key, traefikMiddlewarePrefix):
			result.applyMiddlewareLabel(strings.TrimPrefix(key, traefikMiddlewarePrefix), value)
		}
		return true
	})
	return result
}

// NewTraefikLabels returns an empty Traefik label projection.
func NewTraefikLabels() TraefikLabels {
	return TraefikLabels{
		Enabled:     mo.None[bool](),
		Routers:     mapping.NewMap[string, TraefikRouter](),
		Services:    mapping.NewMap[string, TraefikService](),
		Middlewares: mapping.NewMap[string, config.Middleware](),
	}
}

// HasHTTPConfig reports whether the labels define Traefik HTTP routing resources.
func (labels TraefikLabels) HasHTTPConfig() bool {
	return labels.Routers != nil && !labels.Routers.IsEmpty() ||
		labels.Services != nil && !labels.Services.IsEmpty() ||
		labels.Middlewares != nil && !labels.Middlewares.IsEmpty()
}

// StripTraefikProviderNamespace strips a Traefik provider suffix such as "auth@docker".
func StripTraefikProviderNamespace(name string) string {
	name = strings.TrimSpace(name)
	base, _, found := strings.Cut(name, "@")
	if found {
		return strings.TrimSpace(base)
	}
	return name
}

func (labels *TraefikLabels) applyRouterLabel(rest, value string) {
	name, option, ok := splitTraefikResourceLabel(rest)
	if !ok {
		return
	}
	labels.updateRouter(name, func(router *TraefikRouter) {
		switch option {
		case "rule":
			router.Rule = strings.TrimSpace(value)
			applyTraefikRule(router, value)
		case "entrypoints":
			router.Entrypoints = traefikCSVList(value, false)
		case "middlewares":
			router.Middlewares = traefikCSVList(value, true)
		case "service":
			router.Service = StripTraefikProviderNamespace(value)
		}
	})
}

func (labels *TraefikLabels) applyServiceLabel(rest, value string) {
	name, option, ok := splitTraefikResourceLabel(rest)
	if !ok {
		return
	}
	labels.updateService(name, func(service *TraefikService) {
		switch option {
		case "loadbalancer.server.port":
			service.Port = parseTraefikInt(value, 0)
		case "loadbalancer.server.scheme":
			service.Scheme = strings.TrimSpace(value)
		}
	})
}

func (labels *TraefikLabels) applyMiddlewareLabel(rest, value string) {
	name, option, ok := splitTraefikResourceLabel(rest)
	if !ok {
		return
	}
	labels.updateMiddleware(name, func(middleware *config.Middleware) {
		applyTraefikMiddlewareOption(middleware, option, value)
	})
}

func applyTraefikMiddlewareOption(middleware *config.Middleware, option, value string) {
	if applyTraefikPathMiddleware(middleware, option, value) {
		return
	}
	if applyTraefikRedirectMiddleware(middleware, option, value) {
		return
	}
	if applyTraefikHeaderMiddleware(middleware, option, value) {
		return
	}
	applyTraefikSecurityHeader(middleware, option, value)
}

func applyTraefikPathMiddleware(middleware *config.Middleware, option, value string) bool {
	switch option {
	case "addprefix.prefix":
		middleware.Type = "add_prefix"
		middleware.AddPrefix = strings.TrimSpace(value)
	case "stripprefix.prefixes":
		middleware.Type = "strip_prefix"
		middleware.StripPrefixes = SplitCSV(value).Values()
		middleware.StripPrefix = firstTraefikCSV(value)
	case "replacepath.path":
		middleware.ReplacePath = strings.TrimSpace(value)
	case "replacepathregex.regex":
		middleware.ReplacePathRegex = strings.TrimSpace(value)
	case "replacepathregex.replacement":
		middleware.ReplacePathReplacement = strings.TrimSpace(value)
	default:
		return false
	}
	return true
}

func applyTraefikRedirectMiddleware(middleware *config.Middleware, option, value string) bool {
	switch option {
	case "redirectscheme.scheme":
		middleware.RedirectScheme = strings.TrimSpace(value)
	case "redirectscheme.port":
		middleware.RedirectPort = strings.TrimSpace(value)
	case "redirectscheme.permanent":
		middleware.RedirectPermanent = parseTraefikBool(value)
	case "redirectregex.regex":
		middleware.RedirectRegex = strings.TrimSpace(value)
	case "redirectregex.replacement":
		middleware.RedirectReplacement = strings.TrimSpace(value)
	case "redirectregex.permanent":
		middleware.RedirectPermanent = parseTraefikBool(value)
	default:
		return false
	}
	return true
}

func applyTraefikHeaderMiddleware(middleware *config.Middleware, option, value string) bool {
	switch {
	case option == "chain.middlewares":
		middleware.Type = "chain"
		middleware.Chain = traefikCSVList(value, true).Values()
	case option == "buffering.maxrequestbodybytes":
		middleware.MaxBodyBytes = int64(parseTraefikInt(value, 0))
	case strings.HasPrefix(option, "headers.customrequestheaders."):
		setHeader(middleware.RequestHeaders, strings.TrimPrefix(option, "headers.customrequestheaders."), value)
	case strings.HasPrefix(option, "headers.customresponseheaders."):
		setHeader(middleware.ResponseHeaders, strings.TrimPrefix(option, "headers.customresponseheaders."), value)
	default:
		return false
	}
	return true
}

func (labels *TraefikLabels) updateRouter(name string, apply func(*TraefikRouter)) {
	name = strings.TrimSpace(name)
	if name == "" || labels == nil {
		return
	}
	router, _ := labels.Routers.Get(name)
	router.Name = name
	if router.Entrypoints == nil {
		router.Entrypoints = collectionlist.NewList[string]()
	}
	if router.Middlewares == nil {
		router.Middlewares = collectionlist.NewList[string]()
	}
	if router.Headers == nil {
		router.Headers = mapping.NewMap[string, string]()
	}
	apply(&router)
	labels.Routers.Set(name, router)
}

func (labels *TraefikLabels) updateService(name string, apply func(*TraefikService)) {
	name = strings.TrimSpace(name)
	if name == "" || labels == nil {
		return
	}
	service, _ := labels.Services.Get(name)
	service.Name = name
	apply(&service)
	labels.Services.Set(name, service)
}

func (labels *TraefikLabels) updateMiddleware(name string, apply func(*config.Middleware)) {
	name = strings.TrimSpace(name)
	if name == "" || labels == nil {
		return
	}
	middleware, _ := labels.Middlewares.Get(name)
	middleware.Name = name
	if middleware.RequestHeaders == nil {
		middleware.RequestHeaders = map[string]string{}
	}
	if middleware.ResponseHeaders == nil {
		middleware.ResponseHeaders = map[string]string{}
	}
	apply(&middleware)
	labels.Middlewares.Set(name, middleware)
}
