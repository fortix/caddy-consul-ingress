package caddyconsulingress

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/fortix/caddy-consul-ingress/config"

	"github.com/caddyserver/caddy/v2"
	caddycmd "github.com/caddyserver/caddy/v2/cmd"
	"go.uber.org/zap"
)

func init() {
	caddycmd.RegisterCommand(caddycmd.Command{
		Name:  "consul-ingress",
		Func:  commandFunc,
		Usage: "<command>",
		Short: "Run caddy as an ingress controller for a Consul / Nomad cluster",
		Flags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("consul-ingress", flag.ExitOnError)

			fs.String("template", "t", "A template file that the Caddyfile is generated from")
			fs.String("consul-address", "http://localhost:8500", "Address of the Consul server")
			fs.String("consul-token", "", "Access token for Consul")
			fs.String("urlprefix", "urlprefix-", "Prefix for the tags defining service URLs")
			fs.Duration("polling-interval", 30*time.Second, "Interval caddy should manually check consul for updated services")
			fs.String("kvpath", "/caddy-routes", "Path to the Consul KV store for custom routes")
			fs.String("wildcard-domains", "", "Space separated list of wildcard domains to group services by")
			fs.Bool("verbose", false, "Set the log level to debug")

			return fs
		}(),
	})
}

func commandFunc(flags caddycmd.Flags) (int, error) {
	caddy.TrapSignals()

	// Process options
	options := &config.Options{}

	if templateFileEnv := os.Getenv("CONSUL_INGRESS_TEMPLATE"); templateFileEnv != "" {
		options.TemplateFile = templateFileEnv
	} else {
		options.TemplateFile = flags.String("template")
	}

	if consulAddressEnv := os.Getenv("CONSUL_INGRESS_CONSUL_ADDRESS"); consulAddressEnv != "" {
		options.ConsulAddress = consulAddressEnv
	} else {
		options.ConsulAddress = flags.String("consul-address")
	}

	if consulTokenEnv := os.Getenv("CONSUL_INGRESS_CONSUL_TOKEN"); consulTokenEnv != "" {
		options.ConsulToken = consulTokenEnv
	} else {
		options.ConsulToken = flags.String("consul-token")
	}

	if urlPrefixEnv := os.Getenv("CONSUL_INGRESS_URLPREFIX"); urlPrefixEnv != "" {
		options.UrlPrefix = urlPrefixEnv
	} else {
		options.UrlPrefix = flags.String("urlprefix")
	}

	if kvPathEnv := os.Getenv("CONSUL_INGRESS_KV_PATH"); kvPathEnv != "" {
		options.KVPath = kvPathEnv
	} else {
		options.KVPath = flags.String("kvpath")
	}

	if wildcardDomainsEnv := os.Getenv("CONSUL_INGRESS_WILDCARD_DOMAINS"); wildcardDomainsEnv != "" {
		options.WildcardDomains = strings.Split(wildcardDomainsEnv, " ")
	} else {
		options.WildcardDomains = strings.Split(flags.String("wildcard-domains"), " ")
	}

	if verboseEnv := os.Getenv("CONSUL_INGRESS_VERBOSE"); verboseEnv != "" {
		options.Verbose = true
	} else {
		options.Verbose = flags.Bool("verbose")
	}

	options.Logger = caddy.Log().Named("consul-ingress")

	if pollingIntervalEnv := os.Getenv("CONSUL_INGRESS_POLLING_INTERVAL"); pollingIntervalEnv != "" {
		if p, err := time.ParseDuration(pollingIntervalEnv); err != nil {
			options.Logger.Error("Failed to parse CONSUL_INGRESS_POLLING_INTERVAL", zap.String("CONSUL_INGRESS_POLLING_INTERVAL", pollingIntervalEnv), zap.Error(err))
			options.PollingInterval = flags.Duration("polling-interval")
		} else {
			options.PollingInterval = p
		}
	} else {
		options.PollingInterval = flags.Duration("polling-interval")
	}

	options.Logger.Info("Start caddy admin")
	err := caddy.Run(&caddy.Config{
		Admin: &caddy.AdminConfig{
			Listen: "tcp/localhost:2019",
		},
	})
	if err != nil {
		return 1, err
	}

	client := NewConsulIngressClient(options)
	if err := client.Start(); err != nil {
		if err := caddy.Stop(); err != nil {
			return 1, err
		}

		return 1, err
	}

	select {}
}
