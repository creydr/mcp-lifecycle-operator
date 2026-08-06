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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1alpha1 "github.com/kubernetes-sigs/mcp-lifecycle-operator/api/v1alpha1"
)

// GatewayProvider defines the interface for gateway integration providers.
// Each provider creates provider-specific resources (e.g., HTTPRoute, policies)
// for MCPGatewayBindings that match its name.
type GatewayProvider interface {
	// Name returns the provider identifier, matching MCPGatewayBinding.Spec.Provider.
	Name() string

	// RequiredCRDs returns GVKs that must be present on the cluster for this
	// provider to activate. If any CRD is missing, the provider is skipped.
	RequiredCRDs() []schema.GroupVersionKind

	// OwnedTypes returns typed objects for setting up controller Owns() watches.
	OwnedTypes() []client.Object

	// Reconcile creates or updates provider-specific resources for the binding.
	// ConfigMap may be nil if spec.configRef is empty.
	// Returns the external URL (empty string if none) or an error.
	Reconcile(ctx context.Context, params ProviderParams) (string, error)
}

// ProviderParams holds the common parameters passed to a provider's Reconcile method.
type ProviderParams struct {
	Client    client.Client
	Scheme    *runtime.Scheme
	Binding   *mcpv1alpha1.MCPGatewayBinding
	MCPServer *mcpv1alpha1.MCPServer
	ConfigMap *corev1.ConfigMap
}
