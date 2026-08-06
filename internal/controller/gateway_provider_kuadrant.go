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

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	configKeyToolPrefix = "tool-prefix"
)

var mcpServerRegistrationGVK = schema.GroupVersionKind{
	Group:   "mcp.kuadrant.io",
	Version: "v1alpha1",
	Kind:    "MCPServerRegistration",
}

// KuadrantProvider creates Gateway API HTTPRoute resources and Kuadrant
// MCPServerRegistration resources for MCPGatewayBindings.
type KuadrantProvider struct{}

var _ GatewayProvider = (*KuadrantProvider)(nil)

func (p *KuadrantProvider) Name() string { return ProviderKuadrant }

func (p *KuadrantProvider) RequiredCRDs() []schema.GroupVersionKind {
	return []schema.GroupVersionKind{
		{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute"},
		mcpServerRegistrationGVK,
	}
}

func (p *KuadrantProvider) OwnedTypes() []client.Object {
	reg := &unstructured.Unstructured{}
	reg.SetGroupVersionKind(mcpServerRegistrationGVK)
	return []client.Object{
		&gatewayv1.HTTPRoute{},
		reg,
	}
}

// +kubebuilder:rbac:groups=mcp.kuadrant.io,resources=mcpserverregistrations,verbs=get;list;watch;create;update;delete

func (p *KuadrantProvider) Reconcile(ctx context.Context, params ProviderParams) (string, error) {
	if params.ConfigMap == nil {
		return "", fmt.Errorf("spec.configRef is required for %s provider", p.Name())
	}

	gwName, gwNamespace, err := extractGatewayConfig(params.ConfigMap, params.Binding.Spec.ConfigRef)
	if err != nil {
		return "", err
	}

	httpRoute := buildHTTPRoute(params.Binding, params.MCPServer, gwName, gwNamespace, params.ConfigMap)
	if err := controllerutil.SetControllerReference(params.Binding, httpRoute, params.Scheme); err != nil {
		return "", fmt.Errorf("setting controller reference on HTTPRoute: %w", err)
	}
	if err := reconcileHTTPRoute(ctx, params.Client, httpRoute); err != nil {
		return "", err
	}

	if err := p.reconcileMCPServerRegistration(ctx, params); err != nil {
		return "", err
	}

	return gatewayURL(params.ConfigMap, params.MCPServer), nil
}

func (p *KuadrantProvider) reconcileMCPServerRegistration(ctx context.Context, params ProviderParams) error {
	path := params.MCPServer.Spec.Config.Path
	if path == "" {
		path = defaultMCPPath
	}

	spec := map[string]interface{}{
		"targetRef": map[string]interface{}{
			"group": "gateway.networking.k8s.io",
			"kind":  "HTTPRoute",
			"name":  params.Binding.Name,
		},
		"path": path,
	}

	if prefix, ok := params.ConfigMap.Data[configKeyToolPrefix]; ok && prefix != "" {
		spec["prefix"] = prefix
	}

	reg := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": mcpServerRegistrationGVK.Group + "/" + mcpServerRegistrationGVK.Version,
			"kind":       mcpServerRegistrationGVK.Kind,
			"metadata": map[string]interface{}{
				"name":      params.Binding.Name,
				"namespace": params.Binding.Namespace,
			},
			"spec": spec,
		},
	}

	if err := controllerutil.SetControllerReference(params.Binding, reg, params.Scheme); err != nil {
		return fmt.Errorf("setting controller reference on MCPServerRegistration: %w", err)
	}

	return reconcileUnstructured(ctx, params.Client, mcpServerRegistrationGVK, reg)
}
