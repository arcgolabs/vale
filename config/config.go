package config

type Config struct {
	Entrypoints   []Entrypoint   `hcl:"entrypoint,block"`
	Services      []Service      `hcl:"service,block"`
	Routes        []Route        `hcl:"route,block"`
	ProxyEngine   string         `hcl:"proxy_engine,optional"`
	Admin         *Admin         `hcl:"admin,block"`
	Observability *Observability `hcl:"observability,block"`
	Health        *Health        `hcl:"health,block"`
}

type Entrypoint struct {
	Name    string `hcl:",label"`
	Address string `hcl:"address"`
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
	Name       string            `hcl:",label"`
	Entrypoint string            `hcl:"entrypoint"`
	Service    string            `hcl:"service"`
	Host       string            `hcl:"host,optional"`
	PathPrefix string            `hcl:"path_prefix,optional"`
	Method     string            `hcl:"method,optional"`
	Headers    map[string]string `hcl:"headers,optional"`
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
