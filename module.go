package caddyconsulingress

import "github.com/caddyserver/caddy/v2"

func init() {
	caddy.RegisterModule(CaddyConsulIngress{})
}

// Caddy docker proxy module
type CaddyConsulIngress struct {
}

// CaddyModule returns the Caddy module information.
func (CaddyConsulIngress) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "consul_ingress",
		New: func() caddy.Module { return new(CaddyConsulIngress) },
	}
}
