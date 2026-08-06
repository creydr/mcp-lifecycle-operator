# Gateway Integration Examples

This directory contains examples for exposing MCP servers through gateways using the operator's built-in providers.

## HTTPRoute Provider

The `httproute` provider creates a [Gateway API HTTPRoute](https://gateway-api.sigs.k8s.io/api-types/httproute/) to route traffic from an existing Gateway to the MCP server.

### Prerequisites

- Kubernetes cluster
- [Gateway API CRDs](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api) installed **before** the operator starts (the operator checks for HTTPRoute at startup and skips the controller if the CRD is absent)
- MCP Lifecycle Operator installed (install or restart after the Gateway API CRDs are in place)
- A Gateway resource deployed and managed by a gateway controller (e.g., Envoy Gateway, Istio, Cilium)

### Deploy

```bash
kubectl apply -f gateway-config.yaml
kubectl apply -f mcpserver-with-gateway.yaml
```

### Configuration

Edit `gateway-config.yaml` to match your environment:

| Key                 | Description                              |
|---------------------|------------------------------------------|
| `gateway-name`      | Name of your Gateway resource            |
| `gateway-namespace` | Namespace where the Gateway lives        |
| `hostname`          | Hostname for routing (optional)          |

### Verify

```bash
kubectl get mcpgatewaybindings -n default
kubectl get httproutes -n default
kubectl get mcpserver kubernetes-mcp-server -n default -o jsonpath='{.status.address.url}'
```

### Cleanup

```bash
kubectl delete -f mcpserver-with-gateway.yaml
kubectl delete -f gateway-config.yaml
```

## Kuadrant Provider

The `kuadrant` provider creates an HTTPRoute and an [MCPServerRegistration](https://github.com/Kuadrant/mcp-gateway) to register the MCP server with Kuadrant's mcp-gateway for tool aggregation.

### Prerequisites

- Kubernetes cluster
- [Gateway API CRDs](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api) installed
- [Kuadrant mcp-gateway](https://github.com/Kuadrant/mcp-gateway) installed (provides the MCPServerRegistration CRD)
- MCP Lifecycle Operator installed (install or restart after the CRDs above are in place)
- A Gateway resource deployed and managed by a gateway controller

### Deploy

```bash
kubectl apply -f kuadrant-config.yaml
kubectl apply -f mcpserver-with-kuadrant.yaml
```

### Configuration

Edit `kuadrant-config.yaml` to match your environment:

| Key                 | Description                                        |
|---------------------|----------------------------------------------------|
| `gateway-name`      | Name of your Gateway resource                      |
| `gateway-namespace` | Namespace where the Gateway lives                  |
| `hostname`          | Hostname for routing (optional)                    |
| `tool-prefix`       | Prefix for federated tool names (optional)         |

### Verify

```bash
kubectl get mcpgatewaybindings -n default
kubectl get httproutes -n default
kubectl get mcpserverregistrations.mcp.kuadrant.io -n default
kubectl get mcpserver kubernetes-mcp-server -n default -o jsonpath='{.status.address.url}'
```

### Cleanup

```bash
kubectl delete -f mcpserver-with-kuadrant.yaml
kubectl delete -f kuadrant-config.yaml
```