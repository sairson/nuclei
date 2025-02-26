package nuclei

import (
	"context"
	"time"

	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/nuclei/v3/pkg/model/types/severity"
	"github.com/projectdiscovery/nuclei/v3/pkg/output"
	"github.com/projectdiscovery/nuclei/v3/pkg/progress"
	"github.com/projectdiscovery/nuclei/v3/pkg/protocols/common/hosterrorscache"
	"github.com/projectdiscovery/nuclei/v3/pkg/protocols/common/interactsh"
	"github.com/projectdiscovery/nuclei/v3/pkg/protocols/common/utils/vardump"
	"github.com/projectdiscovery/nuclei/v3/pkg/protocols/headless/engine"
	"github.com/projectdiscovery/nuclei/v3/pkg/templates/types"
	"github.com/projectdiscovery/ratelimit"
)

// TemplateSources contains template sources
// which define where to load templates from
type TemplateSources struct {
	Templates       []string // template file/directory paths
	Workflows       []string // workflow file/directory paths
	RemoteTemplates []string // remote template urls
	RemoteWorkflows []string // remote workflow urls
	TrustedDomains  []string // trusted domains for remote templates/workflows
}

// WithTemplatesOrWorkflows sets templates / workflows to use /load
func WithTemplatesOrWorkflows(sources TemplateSources) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		// by default all of these values are empty
		e.opts.Templates = sources.Templates
		e.opts.Workflows = sources.Workflows
		e.opts.TemplateURLs = sources.RemoteTemplates
		e.opts.WorkflowURLs = sources.RemoteWorkflows
		e.opts.RemoteTemplateDomainList = append(e.opts.RemoteTemplateDomainList, sources.TrustedDomains...)
		return nil
	}
}

// config contains all SDK configuration options
type TemplateFilters struct {
	Severity             string   // filter by severities (accepts CSV values of info, low, medium, high, critical)
	ExcludeSeverities    string   // filter by excluding severities (accepts CSV values of info, low, medium, high, critical)
	ProtocolTypes        string   // filter by protocol types
	ExcludeProtocolTypes string   // filter by excluding protocol types
	Authors              []string // fiter by author
	Tags                 []string // filter by tags present in template
	ExcludeTags          []string // filter by excluding tags present in template
	IncludeTags          []string // filter by including tags present in template
	IDs                  []string // filter by template IDs
	ExcludeIDs           []string // filter by excluding template IDs
	TemplateCondition    []string // DSL condition/ expression
}

// WithTemplateFilters sets template filters and only templates matching the filters will be
// loaded and executed
func WithTemplateFilters(filters TemplateFilters) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		s := severity.Severities{}
		if err := s.Set(filters.Severity); err != nil {
			return err
		}
		es := severity.Severities{}
		if err := es.Set(filters.ExcludeSeverities); err != nil {
			return err
		}
		pt := types.ProtocolTypes{}
		if err := pt.Set(filters.ProtocolTypes); err != nil {
			return err
		}
		ept := types.ProtocolTypes{}
		if err := ept.Set(filters.ExcludeProtocolTypes); err != nil {
			return err
		}
		e.opts.Authors = filters.Authors
		e.opts.Tags = filters.Tags
		e.opts.ExcludeTags = filters.ExcludeTags
		e.opts.IncludeTags = filters.IncludeTags
		e.opts.IncludeIds = filters.IDs
		e.opts.ExcludeIds = filters.ExcludeIDs
		e.opts.Severities = s
		e.opts.ExcludeSeverities = es
		e.opts.Protocols = pt
		e.opts.ExcludeProtocols = ept
		e.opts.IncludeConditions = filters.TemplateCondition
		return nil
	}
}

// InteractshOpts contains options for interactsh
type InteractshOpts interactsh.Options

// WithInteractshOptions sets interactsh options
func WithInteractshOptions(opts InteractshOpts) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		if e.mode == threadSafe {
			return ErrOptionsNotSupported.Msgf("WithInteractshOptions")
		}
		optsPtr := &opts
		e.interactshOpts = (*interactsh.Options)(optsPtr)
		return nil
	}
}

// Concurrency options
type Concurrency struct {
	TemplateConcurrency         int // number of templates to run concurrently (per host in host-spray mode)
	HostConcurrency             int // number of hosts to scan concurrently  (per template in template-spray mode)
	HeadlessHostConcurrency     int // number of hosts to scan concurrently for headless templates  (per template in template-spray mode)
	HeadlessTemplateConcurrency int // number of templates to run concurrently for headless templates (per host in host-spray mode)
}

// WithConcurrency sets concurrency options
func WithConcurrency(opts Concurrency) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		e.opts.TemplateThreads = opts.TemplateConcurrency
		e.opts.BulkSize = opts.HostConcurrency
		e.opts.HeadlessBulkSize = opts.HeadlessHostConcurrency
		e.opts.HeadlessTemplateThreads = opts.HeadlessTemplateConcurrency
		return nil
	}
}

// WithGlobalRateLimit sets global rate (i.e all hosts combined) limit options
func WithGlobalRateLimit(maxTokens int, duration time.Duration) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		e.rateLimiter = ratelimit.New(context.Background(), uint(maxTokens), duration)
		return nil
	}
}

// HeadlessOpts contains options for headless templates
type HeadlessOpts struct {
	PageTimeout     int // timeout for page load
	ShowBrowser     bool
	HeadlessOptions []string
	UseChrome       bool
}

// EnableHeadless allows execution of headless templates
// *Use With Caution*: Enabling headless mode may open up attack surface due to browser usage
// and can be prone to exploitation by custom unverified templates if not properly configured
func EnableHeadlessWithOpts(hopts *HeadlessOpts) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		e.opts.Headless = true
		if hopts != nil {
			e.opts.HeadlessOptionalArguments = hopts.HeadlessOptions
			e.opts.PageTimeout = hopts.PageTimeout
			e.opts.ShowBrowser = hopts.ShowBrowser
			e.opts.UseInstalledChrome = hopts.UseChrome
		}
		if engine.MustDisableSandbox() {
			gologger.Warning().Msgf("The current platform and privileged user will run the browser without sandbox\n")
		}
		browser, err := engine.New(e.opts)
		if err != nil {
			return err
		}
		e.executerOpts.Browser = browser
		return nil
	}
}

// StatsOptions
type StatsOptions struct {
	Interval         int
	JSON             bool
	MetricServerPort int
}

// EnableStats enables Stats collection with defined interval(in sec) and callback
// Note: callback is executed in a separate goroutine
func EnableStatsWithOpts(opts StatsOptions) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		if e.mode == threadSafe {
			return ErrOptionsNotSupported.Msgf("EnableStatsWithOpts")
		}
		if opts.Interval == 0 {
			opts.Interval = 5 //sec
		}
		e.opts.StatsInterval = opts.Interval
		e.enableStats = true
		e.opts.StatsJSON = opts.JSON
		e.opts.MetricsPort = opts.MetricServerPort
		return nil
	}
}

// VerbosityOptions
type VerbosityOptions struct {
	Verbose       bool // show verbose output
	Silent        bool // show only results
	Debug         bool // show debug output
	DebugRequest  bool // show request in debug output
	DebugResponse bool // show response in debug output
	ShowVarDump   bool // show variable dumps in output
}

// WithVerbosity allows setting verbosity options of (internal) nuclei engine
// and does not affect SDK output
func WithVerbosity(opts VerbosityOptions) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		if e.mode == threadSafe {
			return ErrOptionsNotSupported.Msgf("WithVerbosity")
		}
		e.opts.Verbose = opts.Verbose
		e.opts.Silent = opts.Silent
		e.opts.Debug = opts.Debug
		e.opts.DebugRequests = opts.DebugRequest
		e.opts.DebugResponse = opts.DebugResponse
		if opts.ShowVarDump {
			vardump.EnableVarDump = true
		}
		return nil
	}
}

// NetworkConfig contains network config options
// ex: retries , httpx probe , timeout etc
type NetworkConfig struct {
	Timeout           int      // Timeout in seconds
	Retries           int      // Number of retries
	LeaveDefaultPorts bool     // Leave default ports for http/https
	MaxHostError      int      // Maximum number of host errors to allow before skipping that host
	TrackError        []string // Adds given errors to max host error watchlist
	DisableMaxHostErr bool     // Disable max host error optimization (Hosts are not skipped even if they are not responding)
	Interface         string   // Interface to use for network scan
	SourceIP          string   // SourceIP sets custom source IP address for network requests
}

// WithNetworkConfig allows setting network config options
func WithNetworkConfig(opts NetworkConfig) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		if e.mode == threadSafe {
			return ErrOptionsNotSupported.Msgf("WithNetworkConfig")
		}
		e.opts.Timeout = opts.Timeout
		e.opts.Retries = opts.Retries
		e.opts.LeaveDefaultPorts = opts.LeaveDefaultPorts
		e.hostErrCache = hosterrorscache.New(opts.MaxHostError, hosterrorscache.DefaultMaxHostsCount, opts.TrackError)
		e.opts.Interface = opts.Interface
		e.opts.SourceIP = opts.SourceIP
		return nil
	}
}

// WithProxy allows setting proxy options
func WithProxy(proxy []string, proxyInternalRequests bool) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		if e.mode == threadSafe {
			return ErrOptionsNotSupported.Msgf("WithProxy")
		}
		e.opts.Proxy = proxy
		e.opts.ProxyInternal = proxyInternalRequests
		return nil
	}
}

// WithScanStrategy allows setting scan strategy options
func WithScanStrategy(strategy string) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		e.opts.ScanStrategy = strategy
		return nil
	}
}

// OutputWriter
type OutputWriter output.Writer

// UseWriter allows setting custom output writer
// by default a mock writer is used with user defined callback
// if outputWriter is used callback will be ignored
func UseOutputWriter(writer OutputWriter) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		if e.mode == threadSafe {
			return ErrOptionsNotSupported.Msgf("UseOutputWriter")
		}
		e.customWriter = writer
		return nil
	}
}

// StatsWriter
type StatsWriter progress.Progress

// UseStatsWriter allows setting a custom stats writer
// which can be used to write stats somewhere (ex: send to webserver etc)
func UseStatsWriter(writer StatsWriter) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		if e.mode == threadSafe {
			return ErrOptionsNotSupported.Msgf("UseStatsWriter")
		}
		e.customProgress = writer
		return nil
	}
}

// WithTemplateUpdateCallback allows setting a callback which will be called
// when nuclei templates are outdated
// Note: Nuclei-templates are crucial part of nuclei and using outdated templates or nuclei sdk is not recommended
// as it may cause unexpected results due to compatibility issues
func WithTemplateUpdateCallback(disableTemplatesAutoUpgrade bool, callback func(newVersion string)) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		if e.mode == threadSafe {
			return ErrOptionsNotSupported.Msgf("WithTemplateUpdateCallback")
		}
		e.disableTemplatesAutoUpgrade = disableTemplatesAutoUpgrade
		e.onUpdateAvailableCallback = callback
		return nil
	}
}

// WithSandboxOptions allows setting supported sandbox options
func WithSandboxOptions(allowLocalFileAccess bool, restrictLocalNetworkAccess bool) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		if e.mode == threadSafe {
			return ErrOptionsNotSupported.Msgf("WithSandboxOptions")
		}
		e.opts.AllowLocalFileAccess = allowLocalFileAccess
		e.opts.RestrictLocalNetworkAccess = restrictLocalNetworkAccess
		return nil
	}
}

// EnableCodeTemplates allows loading/executing code protocol templates
func EnableCodeTemplates() NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		e.opts.EnableCodeTemplates = true
		return nil
	}
}

// WithHeaders allows setting custom header/cookie to include in all http request in header:value format
func WithHeaders(headers []string) NucleiSDKOptions {
	return func(e *NucleiEngine) error {
		e.opts.CustomHeaders = headers
		return nil
	}
}
