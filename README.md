# Caddy Consul Proxy

Plugin to use Caddy 2 as a proxy into a Nomad cluster, configuration is dynamic from consul.

Uses consul tags with a prefix e.g. urlprefix- so that it supports a subset of fabio configuration.

## Development

```shell
~/go/bin/xcaddy run --config ./Caddyfile
```
