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
	Secure                 SecureMiddlewareRuntime
	CORS                   CORSMiddlewareRuntime
	RateLimit              RateLimitRuntime
	CircuitBreaker         CircuitBreakerRuntime
}

type SecureMiddlewareRuntime struct {
	Enabled                         bool
	AllowedHosts                    *collectionlist.List[string]
	AllowedHostsAreRegex            bool
	SSLRedirect                     bool
	SSLHost                         string
	SSLTemporaryRedirect            bool
	STSSeconds                      int64
	STSIncludeSubdomains            bool
	STSPreload                      bool
	FrameDeny                       bool
	ContentTypeNosniff              bool
	BrowserXSSFilter                bool
	ContentSecurityPolicy           string
	ContentSecurityPolicyReportOnly string
	ReferrerPolicy                  string
	PermissionsPolicy               string
}

type CORSMiddlewareRuntime struct {
	Enabled              bool
	AllowedOrigins       *collectionlist.List[string]
	AllowedMethods       *collectionlist.List[string]
	AllowedHeaders       *collectionlist.List[string]
	ExposedHeaders       *collectionlist.List[string]
	MaxAge               int
	AllowCredentials     bool
	AllowPrivateNetwork  bool
	OptionsPassthrough   bool
	OptionsSuccessStatus int
}

type RateLimitRuntime struct {
	Enabled bool
	Rate    float64
	Burst   int
}

type CircuitBreakerRuntime struct {
	Enabled          bool
	MaxRequests      uint32
	Interval         string
	Timeout          string
	FailureThreshold uint32
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
