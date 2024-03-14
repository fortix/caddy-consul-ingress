package parser

import (
	"sort"
	"strings"

	"github.com/fortix/caddy-consul-ingress/config"

	consul "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
)

// Struct to hold service definition along with parsed tags
type ServiceDef struct {
	To            string
	Upstream      string
	SrvUrls       []string
	UseHttps      bool
	SkipTlsVerify bool
}

type ServiceParser struct {
	log     *zap.Logger
	options *config.Options
}

func NewParser(log *zap.Logger, options *config.Options) *ServiceParser {
	return &ServiceParser{
		log:     log,
		options: options,
	}
}

func (p *ServiceParser) ParseKV(kvPairs *consul.KVPairs) []*ServiceDef {
	serviceMap := make(map[string]*ServiceDef)

	for _, kv := range *kvPairs {
		lines := strings.Split(string(kv.Value), "\n")
		for _, line := range lines {
			segments := strings.Fields(line)
			if len(segments) >= 2 {
				to, upstream := p.parseService(segments[1])
				srvUrl := segments[0]
				useHttps := false
				skipTlsVerify := false

				for _, segment := range segments[1:len(segments)] {
					if segment == "proto=https" {
						useHttps = true
					}
					if segment == "tlsskipverify=true" {
						skipTlsVerify = true
					}
				}

				def, ok := serviceMap[upstream]
				if !ok {
					def = &ServiceDef{
						To:            to,
						Upstream:      upstream,
						SrvUrls:       []string{},
						UseHttps:      useHttps,
						SkipTlsVerify: skipTlsVerify,
					}
					serviceMap[upstream] = def
				} else {
					if useHttps {
						def.UseHttps = true
					}
					if skipTlsVerify {
						def.SkipTlsVerify = true
					}
				}

				def.SrvUrls = append(def.SrvUrls, srvUrl)
			}
		}
	}

	serviceDefs := make([]*ServiceDef, 0, len(serviceMap))
	for _, def := range serviceMap {
		serviceDefs = append(serviceDefs, def)
	}

	// Sort the serviceDefs by service name to keep hash comparison consistent
	sort.Slice(serviceDefs, func(i, j int) bool {
		return serviceDefs[i].Upstream < serviceDefs[j].Upstream
	})

	return serviceDefs
}

func (p *ServiceParser) ParseService(services map[string][]string) []*ServiceDef {
	var serviceDefs = []*ServiceDef{}

	// Parse the services and their tags
	for service, tags := range services {
		if len(tags) > 0 {
			to, upstream := p.parseService(service)

			def := &ServiceDef{
				To:            to,
				Upstream:      upstream,
				SrvUrls:       []string{},
				UseHttps:      false,
				SkipTlsVerify: false,
			}

			for _, tag := range tags {
				if strings.HasPrefix(tag, p.options.UrlPrefix) {
					segments := strings.Fields(tag)
					srvUrl := strings.TrimPrefix(segments[0], p.options.UrlPrefix)
					def.SrvUrls = append(def.SrvUrls, srvUrl)

					for _, segment := range segments[1:] {
						if segment == "proto=https" {
							def.UseHttps = true
						}
						if segment == "tlsskipverify=true" {
							def.SkipTlsVerify = true
						}
					}
				}
			}

			// If have urls then append to serviceDefs
			if len(def.SrvUrls) > 0 {
				serviceDefs = append(serviceDefs, def)
			}
		}
	}

	// Sort the serviceDefs by service name to keep hash comparison consistent
	sort.Slice(serviceDefs, func(i, j int) bool {
		return serviceDefs[i].Upstream < serviceDefs[j].Upstream
	})

	return serviceDefs
}

func (p *ServiceParser) parseService(service string) (string, string) {
	var to string
	var upstream string

	// If service starts with http:// or https:// then use it as is and to will be "to"
	if strings.HasPrefix(service, "http://") || strings.HasPrefix(service, "https://") {
		to = "to"
		upstream = service
	} else {
		// It's a service so append .service.consul to it
		to = "dynamic srv"
		upstream = service + ".service.consul"
	}

	return to, upstream
}
