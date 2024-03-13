package generator

import (
	"bytes"
	"embed"
	"os"
	"sort"
	"strings"

	"github.com/fortix/caddy-consul-ingress/config"

	"text/template"

	"go.uber.org/zap"
)

var (
	//go:embed templates/*.tmpl
	tmplFiles embed.FS
)

type CaddyfileGenerator struct {
	log           *zap.Logger
	options       *config.Options
	baseCaddyfile string
}

// Struct to hold service definition along with parsed tags
type serviceDef struct {
	service       string
	srvUrls       []string
	useHttps      bool
	skipTlsVerify bool
}

func NewGenerator(log *zap.Logger, options *config.Options) *CaddyfileGenerator {
	d := &CaddyfileGenerator{
		log:     log,
		options: options,
	}

	// Load the base caddyfile if given
	if options.Caddyfile != "" {
		data, err := os.ReadFile(options.Caddyfile)
		if err != nil {
			log.Fatal("Failed to read Caddyfile", zap.Error(err))
		} else {
			d.baseCaddyfile = string(data)
		}
	} else {
		log.Debug("No base Caddyfile given")
	}

	return d
}

func (generator *CaddyfileGenerator) Generate(services map[string][]string) string {
	var caddyfile = ""
	var serviceDefs = []*serviceDef{}

	// Parse the services and their tags
	for service, tags := range services {
		if len(tags) > 0 {
			serviceDef := generator.parseService(service, tags)
			if serviceDef != nil {
				serviceDefs = append(serviceDefs, serviceDef)
			}
		}
	}

	// Sort the serviceDefs by service name to keep hash comparison consistent
	sort.Slice(serviceDefs, func(i, j int) bool {
		return serviceDefs[i].service < serviceDefs[j].service
	})

	// Start with the common base part
	caddyfile += generator.baseCaddyfile

	// If serviceDefs is empty, then log a message
	if len(serviceDefs) == 0 {
		generator.log.Warn("No services found")
	} else {
		generator.log.Info("Number of services found", zap.Int("count", len(serviceDefs)))

		var tmpl *template.Template
		var err error

		if generator.options.TemplateFile != "" {
			tmpl, err = template.ParseFiles(generator.options.TemplateFile)
		} else {
			tmpl, err = template.New("service.tmpl").ParseFS(tmplFiles, "templates/service.tmpl")
		}

		if err != nil {
			generator.log.Fatal("Failed to parse template", zap.Error(err))
		}

		for _, serviceDef := range serviceDefs {
			urls := strings.Join(serviceDef.srvUrls, " ")

			generator.log.Info("Add service", zap.String("service", serviceDef.service), zap.String("urls", urls))

			var tmplData = map[string]interface{}{
				"serviceName":   serviceDef.service,
				"urls":          urls,
				"useHttps":      serviceDef.useHttps,
				"skipTlsVerify": serviceDef.skipTlsVerify,
			}

			var tmplBytes bytes.Buffer
			err = tmpl.Execute(&tmplBytes, tmplData)
			if err != nil {
				generator.log.Fatal("Failed to execute template", zap.Error(err))
			}

			caddyfile += tmplBytes.String() + "\n"
		}
	}

	return caddyfile
}

func (generator *CaddyfileGenerator) parseService(service string, tags []string) *serviceDef {
	def := &serviceDef{
		service:       service,
		srvUrls:       []string{},
		useHttps:      false,
		skipTlsVerify: false,
	}

	for _, tag := range tags {
		if strings.HasPrefix(tag, generator.options.UrlPrefix) {
			segments := strings.Fields(tag)
			srvUrl := strings.TrimPrefix(segments[0], generator.options.UrlPrefix)
			def.srvUrls = append(def.srvUrls, srvUrl)

			for _, segment := range segments[1:] {
				if segment == "proto=https" {
					def.useHttps = true
				}
				if segment == "tlsskipverify=true" {
					def.skipTlsVerify = true
				}
			}
		}
	}

	// If no tags found then return nil to indicate that this service should be ignored
	if len(def.srvUrls) == 0 {
		return nil
	}

	return def
}
