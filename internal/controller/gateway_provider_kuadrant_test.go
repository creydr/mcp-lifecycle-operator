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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	mcpv1alpha1 "github.com/kubernetes-sigs/mcp-lifecycle-operator/api/v1alpha1"
)

var _ = Describe("MCPGatewayBinding Controller (kuadrant)", func() {
	ctx := context.Background()

	const (
		mcpServerName = "test-kuadrant-mcp"
		bindingName   = "test-kuadrant-binding"
		configMapName = "test-kuadrant-config"
	)

	newReconciler := func() *MCPGatewayBindingReconciler {
		return &MCPGatewayBindingReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
			Providers: []GatewayProvider{
				&KuadrantProvider{},
			},
			activeProviders: map[string]GatewayProvider{
				ProviderKuadrant: &KuadrantProvider{},
			},
		}
	}

	createMCPServer := func() {
		server := newTestMCPServer(mcpServerName)
		server.Spec.Config.Path = "/mcp"
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
	}

	createConfigMap := func(data map[string]string) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: "default",
			},
			Data: data,
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
	}

	createBinding := func(provider string) {
		binding := &mcpv1alpha1.MCPGatewayBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      bindingName,
				Namespace: "default",
			},
			Spec: mcpv1alpha1.MCPGatewayBindingSpec{
				MCPServerRef: mcpServerName,
				Provider:     provider,
				ConfigRef:    configMapName,
			},
		}
		Expect(k8sClient.Create(ctx, binding)).To(Succeed())
	}

	AfterEach(func() {
		reg := &unstructured.Unstructured{}
		reg.SetGroupVersionKind(mcpServerRegistrationGVK)
		reg.SetName(bindingName)
		reg.SetNamespace("default")
		for _, obj := range []client.Object{
			&gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: bindingName, Namespace: "default"}},
			reg,
			&mcpv1alpha1.MCPGatewayBinding{ObjectMeta: metav1.ObjectMeta{Name: bindingName, Namespace: "default"}},
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: configMapName, Namespace: "default"}},
			&mcpv1alpha1.MCPServer{ObjectMeta: metav1.ObjectMeta{Name: mcpServerName, Namespace: "default"}},
		} {
			_ = k8sClient.Delete(ctx, obj)
		}
	})

	It("should create an HTTPRoute and MCPServerRegistration", func() {
		createMCPServer()
		createConfigMap(map[string]string{
			configKeyGatewayName:      "my-gateway",
			configKeyGatewayNamespace: "gateway-ns",
			configKeyToolPrefix:       "mytools_",
		})
		createBinding(ProviderKuadrant)

		r := newReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		route := &gatewayv1.HTTPRoute{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: "default"}, route)).To(Succeed())
		Expect(route.Spec.ParentRefs).To(HaveLen(1))
		Expect(string(route.Spec.ParentRefs[0].Name)).To(Equal("my-gateway"))

		reg := &unstructured.Unstructured{}
		reg.SetGroupVersionKind(mcpServerRegistrationGVK)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: "default"}, reg)).To(Succeed())

		spec, _, _ := unstructured.NestedMap(reg.Object, "spec")
		targetRef, _, _ := unstructured.NestedMap(spec, "targetRef")
		Expect(targetRef["kind"]).To(Equal("HTTPRoute"))
		Expect(targetRef["name"]).To(Equal(bindingName))

		prefix, _, _ := unstructured.NestedString(reg.Object, "spec", "prefix")
		Expect(prefix).To(Equal("mytools_"))

		path, _, _ := unstructured.NestedString(reg.Object, "spec", "path")
		Expect(path).To(Equal("/mcp"))

		ownerRef := metav1.GetControllerOf(reg)
		Expect(ownerRef).NotTo(BeNil())
		Expect(ownerRef.Name).To(Equal(bindingName))
		Expect(ownerRef.Kind).To(Equal(mcpv1alpha1.MCPGatewayBindingKind))

		binding := &mcpv1alpha1.MCPGatewayBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: "default"}, binding)).To(Succeed())
		registered := meta.FindStatusCondition(binding.Status.Conditions, ConditionTypeRegistered)
		Expect(registered).NotTo(BeNil())
		Expect(registered.Status).To(Equal(metav1.ConditionTrue))
	})

	It("should create MCPServerRegistration without prefix when not configured", func() {
		createMCPServer()
		createConfigMap(map[string]string{
			configKeyGatewayName:      "my-gateway",
			configKeyGatewayNamespace: "gateway-ns",
		})
		createBinding(ProviderKuadrant)

		r := newReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		reg := &unstructured.Unstructured{}
		reg.SetGroupVersionKind(mcpServerRegistrationGVK)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: "default"}, reg)).To(Succeed())

		_, found, _ := unstructured.NestedString(reg.Object, "spec", "prefix")
		Expect(found).To(BeFalse())
	})

	It("should set URL when hostname is configured", func() {
		createMCPServer()
		createConfigMap(map[string]string{
			configKeyGatewayName:      "my-gateway",
			configKeyGatewayNamespace: "gateway-ns",
			configKeyHostname:         "mcp.example.com",
		})
		createBinding(ProviderKuadrant)

		r := newReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		binding := &mcpv1alpha1.MCPGatewayBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: "default"}, binding)).To(Succeed())
		Expect(binding.Status.URL).To(Equal("http://mcp.example.com/mcp"))
	})

	It("should not reconcile bindings for other providers", func() {
		createMCPServer()
		createConfigMap(map[string]string{
			configKeyGatewayName:      "my-gateway",
			configKeyGatewayNamespace: "gateway-ns",
		})
		createBinding(ProviderHTTPRoute)

		r := newReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		reg := &unstructured.Unstructured{}
		reg.SetGroupVersionKind(mcpServerRegistrationGVK)
		err = k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: "default"}, reg)
		Expect(err).To(HaveOccurred())
		Expect(client.IgnoreNotFound(err)).To(Succeed())
	})

	It("should set Registered=False when ConfigMap is missing", func() {
		createMCPServer()
		createBinding(ProviderKuadrant)

		r := newReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		binding := &mcpv1alpha1.MCPGatewayBinding{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: "default"}, binding)).To(Succeed())
		registered := meta.FindStatusCondition(binding.Status.Conditions, ConditionTypeRegistered)
		Expect(registered).NotTo(BeNil())
		Expect(registered.Status).To(Equal(metav1.ConditionFalse))
		Expect(registered.Message).To(ContainSubstring(configMapName))
	})

	It("should update MCPServerRegistration when ConfigMap changes", func() {
		createMCPServer()
		createConfigMap(map[string]string{
			configKeyGatewayName:      "my-gateway",
			configKeyGatewayNamespace: "gateway-ns",
			configKeyToolPrefix:       "old_",
		})
		createBinding(ProviderKuadrant)

		r := newReconciler()
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		reg := &unstructured.Unstructured{}
		reg.SetGroupVersionKind(mcpServerRegistrationGVK)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: "default"}, reg)).To(Succeed())
		prefix, _, _ := unstructured.NestedString(reg.Object, "spec", "prefix")
		Expect(prefix).To(Equal("old_"))

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: "default"}, cm)).To(Succeed())
		cm.Data[configKeyToolPrefix] = "new_"
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())

		_, err = r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: bindingName, Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: "default"}, reg)).To(Succeed())
		prefix, _, _ = unstructured.NestedString(reg.Object, "spec", "prefix")
		Expect(prefix).To(Equal("new_"))
	})
})
