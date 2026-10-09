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
	"fmt"
	"os/exec"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

const (
	envoyGatewayVersion = "v1.9.0"
	egGatewayNamespace  = "gateway-system"
	egGatewayName       = "e2e-gateway"
	egGatewayClassName  = "eg"
	egListenerName      = "http"
)

type envoyGatewayLifecycle struct{}

func (l *envoyGatewayLifecycle) Setup(ctx context.Context, cfg *envconf.Config) error {
	// Envoy Gateway's install.yaml bundles standard and experimental Gateway
	// API CRDs. The standard CRDs include a ValidatingAdmissionPolicy that
	// blocks the experimental CRDs in the same manifest. The partial apply
	// failure is expected; the wait steps below validate that the required
	// resources were created.
	installCmd := exec.CommandContext(ctx, "kubectl", "apply", "--server-side", "--force-conflicts", "-f",
		fmt.Sprintf("https://github.com/envoyproxy/gateway/releases/download/%s/install.yaml", envoyGatewayVersion))
	installCmd.CombinedOutput() //nolint:errcheck // partial failure expected

	steps := []struct {
		name string
		cmd  []string
	}{
		{
			"wait for Envoy Gateway",
			[]string{"kubectl", "wait", "--for=condition=Available", "--timeout=300s",
				"deployment/envoy-gateway", "-n", "envoy-gateway-system"},
		},
		{
			"wait for HTTPRoute CRD",
			[]string{"kubectl", "wait", "--for=condition=Established", "--timeout=120s",
				"crd/httproutes.gateway.networking.k8s.io"},
		},
	}

	for _, s := range steps {
		cmd := exec.CommandContext(ctx, s.cmd[0], s.cmd[1:]...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %w\n%s", s.name, err, string(out))
		}
	}

	r := cfg.Client().Resources()

	gc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: egGatewayClassName},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: "gateway.envoyproxy.io/gatewayclass-controller",
		},
	}
	if err := r.Create(ctx, gc); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create GatewayClass %s: %w", egGatewayClassName, err)
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: egGatewayNamespace}}
	if err := r.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", egGatewayNamespace, err)
	}

	fromAll := gatewayv1.NamespacesFromAll
	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      egGatewayName,
			Namespace: egGatewayNamespace,
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(egGatewayClassName),
			Listeners: []gatewayv1.Listener{{
				Name:     gatewayv1.SectionName(egListenerName),
				Port:     80,
				Protocol: gatewayv1.HTTPProtocolType,
				AllowedRoutes: &gatewayv1.AllowedRoutes{
					Namespaces: &gatewayv1.RouteNamespaces{
						From: &fromAll,
					},
				},
			}},
		},
	}
	if err := r.Create(ctx, gw); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create Gateway %s/%s: %w", egGatewayNamespace, egGatewayName, err)
	}

	return nil
}

func (l *envoyGatewayLifecycle) Teardown(ctx context.Context, cfg *envconf.Config) error {
	r := cfg.Client().Resources()

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: egGatewayName, Namespace: egGatewayNamespace},
	}
	_ = r.Delete(ctx, gw)

	gc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: egGatewayClassName},
	}
	_ = r.Delete(ctx, gc)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: egGatewayNamespace}}
	_ = r.Delete(ctx, ns)

	cmd := exec.CommandContext(ctx, "kubectl", "delete", "-f",
		fmt.Sprintf("https://github.com/envoyproxy/gateway/releases/download/%s/install.yaml", envoyGatewayVersion),
		"--ignore-not-found")
	_ = cmd.Run()
	return nil
}
