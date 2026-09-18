//go:build e2e && e2e_gateway

/*
Copyright 2026 The Kubernetes Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package framework

import (
	"fmt"
	"net/http"
	"os"
	"testing"
)

// ProviderConfig describes a gateway provider for conformance testing.
type ProviderConfig struct {
	Name       string
	ConfigData map[string]string
}

var providers = map[string]ProviderConfig{
	"httproute": {
		Name: "httproute",
		ConfigData: map[string]string{
			"gateway-name":      "e2e-gateway",
			"gateway-namespace": "gateway-system",
			"gateway-class":     "eg",
			"route-hostname":    "mcp.e2e.test",
			"public-hostname":   "mcp.e2e.test",
		},
	},
	"kuadrant": {
		Name: "kuadrant",
		ConfigData: map[string]string{
			"gateway-name":      "mcp-gateway",
			"gateway-namespace": "gateway-system",
			"gateway-class":     "istio",
			"route-hostname":    "mcp.127-0-0-1.sslip.io",
			"public-hostname":   "mcp.127-0-0-1.sslip.io",
			"prefix":            "e2e_",
		},
	},
}

// ActiveProvider returns the ProviderConfig selected by the GATEWAY_PROVIDER
// environment variable. It fatals if the variable is unset or unknown.
func ActiveProvider(t *testing.T) ProviderConfig {
	t.Helper()
	name := os.Getenv("GATEWAY_PROVIDER")
	if name == "" {
		t.Fatal("GATEWAY_PROVIDER environment variable is not set")
	}
	prov, ok := providers[name]
	if !ok {
		t.Fatalf("unknown gateway provider %q, available: %v", name, providerNames())
	}
	return prov
}

func providerNames() []string {
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	return names
}

// NewGatewayRequest builds an *http.Request that targets the gateway's
// LoadBalancer address directly. The Host header is set to hostname so
// the gateway's listener can perform host-based routing.
func NewGatewayRequest(t *testing.T, gatewayAddress, method, path, hostname string) *http.Request {
	t.Helper()

	u := fmt.Sprintf("http://%s%s", gatewayAddress, path)
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		t.Fatalf("failed to create gateway request: %v", err)
	}
	req.Host = hostname
	return req
}

type hostOverrideTransport struct {
	base http.RoundTripper
	host string
}

func (t *hostOverrideTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Host = t.host
	return t.base.RoundTrip(req)
}

// WithHostOverride wraps the given http.Client's transport so that every
// outgoing request sets the Host header to the given value. This is needed
// when the MCP SDK client creates its own requests internally and the
// gateway performs host-based routing.
func WithHostOverride(c *http.Client, host string) *http.Client {
	base := c.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{
		Transport: &hostOverrideTransport{base: base, host: host},
	}
}

func init() {
	for name, p := range providers {
		if p.Name != name {
			panic(fmt.Sprintf("provider config key %q does not match Name %q", name, p.Name))
		}
	}
}
