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
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	gw "github.com/kubernetes-sigs/mcp-lifecycle-operator/internal/gateway"
)

const (
	ProviderName        = "kuadrant"
	ConfigKeyToolPrefix = "tool-prefix"
)

var MCPServerRegistrationGVK = schema.GroupVersionKind{
	Group:   "mcp.kuadrant.io",
	Version: "v1alpha1",
	Kind:    "MCPServerRegistration",
}

// Provider creates Gateway API HTTPRoute resources and Kuadrant
// MCPServerRegistration resources for MCPGatewayBindings.
type Provider struct{}

var _ gw.Provider = (*Provider)(nil)

func (p *Provider) Name() string { return ProviderName }

func (p *Provider) RequiredCRDs() []schema.GroupVersionKind {
	return []schema.GroupVersionKind{
		{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"},
		MCPServerRegistrationGVK,
	}
}

func (p *Provider) OwnedTypes() []client.Object {
	reg := &unstructured.Unstructured{}
	reg.SetGroupVersionKind(MCPServerRegistrationGVK)
	return []client.Object{
		&gatewayv1.HTTPRoute{},
		reg,
	}
}

// +kubebuilder:rbac:groups=mcp.kuadrant.io,resources=mcpserverregistrations,verbs=get;list;watch;create;update;delete

func (p *Provider) Reconcile(ctx context.Context, params gw.ProviderParams) (string, error) {
	if params.ConfigMap == nil {
		return "", fmt.Errorf("spec.configRef is required for %s provider", p.Name())
	}

	gwName, gwNamespace, err := gw.ExtractGatewayConfig(params.ConfigMap, params.Binding.Spec.ConfigRef)
	if err != nil {
		return "", err
	}

	httpRoute := gw.BuildHTTPRoute(params.Binding, params.MCPServer, gwName, gwNamespace, params.ConfigMap)
	if err := controllerutil.SetControllerReference(params.Binding, httpRoute, params.Scheme); err != nil {
		return "", fmt.Errorf("setting controller reference on HTTPRoute: %w", err)
	}
	if err := gw.ReconcileHTTPRoute(ctx, params.Client, httpRoute); err != nil {
		return "", err
	}

	if err := p.reconcileMCPServerRegistration(ctx, params); err != nil {
		return "", err
	}

	return gw.GatewayURL(params.ConfigMap, params.MCPServer), nil
}

func (p *Provider) reconcileMCPServerRegistration(ctx context.Context, params gw.ProviderParams) error {
	path := params.MCPServer.Spec.Config.Path
	if path == "" {
		path = gw.DefaultMCPPath
	}

	spec := map[string]any{
		"targetRef": map[string]any{
			"group": "gateway.networking.k8s.io",
			"kind":  "HTTPRoute",
			"name":  params.Binding.Name,
		},
		"path": path,
	}

	if prefix, ok := params.ConfigMap.Data[ConfigKeyToolPrefix]; ok && prefix != "" {
		spec["prefix"] = prefix
	}

	reg := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": MCPServerRegistrationGVK.Group + "/" + MCPServerRegistrationGVK.Version,
			"kind":       MCPServerRegistrationGVK.Kind,
			"metadata": map[string]any{
				"name":      params.Binding.Name,
				"namespace": params.Binding.Namespace,
			},
			"spec": spec,
		},
	}

	if err := controllerutil.SetControllerReference(params.Binding, reg, params.Scheme); err != nil {
		return fmt.Errorf("setting controller reference on MCPServerRegistration: %w", err)
	}

	return gw.ReconcileUnstructured(ctx, params.Client, MCPServerRegistrationGVK, reg)
}
