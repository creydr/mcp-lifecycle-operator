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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	mcpv1alpha1 "github.com/kubernetes-sigs/mcp-lifecycle-operator/api/v1alpha1"
)

// MCPGatewayBindingReconciler reconciles MCPGatewayBinding resources by
// delegating to registered GatewayProvider implementations.
type MCPGatewayBindingReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Providers []GatewayProvider

	activeProviders map[string]GatewayProvider
}

// +kubebuilder:rbac:groups=mcp.x-k8s.io,resources=mcpgatewaybindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=mcp.x-k8s.io,resources=mcpgatewaybindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mcp.x-k8s.io,resources=mcpservers,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *MCPGatewayBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	binding := &mcpv1alpha1.MCPGatewayBinding{}
	if err := r.Get(ctx, req.NamespacedName, binding); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	provider, ok := r.activeProviders[binding.Spec.Provider]
	if !ok {
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling MCPGatewayBinding", "name", binding.Name, "namespace", binding.Namespace, "provider", binding.Spec.Provider)

	mcpServer := &mcpv1alpha1.MCPServer{}
	if err := r.Get(ctx, client.ObjectKey{Name: binding.Spec.MCPServerRef, Namespace: binding.Namespace}, mcpServer); err != nil {
		return ctrl.Result{}, r.setNotRegistered(ctx, binding,
			fmt.Sprintf("MCPServer %q not found: %v", binding.Spec.MCPServerRef, err))
	}

	var configMap *corev1.ConfigMap
	if binding.Spec.ConfigRef != "" {
		configMap = &corev1.ConfigMap{}
		if err := r.Get(ctx, client.ObjectKey{Name: binding.Spec.ConfigRef, Namespace: binding.Namespace}, configMap); err != nil {
			return ctrl.Result{}, r.setNotRegistered(ctx, binding,
				fmt.Sprintf("ConfigMap %q not found: %v", binding.Spec.ConfigRef, err))
		}
	}

	url, err := provider.Reconcile(ctx, ProviderParams{
		Client:    r.Client,
		Scheme:    r.Scheme,
		Binding:   binding,
		MCPServer: mcpServer,
		ConfigMap: configMap,
	})
	if err != nil {
		return ctrl.Result{}, r.setNotRegistered(ctx, binding, err.Error())
	}

	return ctrl.Result{}, r.updateBindingStatus(ctx, binding, metav1.ConditionTrue,
		ReasonGatewayRegistered, fmt.Sprintf("%s resources created", provider.Name()), url)
}

func (r *MCPGatewayBindingReconciler) setNotRegistered(
	ctx context.Context,
	binding *mcpv1alpha1.MCPGatewayBinding,
	message string,
) error {
	return r.updateBindingStatus(ctx, binding, metav1.ConditionFalse, ReasonGatewayNotRegistered, message, "")
}

func (r *MCPGatewayBindingReconciler) updateBindingStatus(
	ctx context.Context,
	binding *mcpv1alpha1.MCPGatewayBinding,
	status metav1.ConditionStatus,
	reason, message, url string,
) error {
	condition := metav1.Condition{
		Type:               ConditionTypeRegistered,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: binding.Generation,
		LastTransitionTime: metav1.Now(),
	}
	preserveLastTransitionTime(&condition, binding.Status.Conditions)

	meta.SetStatusCondition(&binding.Status.Conditions, condition)
	binding.Status.URL = url

	return r.Status().Update(ctx, binding)
}

// SetupWithManager sets up the controller with the Manager.
// It activates only providers whose required CRDs are present on the cluster.
func (r *MCPGatewayBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	setupLog := mgr.GetLogger().WithName("setup")

	r.activeProviders = make(map[string]GatewayProvider)
	for _, p := range r.Providers {
		if r.crdAvailable(mgr, p) {
			r.activeProviders[p.Name()] = p
			setupLog.Info("Gateway provider activated", "provider", p.Name())
		} else {
			setupLog.Info("Gateway provider CRDs not found, skipping",
				"provider", p.Name())
		}
	}

	if len(r.activeProviders) == 0 {
		setupLog.Info("No gateway providers available, skipping MCPGatewayBinding controller")
		return nil
	}

	b := ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1alpha1.MCPGatewayBinding{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findBindingsForConfigMap),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&mcpv1alpha1.MCPServer{},
			handler.EnqueueRequestsFromMapFunc(r.findBindingsForMCPServer),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Named("mcpgatewaybinding")

	for _, p := range r.activeProviders {
		for _, t := range p.OwnedTypes() {
			b = b.Owns(t)
		}
	}

	return b.Complete(r)
}

func (r *MCPGatewayBindingReconciler) crdAvailable(mgr ctrl.Manager, p GatewayProvider) bool {
	for _, gvk := range p.RequiredCRDs() {
		if _, err := mgr.GetRESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version); err != nil {
			return false
		}
	}
	return true
}

func (r *MCPGatewayBindingReconciler) findBindingsForConfigMap(ctx context.Context, obj client.Object) []ctrl.Request {
	bindingList := &mcpv1alpha1.MCPGatewayBindingList{}
	if err := r.List(ctx, bindingList, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	var requests []ctrl.Request
	for i := range bindingList.Items {
		if _, ok := r.activeProviders[bindingList.Items[i].Spec.Provider]; ok &&
			bindingList.Items[i].Spec.ConfigRef == obj.GetName() {
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(&bindingList.Items[i]),
			})
		}
	}
	return requests
}

func (r *MCPGatewayBindingReconciler) findBindingsForMCPServer(ctx context.Context, obj client.Object) []ctrl.Request {
	bindingList := &mcpv1alpha1.MCPGatewayBindingList{}
	if err := r.List(ctx, bindingList, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	var requests []ctrl.Request
	for i := range bindingList.Items {
		if _, ok := r.activeProviders[bindingList.Items[i].Spec.Provider]; ok &&
			bindingList.Items[i].Spec.MCPServerRef == obj.GetName() {
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(&bindingList.Items[i]),
			})
		}
	}
	return requests
}
