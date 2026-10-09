//go:build e2e

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

package httproute

import (
	"context"
	"testing"

	"sigs.k8s.io/e2e-framework/pkg/envconf"

	f "github.com/kubernetes-sigs/mcp-lifecycle-operator/test/e2e/framework"
)

type httprouteProvider struct {
	gatewayName      string
	gatewayNamespace string
	sectionName      string
}

func newHTTPRouteProvider() *httprouteProvider {
	return &httprouteProvider{
		gatewayName:      "e2e-gateway",
		gatewayNamespace: "gateway-system",
	}
}

func (p *httprouteProvider) Name() string { return "httproute" }

func (p *httprouteProvider) Setup(ctx context.Context, t *testing.T, cfg *envconf.Config, ns string) {
	t.Helper()
	listenerName, _ := f.DiscoverGateway(ctx, t, cfg, p.gatewayName, p.gatewayNamespace)
	p.sectionName = listenerName
}

func (p *httprouteProvider) ConfigData() map[string]string {
	data := map[string]string{
		"gateway-name":      p.gatewayName,
		"gateway-namespace": p.gatewayNamespace,
		"route-hostname":    "mcp.e2e.test",
		"public-hostname":   "mcp.e2e.test",
	}
	if p.sectionName != "" {
		data["section-name"] = p.sectionName
	}
	return data
}

func (p *httprouteProvider) InvalidConfigData() map[string]string {
	data := map[string]string{
		"gateway-name":      "nonexistent-gateway",
		"gateway-namespace": p.gatewayNamespace,
		"route-hostname":    "invalid.mcp.local",
		"public-hostname":   "invalid.mcp.local",
	}
	if p.sectionName != "" {
		data["section-name"] = p.sectionName
	}
	return data
}

func (p *httprouteProvider) GatewayAddress(ctx context.Context, t *testing.T, cfg *envconf.Config) string {
	t.Helper()
	return f.WaitForGatewayAddress(ctx, t, cfg.Client().Resources(), p.gatewayName, p.gatewayNamespace)
}
