package proxy

// NewProxyForTest creates a Proxy with the given providers for testing.
// This is a test-only constructor that allows setting unexported fields.
func NewProxyForTest(providers map[string]Provider) *Proxy {
	return &Proxy{
		providers: providers,
	}
}
