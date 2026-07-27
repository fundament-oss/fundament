package main

import (
	"fmt"
)

const gatewayTemplate = `apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: %s
  namespace: %s
spec:
  gatewayClassName: istio
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
    - name: https
      protocol: HTTPS
      port: 443
      tls:
        mode: Terminate
        certificateRefs:
          - name: %s-tls
      allowedRoutes:
        namespaces:
          from: All
`

const gatewayWithCertManagerTemplate = `apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: %s
  namespace: %s
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt
spec:
  gatewayClassName: istio
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
    - name: https
      protocol: HTTPS
      port: 443
      tls:
        mode: Terminate
        certificateRefs:
          - name: %s-tls
      allowedRoutes:
        namespaces:
          from: All
`

func buildDefaultGateway(cfg pluginConfig, certManagerAvailable bool) []byte {
	if certManagerAvailable {
		return fmt.Appendf(nil, gatewayWithCertManagerTemplate, cfg.GatewayName, cfg.GatewayNamespace, cfg.GatewayName)
	}
	return fmt.Appendf(nil, gatewayTemplate, cfg.GatewayName, cfg.GatewayNamespace, cfg.GatewayName)
}
