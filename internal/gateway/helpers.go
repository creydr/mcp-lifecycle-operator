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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1alpha1 "github.com/kubernetes-sigs/mcp-lifecycle-operator/api/v1alpha1"
)

const (
	ConfigKeyGatewayName      = "gateway-name"
	ConfigKeyGatewayNamespace = "gateway-namespace"
	ConfigKeyHostname         = "hostname"
)

func ExtractGatewayConfig(cm *corev1.ConfigMap, configRef string) (gwName, gwNamespace string, err error) {
	gwName, ok := cm.Data[ConfigKeyGatewayName]
	if !ok || gwName == "" {
		return "", "", fmt.Errorf("ConfigMap %q missing required key %q", configRef, ConfigKeyGatewayName)
	}
	gwNamespace, ok = cm.Data[ConfigKeyGatewayNamespace]
	if !ok || gwNamespace == "" {
		return "", "", fmt.Errorf("ConfigMap %q missing required key %q", configRef, ConfigKeyGatewayNamespace)
	}
	return gwName, gwNamespace, nil
}

func GatewayURL(cm *corev1.ConfigMap, mcpServer *mcpv1alpha1.MCPServer) string {
	hostname, ok := cm.Data[ConfigKeyHostname]
	if !ok || hostname == "" {
		return ""
	}
	path := mcpServer.Spec.Config.Path
	if path == "" {
		path = DefaultMCPPath
	}
	return fmt.Sprintf("http://%s%s", hostname, path)
}

func ReconcileUnstructured(ctx context.Context, c client.Client, gvk schema.GroupVersionKind, desired *unstructured.Unstructured) error {
	logger := log.FromContext(ctx)
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	err := c.Get(ctx, client.ObjectKey{Name: desired.GetName(), Namespace: desired.GetNamespace()}, existing)
	if apierrors.IsNotFound(err) {
		logger.Info("Creating resource", "kind", gvk.Kind, "name", desired.GetName())
		return c.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	desiredSpec, _, _ := unstructured.NestedMap(desired.Object, "spec")
	existingSpec, _, _ := unstructured.NestedMap(existing.Object, "spec")
	if !equality.Semantic.DeepEqual(existingSpec, desiredSpec) {
		logger.Info("Updating resource", "kind", gvk.Kind, "name", desired.GetName())
		if err := unstructured.SetNestedMap(existing.Object, desiredSpec, "spec"); err != nil {
			return err
		}
		return c.Update(ctx, existing)
	}
	return nil
}

func DeleteIfExists(ctx context.Context, c client.Client, gvk schema.GroupVersionKind, name, namespace string) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	if err := c.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func ParseLabelSelector(selector string) map[string]any {
	labels := make(map[string]any)
	for part := range strings.SplitSeq(selector, ",") {
		part = strings.TrimSpace(part)
		if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
			labels[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return labels
}
