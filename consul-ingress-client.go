package caddyconsulingress

import (
	"bytes"
	"crypto/md5"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fortix/caddy-consul-ingress/config"
	"github.com/fortix/caddy-consul-ingress/generator"
	"github.com/fortix/caddy-consul-ingress/parser"

	"github.com/caddyserver/caddy/v2"
	consul "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
)

var CaddyfileAutosavePath = filepath.Join(caddy.AppConfigDir(), "Caddyfile.autosave")

type ConsulIngressClient struct {
	mutex             sync.Mutex
	options           *config.Options
	parser            *parser.ServiceParser
	generator         *generator.CaddyfileGenerator
	lastCaddyfileHash string
	serviceDefs       []*parser.ServiceDef
	kvServiceDefs     []*parser.ServiceDef
}

func NewConsulIngressClient(options *config.Options) *ConsulIngressClient {
	return &ConsulIngressClient{
		mutex:             sync.Mutex{},
		options:           options,
		parser:            parser.NewParser(logger(), options),
		generator:         generator.NewGenerator(logger(), options),
		lastCaddyfileHash: "",
		serviceDefs:       []*parser.ServiceDef{},
		kvServiceDefs:     []*parser.ServiceDef{},
	}
}

func (ingressClient *ConsulIngressClient) Start() error {
	log := logger()

	log.Info("Starting Consul Ingress Client")

	consulConfig := &consul.Config{
		Address: ingressClient.options.ConsulAddress,
		Token:   ingressClient.options.ConsulToken,
	}

	// Start a goroutine to watch for changes in Consul services
	log.Info("Watch for changes in Consul services")
	go func() {
		for {
			consulClient, err := consul.NewClient(consulConfig)
			if err != nil {
				log.Warn("Failed to create Consul client", zap.Error(err))
				time.Sleep(5 * time.Second) // Wait before attempting reconnection
				continue
			}

			params := &consul.QueryOptions{
				WaitIndex: 0,
				WaitTime:  ingressClient.options.PollingInterval,
			}

			for {
				services, meta, err := consulClient.Catalog().Services(params)
				if err != nil {
					log.Error("Failed to retrieve services from Consul", zap.Error(err))
					break
				}

				if meta.LastIndex > params.WaitIndex {
					params.WaitIndex = meta.LastIndex

					// Parse the services and their tags
					ingressClient.serviceDefs = ingressClient.parser.ParseService(services)

					// Output the list of services
					for _, serviceDef := range ingressClient.serviceDefs {
						log.Info("Service", zap.String("to", serviceDef.To), zap.String("service", serviceDef.Upstream), zap.Strings("srvUrls", serviceDef.SrvUrls), zap.Bool("useHttps", serviceDef.UseHttps), zap.Bool("skipTlsVerify", serviceDef.SkipTlsVerify))
					}

					ingressClient.updateCaddyfile(log)
				}
			}

			// Connection to Consul lost, attempt reconnection
			log.Warn("Connection to Consul lost, attempting reconnection...")
			time.Sleep(5 * time.Second) // Wait before attempting reconnection
		}
	}()

	// Start a goroutine to watch for changes in Consul KV store
	if ingressClient.options.KVPath != "" {
		log.Info("Watch for changes in Consul Key Value store")
		go func() {
			for {
				consulClient, err := consul.NewClient(consulConfig)
				if err != nil {
					log.Warn("Failed to create Consul client", zap.Error(err))
					time.Sleep(5 * time.Second) // Wait before attempting reconnection
					continue
				}

				params := &consul.QueryOptions{
					WaitIndex: 0,
					WaitTime:  ingressClient.options.PollingInterval,
				}

				for {
					kvPairs, meta, err := consulClient.KV().List(ingressClient.options.KVPath, params)
					if err != nil {
						log.Error("Failed to retrieve KV pairs from Consul", zap.Error(err))
						break
					}

					if meta.LastIndex > params.WaitIndex {
						params.WaitIndex = meta.LastIndex

						// Parse the services and their tags
						ingressClient.kvServiceDefs = ingressClient.parser.ParseKV(&kvPairs)

						// Output the list of services
						for _, serviceDef := range ingressClient.kvServiceDefs {
							log.Info("Service", zap.String("to", serviceDef.To), zap.String("service", serviceDef.Upstream), zap.Strings("srvUrls", serviceDef.SrvUrls), zap.Bool("useHttps", serviceDef.UseHttps), zap.Bool("skipTlsVerify", serviceDef.SkipTlsVerify))
						}

						ingressClient.updateCaddyfile(log)
					}
				}
			}

			// Connection to Consul lost, attempt reconnection
			log.Warn("Connection to Consul lost, attempting reconnection...")
			time.Sleep(5 * time.Second) // Wait before attempting reconnection
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

		// Create a new buffer with the configuration
		buf := bytes.NewBufferString(caddyfile)

		// Create a new request
		req, err := http.NewRequest("POST", "http://localhost:2019/load", buf)
		if err != nil {
			log.Error("Failed to create a new request", zap.Error(err))
		}

		// Set the content type
		req.Header.Set("Content-Type", "text/caddyfile")

		// Send the request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Error("Failed to send the request", zap.Error(err))
		}
		defer resp.Body.Close()

		// Check the response
		if resp.StatusCode == http.StatusOK {
			log.Info("Successfully updated the Caddyfile")
		} else {
			log.Warn("Failed to update the Caddyfile", zap.Any("response", resp.StatusCode))
		}
	} else {
		log.Debug("Caddyfile has not changed, skipping reload")
	}
}
