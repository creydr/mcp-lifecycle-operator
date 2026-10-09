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

package kuadrant

import (
	"context"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kuadrantapi "github.com/kubernetes-sigs/mcp-lifecycle-operator/internal/controller/providers/kuadrant/api"
	f "github.com/kubernetes-sigs/mcp-lifecycle-operator/test/e2e/framework"
)

var schemeOnce sync.Once

type kuadrantProvider struct {
	gatewayName      string
	gatewayNamespace string
	extensionName    string
	extensionNs      string
	prefix           string
	sectionName      string
}

func newKuadrantProvider() *kuadrantProvider {
	return &kuadrantProvider{
		gatewayName:      "mcp-gateway",
		gatewayNamespace: "gateway-system",
		extensionName:    "mcp-gateway-extension",
		extensionNs:      "mcp-system",
		prefix:           "e2e_",
	}
}

func ensureScheme(t *testing.T, scheme *runtime.Scheme) {
	t.Helper()
	schemeOnce.Do(func() {
		if err := kuadrantapi.AddToScheme(scheme); err != nil {
			t.Fatalf("failed to register Kuadrant types: %v", err)
		}
	})
}

func (p *kuadrantProvider) Name() string { return "kuadrant" }

func (p *kuadrantProvider) Setup(ctx context.Context, t *testing.T, cfg *envconf.Config, ns string) {
	t.Helper()
	ensureScheme(t, cfg.Client().Resources().GetScheme())
	f.EnsureReferenceGrant(ctx, t, cfg, ns, p.gatewayNamespace)
	f.WaitForExtensionReady(ctx, t, cfg, p.extensionName, p.extensionNs)

	listenerName, _ := f.DiscoverGateway(ctx, t, cfg, p.gatewayName, p.gatewayNamespace)
	p.sectionName = listenerName
}

func (p *kuadrantProvider) ConfigData() map[string]string {
	data := map[string]string{
		"extension-name":      p.extensionName,
		"extension-namespace": p.extensionNs,
		"prefix":              p.prefix,
		"route-hostname":      "mcp.e2e.test",
	}
	if p.sectionName != "" {
		data["section-name"] = p.sectionName
	}
	return data
}

func (p *kuadrantProvider) InvalidConfigData() map[string]string {
	data := map[string]string{
		"extension-name":      "nonexistent-extension",
		"extension-namespace": p.gatewayNamespace,
		"prefix":              p.prefix,
		"route-hostname":      "invalid.mcp.local",
	}
	if p.sectionName != "" {
		data["section-name"] = p.sectionName
	}
	return data
}

func (p *kuadrantProvider) GatewayAddress(ctx context.Context, t *testing.T, cfg *envconf.Config) string {
	t.Helper()
	return f.WaitForGatewayAddress(ctx, t, cfg.Client().Resources(), p.gatewayName, p.gatewayNamespace)
}
