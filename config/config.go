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
	Name            string            `hcl:",label"`
	Type            string            `hcl:"type,optional"`
	StripPrefix     string            `hcl:"strip_prefix,optional"`
	AddPrefix       string            `hcl:"add_prefix,optional"`
	RequestHeaders  map[string]string `hcl:"request_headers,optional"`
	ResponseHeaders map[string]string `hcl:"response_headers,optional"`
	MaxBodyBytes    int64             `hcl:"max_body_bytes,optional"`
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
