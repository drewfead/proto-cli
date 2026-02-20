package cliauth

// Config holds the auth configuration assembled from a LoginProvider and options.
type Config struct {
	Provider  LoginProvider
	Store     AuthStore
	Decorator AuthDecorator
}

// Option configures an auth Config.
type Option func(*Config)

// WithStore sets a custom AuthStore instead of the default KeychainStore.
func WithStore(store AuthStore) Option {
	return func(c *Config) {
		c.Store = store
	}
}

// WithDecorator sets an AuthDecorator for decorating outgoing gRPC requests.
func WithDecorator(decorator AuthDecorator) Option {
	return func(c *Config) {
		c.Decorator = decorator
	}
}

// NewConfig creates a Config for the given provider with sensible defaults.
// If no Store is provided via options, a KeychainStore is used.
func NewConfig(appName string, provider LoginProvider, opts ...Option) *Config {
	cfg := &Config{
		Provider: provider,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.Store == nil {
		cfg.Store = NewKeychainStore(appName)
	}
	return cfg
}
