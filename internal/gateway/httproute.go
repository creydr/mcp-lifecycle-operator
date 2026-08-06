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

package gateway

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	mcpv1alpha1 "github.com/kubernetes-sigs/mcp-lifecycle-operator/api/v1alpha1"
)

func BuildHTTPRoute(
	binding *mcpv1alpha1.MCPGatewayBinding,
	mcpServer *mcpv1alpha1.MCPServer,
	gwName, gwNamespace string,
	cm *corev1.ConfigMap,
) *gatewayv1.HTTPRoute {
	path := mcpServer.Spec.Config.Path
	if path == "" {
		path = DefaultMCPPath
	}
	pathType := gatewayv1.PathMatchPathPrefix
	gwNS := gatewayv1.Namespace(gwNamespace)
	port := mcpServer.Spec.Config.Port

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      binding.Name,
			Namespace: binding.Namespace,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:      gatewayv1.ObjectName(gwName),
						Namespace: &gwNS,
					},
				},
			},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{
						{
							Path: &gatewayv1.HTTPPathMatch{
								Type:  &pathType,
								Value: &path,
							},
						},
					},
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName(mcpServer.Name),
									Port: &port,
								},
							},
						},
					},
				},
			},
		},
	}

	if hostname, ok := cm.Data[ConfigKeyHostname]; ok && hostname != "" {
		route.Spec.Hostnames = []gatewayv1.Hostname{gatewayv1.Hostname(hostname)}
	}

	return route
}

func ReconcileHTTPRoute(ctx context.Context, c client.Client, desired *gatewayv1.HTTPRoute) error {
	logger := log.FromContext(ctx)
	existing := &gatewayv1.HTTPRoute{}
	err := c.Get(ctx, client.ObjectKey{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		logger.Info("Creating HTTPRoute", "name", desired.Name)
		if err := c.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create HTTPRoute: %w", err)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		logger.Info("Updating HTTPRoute", "name", desired.Name)
		existing.Spec = desired.Spec
		if err := c.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update HTTPRoute: %w", err)
		}
	}
	return nil
}
