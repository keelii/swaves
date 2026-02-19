package media

import "strings"

type FactoryConfig struct {
	DefaultProvider string

	SEEBaseURL string
	SEEToken   string

	ImageKitAPIBaseURL    string
	ImageKitUploadBaseURL string
	ImageKitPrivateKey    string
}

type Factory struct {
	cfg FactoryConfig
}

func NewFactory(cfg FactoryConfig) *Factory {
	cfg.DefaultProvider = normalizeProviderName(cfg.DefaultProvider)
	if cfg.DefaultProvider == "" {
		cfg.DefaultProvider = "see"
	}
	return &Factory{cfg: cfg}
}

func (f *Factory) Resolve(name string) Provider {
	name = normalizeProviderName(name)
	if name == "" {
		name = f.cfg.DefaultProvider
	}

	switch name {
	case "imagekit":
		return NewImageKitProvider(ImageKitConfig{
			APIBaseURL:    f.cfg.ImageKitAPIBaseURL,
			UploadBaseURL: f.cfg.ImageKitUploadBaseURL,
			PrivateKey:    f.cfg.ImageKitPrivateKey,
		})
	case "see":
		fallthrough
	default:
		return NewSEEProvider(SEEConfig{
			BaseURL: f.cfg.SEEBaseURL,
			Token:   f.cfg.SEEToken,
		})
	}
}

func normalizeProviderName(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	if raw == "s.ee" || raw == "smms" || raw == "sm.ms" {
		return "see"
	}
	return raw
}
