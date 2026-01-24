package plugin

import (
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
)

const (
	// PluginName is the name used for plugin negotiation.
	PluginName = "reorg-plugin"

	// ProtocolVersion is the plugin protocol version.
	// Increment this when making breaking changes to the plugin interface.
	ProtocolVersion = 1

	// MagicCookieKey is the environment variable name for the magic cookie.
	MagicCookieKey = "REORG_PLUGIN"

	// MagicCookieValue is the expected value of the magic cookie.
	// This prevents users from accidentally running the plugin binary.
	MagicCookieValue = "reorg-plugin-v1"
)

// Handshake is the configuration for plugin handshake.
// Both the host and plugin must agree on this configuration.
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  ProtocolVersion,
	MagicCookieKey:   MagicCookieKey,
	MagicCookieValue: MagicCookieValue,
}

// PluginMap is the map of plugins we can dispense.
// The host uses this to know which plugins are available.
var PluginMap = map[string]plugin.Plugin{
	PluginName: &GRPCPluginImpl{},
}

// Serve starts the plugin server. This should be called from the plugin's main().
// The implementation parameter is the plugin's implementation of the Plugin interface.
//
// Example:
//
//	func main() {
//	    plugin.Serve(&MyPlugin{})
//	}
func Serve(impl Plugin) {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins: map[string]plugin.Plugin{
			PluginName: &GRPCPluginImpl{Impl: impl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}

// ServeWithBroker starts the plugin server with a custom broker configuration.
// This is useful for advanced scenarios where you need more control.
func ServeWithBroker(impl Plugin, opts ...ServeOption) {
	cfg := &plugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins: map[string]plugin.Plugin{
			PluginName: &GRPCPluginImpl{Impl: impl},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	plugin.Serve(cfg)
}

// ServeOption is a function that configures the serve config.
type ServeOption func(*plugin.ServeConfig)

// WithLogger returns a ServeOption that sets the logger.
func WithLogger(logger hclog.Logger) ServeOption {
	return func(cfg *plugin.ServeConfig) {
		cfg.Logger = logger
	}
}

// WithTest returns a ServeOption that enables test mode.
// In test mode, the server will not call os.Exit.
func WithTest(reattachCh chan *plugin.ReattachConfig) ServeOption {
	return func(cfg *plugin.ServeConfig) {
		cfg.Test = &plugin.ServeTestConfig{
			ReattachConfigCh: reattachCh,
		}
	}
}
