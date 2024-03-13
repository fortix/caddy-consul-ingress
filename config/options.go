package config

import "time"

// Options are the options for generator
type Options struct {
	Caddyfile       string
	ConsulAddress   string
	ConsulToken     string
	UrlPrefix       string
	TemplateFile    string
	PollingInterval time.Duration
}
