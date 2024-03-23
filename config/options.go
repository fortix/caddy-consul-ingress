package config

import (
	"time"

	"go.uber.org/zap"
)

// Options are the options for generator
type Options struct {
	TemplateFile       string
	ConsulAddress      string
	ConsulToken        string
	UrlPrefix          string
	KVPath             string
	WildcardDomains    []string
	PollingInterval    time.Duration
	Verbose            bool
	RestartOnCfgChange bool
	Logger             *zap.Logger
}
