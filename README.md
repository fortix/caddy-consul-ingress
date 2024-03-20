# Caddy Consul Proxy

Plugin to use Caddy 2 as an ingress controller for a Nomad cluster, configuration is generated dynamically from consul.

The configuration is generated from tags placed against live services and custom routes loaded from a Key Value path.

## Configuration

| Environment Variable | Flag | Description |
| -------------------- | ---- | ----------- |
| CONSUL_INGRESS_TEMPLATE_FILE | --template | The template file to use to generate the Caddyfile, supports Go templates |
| CONSUL_INGRESS_CONSUL_ADDRESS | --consul-address | The address of the consul server, defaults to `http://localhost:8500` |
| CONSUL_INGRESS_CONSUL_TOKEN | --consul-token | The access token for Consul |
| CONSUL_INGRESS_URLPREFIX | --urlprefix | Only tags starting with this string are considered for service routing, defaults to `urlprefix-` |
| CONSUL_INGRESS_KV_PATH | --kvpath | The Key Value path to load custom routes from, defaults to `/caddy-routes` |
| CONSUL_INGRESS_POLLING_INTERVAL | --polling-interval | Rate to poll Consul at in seconds, defaults to `30` |
| CONSUL_INGRESS_WILDCARD_DOMAINS | --wildcard-domains | Space separated list of wildcard domains e.g. `*.example.com` |

### Default Template

The plugin uses the following default template to generate the Caddyfile, it can be replaced with the `--template` parameter:

```
{
  admin localhost:2019

  log {
    output stdout
    level INFO
    format console
  }
}

(reverseProxyConfig) {
  header_up +X_FORWARDED_PORT 443
  lb_policy least_conn
  lb_try_duration 5s
  lb_try_interval 250ms
  fail_duration 2s
  unhealthy_status 5xx
}

[[ range $domain, $serviceGroup := .wildcardServices ]]
[[ $domain ]] {
  encode zstd gzip

  [[ range $serviceIndex, $service := $serviceGroup.Services ]]
  @wildcard_[[ $serviceIndex ]] host [[ range $index, $element := $service.SrvUrls ]][[ if $index ]] [[ end ]][[ $element ]][[ end ]]
  handle @wildcard_[[ $serviceIndex ]] {
    reverse_proxy {
      [[ $service.To ]] [[ $service.Upstream ]]
      import reverseProxyConfig
      transport http {
        keepalive off
        [[ if $service.UseHttps ]]
        tls
        [[ if $service.SkipTlsVerify ]]tls_insecure_skip_verify[[ end ]]
        [[ end ]]
      }
    }
  }
  [[ end ]]

  handle {
    [[ if not $serviceGroup.Upstream ]]
    abort
    [[ else ]]
    reverse_proxy {
      [[ $serviceGroup.To ]] [[ $serviceGroup.Upstream ]]
      import reverseProxyConfig
      transport http {
        [[ if $service.UseHttps ]]
        tls
        [[ if $service.SkipTlsVerify ]]tls_insecure_skip_verify[[ end ]]
        [[ end ]]
      }
    }
    [[ end ]]
  }
}
[[ end ]]

[[ range $service := .services ]]
[[ range $index, $element := $service.SrvUrls ]][[ if $index ]] [[ end ]][[ $element ]][[ end ]] {
  encode zstd gzip

  reverse_proxy {
    [[ $service.To ]] [[ $service.Upstream ]]
    import reverseProxyConfig
    transport http {
      [[ if $service.UseHttps ]]
      tls
      [[ if $service.SkipTlsVerify ]]tls_insecure_skip_verify[[ end ]]
      [[ end ]]
    }
  }
}
[[ end ]]
```

## Building

```shell
xcaddy build --with github.com/fortix/caddy-consul-ingress
```

## Running

```shell
caddy consul-ingress
```

### Service Tags

Assuming `--urlprefix` is the default then the tag `urlprefix-www.example.com` will add a reverse proxy from the domain to the service connected to the tag.

For https based services `proto=https` can be added to the tag to indicate the service is https and `tlsskipverify=true` to skip SSL verification, e.g. `urlprefix-www.example.com proto=https tlsskipverify=true`

### Static Services

Static services are defined using Consul key value storage, by default the path `/caddy-routes` is read and all keys under it are considered.

A KV pair can hold multiple routes e.g.

```
kv1.example.com exampleservice1
kv2.example.com https://www.example.com proto=https tlsskipverify=true
```

Each line must start with the domain name and be followed by the service name or URL to go to.

For https based services `proto=https` can be added to the tag to indicate the service is https and `tlsskipverify=true` to skip SSL verification, e.g. `urlprefix-www.example.com proto=https tlsskipverify=true`

## Development

```shell
xcaddy consul-ingress --consul-address consul.service.consul:8500
```
