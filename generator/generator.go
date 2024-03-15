package generator

import (
	"bytes"
	"embed"
	"os"
	"strings"
	"text/template"

	"github.com/fortix/caddy-consul-ingress/config"
	"github.com/fortix/caddy-consul-ingress/parser"

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

func (generator *CaddyfileGenerator) Generate(serviceDefs []*parser.ServiceDef, kvServiceDefs []*parser.ServiceDef) string {
	var caddyfile = ""
	var tmpl *template.Template
	var err error

	// Create the template
	if len(serviceDefs) > 0 || len(kvServiceDefs) > 0 {
		if generator.options.TemplateFile != "" {
			tmpl, err = template.New("service.tmpl").Delims("[[", "]]").ParseFiles(generator.options.TemplateFile)
		} else {
			tmpl, err = template.New("service.tmpl").Delims("[[", "]]").ParseFS(tmplFiles, "templates/service.tmpl")
		}

		if err != nil {
			generator.log.Fatal("Failed to parse template", zap.Error(err))
		}
	}

	// Start with the common base part
	caddyfile += generator.baseCaddyfile

	// If serviceDefs is empty, then log a message
	if len(serviceDefs) == 0 {
		generator.log.Warn("No services found")
	} else {
		generator.log.Info("Number of services found", zap.Int("count", len(serviceDefs)))

		for _, serviceDef := range serviceDefs {
			urls := strings.Join(serviceDef.SrvUrls, " ")

			generator.log.Info("Add service", zap.String("service", serviceDef.Upstream), zap.String("urls", urls))

			var tmplData = map[string]interface{}{
				"to":            serviceDef.To,
				"upstream":      serviceDef.Upstream,
				"urls":          urls,
				"useHttps":      serviceDef.UseHttps,
				"skipTlsVerify": serviceDef.SkipTlsVerify,
			}

			var tmplBytes bytes.Buffer
			err = tmpl.Execute(&tmplBytes, tmplData)
			if err != nil {
				generator.log.Fatal("Failed to execute template", zap.Error(err))
			}

			caddyfile += tmplBytes.String() + "\n"
		}
	}

	// If kvServiceDefs is empty, then log a message
	if len(kvServiceDefs) == 0 {
		generator.log.Warn("No static services found")
	} else {
		generator.log.Info("Number of static services found", zap.Int("count", len(kvServiceDefs)))

		for _, serviceDef := range kvServiceDefs {
			urls := strings.Join(serviceDef.SrvUrls, " ")

			generator.log.Info("Add static service", zap.String("service", serviceDef.Upstream), zap.String("urls", urls))

			var tmplData = map[string]interface{}{
				"to":            serviceDef.To,
				"upstream":      serviceDef.Upstream,
				"urls":          urls,
				"useHttps":      serviceDef.UseHttps,
				"skipTlsVerify": serviceDef.SkipTlsVerify,
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
