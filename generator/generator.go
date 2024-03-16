package generator

import (
	"bytes"
	"embed"
	"html/template"
	"path"

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
	return &CaddyfileGenerator{
		log:     log,
		options: options,
	}
}

func (generator *CaddyfileGenerator) Generate(serviceDefs *parser.Services, kvServiceDefs *parser.Services) string {

	// Combine the service definitions and the KV service definitions into a single slice of service definitions
	var allServiceDefs []*parser.ServiceDef
	if serviceDefs != nil {
		allServiceDefs = append(allServiceDefs, serviceDefs.Services...)
	}
	if kvServiceDefs != nil {
		allServiceDefs = append(allServiceDefs, kvServiceDefs.Services...)
	}

	// Create a map of wildcard domains to service definitions, merge from serviceDefs and kvServiceDefs if they have the wildcard domain
	wildcardGroups := make(map[string]*parser.ServiceGroup)
	for _, wildcardDomain := range generator.options.WildcardDomains {
		wc := make([]*parser.ServiceDef, 0)
		to := ""
		upstream := ""
		useHttps := false
		skipTlsVerify := false

		if serviceDefs != nil {
			if _, ok := serviceDefs.ServiceGroups[wildcardDomain]; ok {
				wc = append(wc, serviceDefs.ServiceGroups[wildcardDomain].Services...)

				if serviceDefs.ServiceGroups[wildcardDomain].Upstream != "" {
					to = serviceDefs.ServiceGroups[wildcardDomain].To
					upstream = serviceDefs.ServiceGroups[wildcardDomain].Upstream
					useHttps = serviceDefs.ServiceGroups[wildcardDomain].UseHttps
					skipTlsVerify = serviceDefs.ServiceGroups[wildcardDomain].SkipTlsVerify
				}
			}
		}

		if kvServiceDefs != nil {
			if _, ok := kvServiceDefs.ServiceGroups[wildcardDomain]; ok {
				wc = append(wc, kvServiceDefs.ServiceGroups[wildcardDomain].Services...)

				if kvServiceDefs.ServiceGroups[wildcardDomain].Upstream != "" {
					to = kvServiceDefs.ServiceGroups[wildcardDomain].To
					upstream = kvServiceDefs.ServiceGroups[wildcardDomain].Upstream
					useHttps = kvServiceDefs.ServiceGroups[wildcardDomain].UseHttps
					skipTlsVerify = kvServiceDefs.ServiceGroups[wildcardDomain].SkipTlsVerify
				}
			}
		}

		if len(wc) > 0 || upstream != "" {
			wildcardGroups[wildcardDomain] = parser.NewServiceGroup()
			wildcardGroups[wildcardDomain].To = to
			wildcardGroups[wildcardDomain].Upstream = upstream
			wildcardGroups[wildcardDomain].UseHttps = useHttps
			wildcardGroups[wildcardDomain].SkipTlsVerify = skipTlsVerify
			wildcardGroups[wildcardDomain].Services = wc
		}
	}

	var caddyfile = ""
	var tmpl *template.Template
	var err error

	// Create the template
	if generator.options.TemplateFile != "" {
		tmpl, err = template.New(path.Base(generator.options.TemplateFile)).Delims("[[", "]]").ParseFiles(generator.options.TemplateFile)
	} else {
		tmpl, err = template.New("service.tmpl").Delims("[[", "]]").ParseFS(tmplFiles, "templates/service.tmpl")
	}

	if err != nil {
		generator.log.Fatal("Failed to parse template", zap.Error(err))
	}

	var tmplData = map[string]interface{}{
		"services":         allServiceDefs,
		"wildcardServices": wildcardGroups,
	}

	var tmplBytes bytes.Buffer
	err = tmpl.Execute(&tmplBytes, tmplData)
	if err != nil {
		generator.log.Fatal("Failed to execute template", zap.Error(err))
	}

	caddyfile = tmplBytes.String()

	if generator.options.Verbose {
		generator.log.Info(caddyfile)
	}

	return caddyfile
}
