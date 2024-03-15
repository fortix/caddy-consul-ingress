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

// Struct to hold all service groups and services
type Services struct {
	ServiceGroups map[string][]*ServiceDef
	Services      []*ServiceDef
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

func (p *ServiceParser) ParseKV(kvPairs *consul.KVPairs) *Services {
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

				p.log.Info("Found static URL", zap.String("url", srvUrl))

				for _, segment := range segments[1:] {
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

	// Break the list of services up for wildcard domains
	var parsedServices = &Services{}
	parsedServices.ServiceGroups = make(map[string][]*ServiceDef)
	parsedServices.Services = []*ServiceDef{}

	for _, defSrc := range serviceMap {
		def := &ServiceDef{
			To:            defSrc.To,
			Upstream:      defSrc.Upstream,
			SrvUrls:       []string{},
			UseHttps:      defSrc.UseHttps,
			SkipTlsVerify: defSrc.SkipTlsVerify,
		}

		wildcardDefs := make(map[string]*ServiceDef)
		for _, srvUrl := range defSrc.SrvUrls {
			var wildcardMatch bool = false
			var wildcardDomain string
			if len(p.options.WildcardDomains) > 0 {
				cmpUrl := strings.Replace(srvUrl, strings.SplitN(srvUrl, ".", 2)[0], "*", 1)
				for _, wildcardDomain = range p.options.WildcardDomains {
					if cmpUrl == wildcardDomain {
						wildcardMatch = true
						break
					}
				}
			}

			if wildcardMatch {
				// If the wildcard domain is not already in the serviceGroups then add it
				if _, ok := wildcardDefs[wildcardDomain]; !ok {
					wildcardDefs[wildcardDomain] = &ServiceDef{
						To:            defSrc.To,
						Upstream:      defSrc.Upstream,
						SrvUrls:       []string{},
						UseHttps:      def.UseHttps,
						SkipTlsVerify: def.SkipTlsVerify,
					}
				}
				wildcardDefs[wildcardDomain].SrvUrls = append(wildcardDefs[wildcardDomain].SrvUrls, srvUrl)
			} else {
				def.SrvUrls = append(def.SrvUrls, srvUrl)
			}
		}

		// If have urls then append to serviceDefs
		if len(def.SrvUrls) > 0 {
			parsedServices.Services = append(parsedServices.Services, def)
		}

		// If have wildcardDefs then append to serviceGroups
		if len(wildcardDefs) > 0 {
			for wildcardDomain, wildcardDef := range wildcardDefs {
				if _, ok := parsedServices.ServiceGroups[wildcardDomain]; !ok {
					parsedServices.ServiceGroups[wildcardDomain] = []*ServiceDef{}
				}
				parsedServices.ServiceGroups[wildcardDomain] = append(parsedServices.ServiceGroups[wildcardDomain], wildcardDef)
			}
		}
	}

	// Sort the serviceDefs by service name to keep hash comparison consistent
	sort.Slice(parsedServices.Services, func(i, j int) bool {
		return parsedServices.Services[i].Upstream < parsedServices.Services[j].Upstream
	})

	// For each service group, sort the serviceDefs by service name to keep hash comparison consistent
	for _, serviceGroup := range parsedServices.ServiceGroups {
		sort.Slice(serviceGroup, func(i, j int) bool {
			return (serviceGroup)[i].Upstream < (serviceGroup)[j].Upstream
		})
	}

	return parsedServices
}

func (p *ServiceParser) ParseServices(services map[string][]string) *Services {
	var parsedServices = &Services{}
	parsedServices.ServiceGroups = make(map[string][]*ServiceDef)
	parsedServices.Services = []*ServiceDef{}

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

			wildcardDefs := make(map[string]*ServiceDef)

			for _, tag := range tags {
				if strings.HasPrefix(tag, p.options.UrlPrefix) {
					segments := strings.Fields(tag)
					srvUrl := strings.TrimPrefix(segments[0], p.options.UrlPrefix)

					p.log.Info("Found service URL", zap.String("url", srvUrl))

					var useHttps bool = false
					var skipTlsVerify bool = false

					for _, segment := range segments[1:] {
						if segment == "proto=https" {
							useHttps = true
						}
						if segment == "tlsskipverify=true" {
							skipTlsVerify = true
						}
					}

					// Test if the url is part of a wildcard domain
					var wildcardMatch bool = false
					var wildcardDomain string
					if len(p.options.WildcardDomains) > 0 {
						cmpUrl := strings.Replace(srvUrl, strings.SplitN(srvUrl, ".", 2)[0], "*", 1)
						for _, wildcardDomain = range p.options.WildcardDomains {
							if cmpUrl == wildcardDomain {
								wildcardMatch = true
								break
							}
						}
					}

					if wildcardMatch {
						wildcardDef := &ServiceDef{
							To:            to,
							Upstream:      upstream,
							SrvUrls:       []string{},
							UseHttps:      useHttps,
							SkipTlsVerify: skipTlsVerify,
						}
						wildcardDef.SrvUrls = append(wildcardDef.SrvUrls, srvUrl)

						// If the wildcard domain is not already in the serviceGroups then add it
						if _, ok := wildcardDefs[wildcardDomain]; !ok {
							wildcardDefs[wildcardDomain] = &ServiceDef{
								To:            to,
								Upstream:      upstream,
								SrvUrls:       []string{},
								UseHttps:      false,
								SkipTlsVerify: false,
							}
						}

						if useHttps {
							wildcardDefs[wildcardDomain].UseHttps = true
						}
						if skipTlsVerify {
							wildcardDefs[wildcardDomain].SkipTlsVerify = true
						}
						wildcardDefs[wildcardDomain].SrvUrls = append(wildcardDefs[wildcardDomain].SrvUrls, srvUrl)

					} else {
						if useHttps {
							def.UseHttps = true
						}
						if skipTlsVerify {
							def.SkipTlsVerify = true
						}
						def.SrvUrls = append(def.SrvUrls, srvUrl)
					}
				}
			}

			// If have urls then append to serviceDefs
			if len(def.SrvUrls) > 0 {
				parsedServices.Services = append(parsedServices.Services, def)
			}

			// If have wildcardDefs then append to serviceGroups
			if len(wildcardDefs) > 0 {
				for wildcardDomain, wildcardDef := range wildcardDefs {
					if _, ok := parsedServices.ServiceGroups[wildcardDomain]; !ok {
						parsedServices.ServiceGroups[wildcardDomain] = []*ServiceDef{}
					}
					parsedServices.ServiceGroups[wildcardDomain] = append(parsedServices.ServiceGroups[wildcardDomain], wildcardDef)
				}
			}
		}
	}

	// Sort the serviceDefs by service name to keep hash comparison consistent
	sort.Slice(parsedServices.Services, func(i, j int) bool {
		return parsedServices.Services[i].Upstream < parsedServices.Services[j].Upstream
	})

	// For each service group, sort the serviceDefs by service name to keep hash comparison consistent
	for _, serviceGroup := range parsedServices.ServiceGroups {
		sort.Slice(serviceGroup, func(i, j int) bool {
			return (serviceGroup)[i].Upstream < (serviceGroup)[j].Upstream
		})
	}

	return parsedServices
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
