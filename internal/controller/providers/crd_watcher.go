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

package providers

import (
	"context"
	"sync"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// crdWatcher watches CustomResourceDefinition objects and starts provider
// controllers when their required CRDs become available.
type crdWatcher struct {
	mgr     ctrl.Manager
	mu      sync.Mutex
	pending []namedRegistration
	started map[string]bool
}

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

func (w *crdWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithName("crd-watcher")

	w.mu.Lock()
	defer w.mu.Unlock()

	remaining := make([]namedRegistration, 0, len(w.pending))
	for _, nf := range w.pending {
		if w.started[nf.name] {
			continue
		}
		if allCRDsPresent(w.mgr, nf.reg.RequiredCRDs) {
			log.Info("Required CRDs available, starting provider", "provider", nf.name)
			if err := nf.reg.Factory(w.mgr); err != nil {
				log.Error(err, "Failed to start provider, will retry", "provider", nf.name)
				remaining = append(remaining, nf)
				continue
			}
			w.started[nf.name] = true
		} else {
			remaining = append(remaining, nf)
		}
	}
	w.pending = remaining
	if len(w.pending) > 0 {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func (w *crdWatcher) setupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("provider-crd-watcher").
		Complete(w)
}

// allCRDsPresent checks whether all given GVKs are available in the API server.
// controller-runtime's DynamicRESTMapper auto-rediscovers unknown groups on
// RESTMapping calls, so explicit Reset() is not needed.
func allCRDsPresent(mgr ctrl.Manager, gvks []schema.GroupVersionKind) bool {
	mapper := mgr.GetRESTMapper()
	for _, gvk := range gvks {
		if _, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version); err != nil {
			return false
		}
	}
	return true
}
