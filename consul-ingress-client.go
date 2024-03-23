package caddyconsulingress

import (
	"crypto/md5"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fortix/caddy-consul-ingress/config"
	"github.com/fortix/caddy-consul-ingress/generator"
	"github.com/fortix/caddy-consul-ingress/parser"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	consul "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
)

var CaddyfileAutosavePath = filepath.Join(caddy.AppConfigDir(), "Caddyfile.autosave")

type ConsulIngressClient struct {
	mutex             sync.Mutex
	options           *config.Options
	logger            *zap.Logger
	parser            *parser.ServiceParser
	generator         *generator.CaddyfileGenerator
	lastCaddyfileHash string
	serviceDefs       *parser.Services
	kvServiceDefs     *parser.Services
}

func NewConsulIngressClient(options *config.Options) *ConsulIngressClient {
	return &ConsulIngressClient{
		mutex:             sync.Mutex{},
		options:           options,
		logger:            options.Logger,
		parser:            parser.NewParser(options.Logger, options),
		generator:         generator.NewGenerator(options.Logger, options),
		lastCaddyfileHash: "",
		serviceDefs:       nil,
		kvServiceDefs:     nil,
	}
}

func (ingressClient *ConsulIngressClient) Start() error {
	ingressClient.logger.Info("Starting Consul Ingress Client")

	consulConfig := &consul.Config{
		Address: ingressClient.options.ConsulAddress,
		Token:   ingressClient.options.ConsulToken,
	}

	// Start a goroutine to watch for changes in Consul services
	ingressClient.logger.Info("Watch for changes in Consul services")
	go func() {
		params := &consul.QueryOptions{
			WaitIndex:         0,
			WaitTime:          ingressClient.options.PollingInterval,
			AllowStale:        false,
			RequireConsistent: true,
		}

		for {
			consulClient, err := consul.NewClient(consulConfig)
			if err != nil {
				ingressClient.logger.Warn("Failed to create Consul client", zap.Error(err))
				time.Sleep(5 * time.Second) // Wait before attempting reconnection
				continue
			}

			for {
				services, meta, err := consulClient.Catalog().Services(params)
				if err != nil {
					ingressClient.logger.Error("Failed to retrieve services from Consul", zap.Error(err))
					break
				}

				if meta.LastIndex > params.WaitIndex {
					params.WaitIndex = meta.LastIndex

					ingressClient.serviceDefs = ingressClient.parser.ParseServices(services)

					ingressClient.updateCaddyfile(ingressClient.logger)
				}
			}

			// Connection to Consul lost, attempt reconnection
			ingressClient.logger.Warn("Connection to Consul lost, attempting reconnection...")
			time.Sleep(5 * time.Second) // Wait before attempting reconnection
		}
	}()

	// Start a goroutine to watch for changes in Consul KV store
	if ingressClient.options.KVPath != "" {
		ingressClient.logger.Info("Watch for changes in Consul Key Value store")
		go func() {
			params := &consul.QueryOptions{
				WaitIndex:         0,
				WaitTime:          ingressClient.options.PollingInterval,
				AllowStale:        false,
				RequireConsistent: true,
			}

			for {
				consulClient, err := consul.NewClient(consulConfig)
				if err != nil {
					ingressClient.logger.Warn("Failed to create Consul client", zap.Error(err))
					time.Sleep(5 * time.Second) // Wait before attempting reconnection
					continue
				}

				for {
					kvPairs, meta, err := consulClient.KV().List(ingressClient.options.KVPath, params)
					if err != nil {
						ingressClient.logger.Error("Failed to retrieve KV pairs from Consul", zap.Error(err))
						break
					}

					if meta.LastIndex > params.WaitIndex {
						params.WaitIndex = meta.LastIndex

						ingressClient.kvServiceDefs = ingressClient.parser.ParseKV(&kvPairs)

						ingressClient.updateCaddyfile(ingressClient.logger)
					}
				}

				// Connection to Consul lost, attempt reconnection
				ingressClient.logger.Warn("Connection to Consul lost, attempting reconnection...")
				time.Sleep(5 * time.Second) // Wait before attempting reconnection
			}
		}()
	}

	return nil
}

func (ingressClient *ConsulIngressClient) updateCaddyfile(log *zap.Logger) {

	// Acquire the lock
	ingressClient.mutex.Lock()
	defer ingressClient.mutex.Unlock()

	// Generate Caddyfile from services
	caddyfile := ingressClient.generator.Generate(ingressClient.serviceDefs, ingressClient.kvServiceDefs)

	// Calculate md5 hash of the generated Caddyfile
	md5Hash := md5.New()
	md5Hash.Write([]byte(caddyfile))
	caddyfileHash := string(md5Hash.Sum(nil))

	if ingressClient.lastCaddyfileHash != string(caddyfileHash) {
		ingressClient.lastCaddyfileHash = caddyfileHash

		// Save the generated Caddyfile to disk
		if autosaveErr := os.WriteFile(CaddyfileAutosavePath, []byte(caddyfile), 0666); autosaveErr != nil {
			log.Warn("Failed to autosave caddyfile", zap.Error(autosaveErr), zap.String("path", CaddyfileAutosavePath))
		}

		// Convert the Caddyfile to JSON
		adapter := caddyconfig.GetAdapter("caddyfile")
		json, warn, err := adapter.Adapt([]byte(caddyfile), nil)
		if err != nil {
			log.Error("Failed to adapt Caddyfile", zap.Error(err))
			return
		}

		if ingressClient.options.Verbose {
			log.Info("Caddyfile", zap.String("caddyfile", string(json)))

			if warn != nil {
				log.Warn("Warnings", zap.Any("warnings", warn))
			}
		}

		// Restart Caddy if the Caddyfile has changed
		if ingressClient.options.RestartOnCfgChange {
			// Restart caddy to break in flight connections
			log.Info("Restarting Caddy")
			caddy.Stop()
			err = caddy.Run(&caddy.Config{
				Admin: &caddy.AdminConfig{
					Listen: "tcp/localhost:2019",
				},
			})
			if err != nil {
				log.Fatal("Failed to start Caddy", zap.Error(err))
			}
		}

		// Load the JSON into Caddy
		err = caddy.Load(json, false)
		if err != nil {
			log.Error("Failed to load Caddyfile", zap.Error(err))
			log.Error(caddyfile)
		} else {
			log.Info("Successfully loaded Caddyfile")
		}
	} else {
		log.Info("Caddyfile has not changed, skipping reload")
	}
}
