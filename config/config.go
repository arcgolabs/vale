package config

type Config struct {
	Entrypoints   []Entrypoint   `hcl:"entrypoint,block"`
	Services      []Service      `hcl:"service,block"`
	Middlewares   []Middleware   `hcl:"middleware,block"`
	Routes        []Route        `hcl:"route,block"`
	Admin         *Admin         `hcl:"admin,block"`
	Observability *Observability `hcl:"observability,block"`
	Health        *Health        `hcl:"health,block"`
	Security      *Security      `hcl:"security,block"`
}

type Entrypoint struct {
	Name    string          `hcl:",label"`
	Address string          `hcl:"address"`
	TLS     *EntrypointTLS  `hcl:"tls,block"`
	ACME    *EntrypointACME `hcl:"acme,block"`
}

type EntrypointTLS struct {
	Enabled  bool   `hcl:"enabled,optional"`
	CertFile string `hcl:"cert_file,optional"`
	KeyFile  string `hcl:"key_file,optional"`
}

type EntrypointACME struct {
	Enabled  bool     `hcl:"enabled,optional"`
	Email    string   `hcl:"email,optional"`
	CacheDir string   `hcl:"cache_dir,optional"`
	Domains  []string `hcl:"domains,optional"`
}

type Service struct {
	Name      string     `hcl:",label"`
	Strategy  string     `hcl:"strategy,optional"`
	Endpoints []Endpoint `hcl:"endpoint,block"`
}

type Endpoint struct {
	URL    string `hcl:"url"`
	Weight int    `hcl:"weight,optional"`
}

type Route struct {
	Name        string            `hcl:",label"`
	Entrypoint  string            `hcl:"entrypoint"`
	Service     string            `hcl:"service"`
	Host        string            `hcl:"host,optional"`
	PathPrefix  string            `hcl:"path_prefix,optional"`
	Method      string            `hcl:"method,optional"`
	Headers     map[string]string `hcl:"headers,optional"`
	Middlewares []string          `hcl:"middlewares,optional"`
}

type Middleware struct {
	Name                   string            `hcl:",label"`
	Type                   string            `hcl:"type,optional"`
	StripPrefix            string            `hcl:"strip_prefix,optional"`
	StripPrefixes          []string          `hcl:"strip_prefixes,optional"`
	AddPrefix              string            `hcl:"add_prefix,optional"`
	ReplacePath            string            `hcl:"replace_path,optional"`
	ReplacePathRegex       string            `hcl:"replace_path_regex,optional"`
	ReplacePathReplacement string            `hcl:"replace_path_replacement,optional"`
	RedirectScheme         string            `hcl:"redirect_scheme,optional"`
	RedirectPort           string            `hcl:"redirect_port,optional"`
	RedirectRegex          string            `hcl:"redirect_regex,optional"`
	RedirectReplacement    string            `hcl:"redirect_replacement,optional"`
	RedirectPermanent      bool              `hcl:"redirect_permanent,optional"`
	RequestHeaders         map[string]string `hcl:"request_headers,optional"`
	ResponseHeaders        map[string]string `hcl:"response_headers,optional"`
	MaxBodyBytes           int64             `hcl:"max_body_bytes,optional"`
	Chain                  []string          `hcl:"chain,optional"`
	Secure                 *SecureMiddleware `hcl:"secure,block"`
	CORS                   *CORSMiddleware   `hcl:"cors,block"`
	RateLimit              *RateLimit        `hcl:"rate_limit,block"`
	CircuitBreaker         *CircuitBreaker   `hcl:"circuit_breaker,block"`
	BasicAuth              *BasicAuth        `hcl:"basic_auth,block"`
	Compress               *Compress         `hcl:"compress,block"`
	IPAllowList            *IPAllowList      `hcl:"ip_allow_list,block"`
	ForwardAuth            *ForwardAuth      `hcl:"forward_auth,block"`
}

type SecureMiddleware struct {
	Enabled                         bool     `hcl:"enabled,optional"`
	AllowedHosts                    []string `hcl:"allowed_hosts,optional"`
	AllowedHostsAreRegex            bool     `hcl:"allowed_hosts_are_regex,optional"`
	SSLRedirect                     bool     `hcl:"ssl_redirect,optional"`
	SSLHost                         string   `hcl:"ssl_host,optional"`
	SSLTemporaryRedirect            bool     `hcl:"ssl_temporary_redirect,optional"`
	STSSeconds                      int64    `hcl:"sts_seconds,optional"`
	STSIncludeSubdomains            bool     `hcl:"sts_include_subdomains,optional"`
	STSPreload                      bool     `hcl:"sts_preload,optional"`
	FrameDeny                       bool     `hcl:"frame_deny,optional"`
	ContentTypeNosniff              bool     `hcl:"content_type_nosniff,optional"`
	BrowserXSSFilter                bool     `hcl:"browser_xss_filter,optional"`
	ContentSecurityPolicy           string   `hcl:"content_security_policy,optional"`
	ContentSecurityPolicyReportOnly string   `hcl:"content_security_policy_report_only,optional"`
	ReferrerPolicy                  string   `hcl:"referrer_policy,optional"`
	PermissionsPolicy               string   `hcl:"permissions_policy,optional"`
}

type CORSMiddleware struct {
	Enabled              bool     `hcl:"enabled,optional"`
	AllowedOrigins       []string `hcl:"allowed_origins,optional"`
	AllowedMethods       []string `hcl:"allowed_methods,optional"`
	AllowedHeaders       []string `hcl:"allowed_headers,optional"`
	ExposedHeaders       []string `hcl:"exposed_headers,optional"`
	MaxAge               int      `hcl:"max_age,optional"`
	AllowCredentials     bool     `hcl:"allow_credentials,optional"`
	AllowPrivateNetwork  bool     `hcl:"allow_private_network,optional"`
	OptionsPassthrough   bool     `hcl:"options_passthrough,optional"`
	OptionsSuccessStatus int      `hcl:"options_success_status,optional"`
}

type RateLimit struct {
	Enabled bool    `hcl:"enabled,optional"`
	Rate    float64 `hcl:"rate,optional"`
	Burst   int     `hcl:"burst,optional"`
}

type CircuitBreaker struct {
	Enabled          bool   `hcl:"enabled,optional"`
	MaxRequests      uint32 `hcl:"max_requests,optional"`
	Interval         string `hcl:"interval,optional"`
	Timeout          string `hcl:"timeout,optional"`
	FailureThreshold uint32 `hcl:"failure_threshold,optional"`
}

type BasicAuth struct {
	Enabled bool              `hcl:"enabled,optional"`
	Realm   string            `hcl:"realm,optional"`
	Users   map[string]string `hcl:"users,optional"`
}

type Compress struct {
	Enabled  bool `hcl:"enabled,optional"`
	MinBytes int  `hcl:"min_bytes,optional"`
}

type IPAllowList struct {
	Enabled            bool     `hcl:"enabled,optional"`
	SourceRange        []string `hcl:"source_range,optional"`
	TrustForwardHeader bool     `hcl:"trust_forward_header,optional"`
}

type ForwardAuth struct {
	Enabled              bool     `hcl:"enabled,optional"`
	Address              string   `hcl:"address"`
	Timeout              string   `hcl:"timeout,optional"`
	TrustForwardHeader   bool     `hcl:"trust_forward_header,optional"`
	ForwardBody          bool     `hcl:"forward_body,optional"`
	MaxBodyBytes         int64    `hcl:"max_body_bytes,optional"`
	AuthRequestHeaders   []string `hcl:"auth_request_headers,optional"`
	AuthResponseHeaders  []string `hcl:"auth_response_headers,optional"`
	MaxResponseBodyBytes int64    `hcl:"max_response_body_bytes,optional"`
}

type Admin struct {
	Address string `hcl:"address"`
}

type Observability struct {
	AccessLog bool `hcl:"access_log,optional"`
	Metrics   bool `hcl:"metrics,optional"`
}

type Health struct {
	Interval string `hcl:"interval,optional"`
	Timeout  string `hcl:"timeout,optional"`
}

type Security struct {
	ReadHeaderTimeout string `hcl:"read_header_timeout,optional"`
	ReadTimeout       string `hcl:"read_timeout,optional"`
	WriteTimeout      string `hcl:"write_timeout,optional"`
	IdleTimeout       string `hcl:"idle_timeout,optional"`
	MaxHeaderBytes    int    `hcl:"max_header_bytes,optional"`
	MaxBodyBytes      int64  `hcl:"max_body_bytes,optional"`
}
