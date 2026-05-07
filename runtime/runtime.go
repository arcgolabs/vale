package runtime

import (
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/arcgolabs/collectionx/bitset"
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

type CompiledSnapshot struct {
	Entrypoints        *mapping.Map[string, string]
	EntrypointConfigs  *mapping.Map[string, EntrypointRuntime]
	RoutesByEntrypoint *mapping.MultiMap[string, *CompiledRoute]
	EntrypointMatchers *mapping.Map[string, *EntrypointMatcher]
	Catalog            *Catalog
	Services           *mapping.Map[string, *ServiceRuntime]
	AdminAddress       string
	AccessLogEnabled   bool
	MetricsEnabled     bool
	HealthInterval     string
	HealthTimeout      string
	Security           SecurityRuntime
	ProxyEngine        string
	BuiltAt            time.Time
}

type CompiledRoute struct {
	Name        string
	Entrypoint  string
	Host        string
	PathPrefix  string
	Method      string
	Headers     *mapping.Map[string, string]
	Service     *ServiceRuntime
	Predicates  *bitset.BitSet
	Middlewares *collectionlist.List[MiddlewareRuntime]
}

type EntrypointRuntime struct {
	Name    string
	Address string
	TLS     TLSRuntime
}

type TLSRuntime struct {
	Enabled  bool
	CertFile string
	KeyFile  string
	ACME     ACMERuntime
}

type ACMERuntime struct {
	Enabled  bool
	Email    string
	CacheDir string
	Domains  *collectionlist.List[string]
}

type SecurityRuntime struct {
	ReadHeaderTimeout string
	ReadTimeout       string
	WriteTimeout      string
	IdleTimeout       string
	MaxHeaderBytes    int
	MaxBodyBytes      int64
}

type MiddlewareRuntime struct {
	Name                   string
	Type                   string
	StripPrefix            string
	StripPrefixes          *collectionlist.List[string]
	AddPrefix              string
	ReplacePath            string
	ReplacePathRegex       string
	ReplacePathReplacement string
	RedirectScheme         string
	RedirectPort           string
	RedirectRegex          string
	RedirectReplacement    string
	RedirectPermanent      bool
	RequestHeaders         *mapping.Map[string, string]
	ResponseHeaders        *mapping.Map[string, string]
	MaxBodyBytes           int64
	Chain                  *collectionlist.List[string]
}

type ServiceRuntime struct {
	Name          string
	Strategy      string
	Endpoints     *collectionlist.List[*EndpointRuntime]
	weightedSlots *collectionlist.List[int]
	rrCounter     atomic.Uint64
}

type EndpointRuntime struct {
	URL         *url.URL
	Weight      int
	Proxy       http.Handler
	Healthy     atomic.Bool
	LastChecked atomic.Int64
}

type Gateway struct {
	current            atomic.Pointer[CompiledSnapshot]
	access             *AccessLogger
	metrics            MetricsRecorder
	middlewareRegistry *MiddlewareRegistry
}
