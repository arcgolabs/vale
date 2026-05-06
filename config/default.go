package config

func Default() *Config {
	return &Config{
		Entrypoints: []Entrypoint{
			{Name: "web", Address: ":8080"},
		},
		Services: []Service{
			{
				Name:     "echo",
				Strategy: "round_robin",
				Endpoints: []Endpoint{
					{URL: "http://127.0.0.1:8081", Weight: 1},
				},
			},
		},
		Routes: []Route{
			{
				Name:       "echo-route",
				Entrypoint: "web",
				Service:    "echo",
				PathPrefix: "/",
			},
		},
		Admin: &Admin{Address: ":19090"},
		Observability: &Observability{
			AccessLog: true,
			Metrics:   true,
		},
		Health: &Health{
			Interval: "5s",
			Timeout:  "2s",
		},
	}
}
