-- Test data for plugin short descriptions and extended descriptions

-- Grafana Alloy
UPDATE zappstore.plugins SET
    description_short = 'OpenTelemetry Collector distribution',
    description = '## Overview

Grafana Alloy is a flexible, high-performance OpenTelemetry Collector distribution with native Prometheus support.

## Key Features

- **OpenTelemetry Native**: Full support for OTLP and OpenTelemetry instrumentation
- **Prometheus Compatible**: Scrape Prometheus metrics and remote write to any compatible backend
- **Flexible Pipelines**: Build custom telemetry pipelines with a rich component library
- **Grafana Integration**: Seamless integration with Loki, Mimir, Tempo, and Grafana

## Use Cases

- Unified telemetry collection for metrics, logs, and traces
- Prometheus metrics collection and forwarding
- OpenTelemetry instrumentation aggregation'
WHERE id = '019b4000-3000-7000-8000-000000000001';

-- cert-manager
UPDATE zappstore.plugins SET
    description_short = 'Automatic TLS certificate management',
    description = '## Overview

cert-manager adds certificates and certificate issuers as resource types in Kubernetes clusters, and simplifies the process of obtaining, renewing and using those certificates.

## Key Features

- **Automated Issuance**: Automatically provision TLS certificates from various issuers
- **Multiple Issuers**: Support for Let''s Encrypt, HashiCorp Vault, Venafi, and more
- **Auto-Renewal**: Certificates are automatically renewed before expiry
- **Ingress Integration**: Native integration with Kubernetes Ingress resources

## Use Cases

- Automatic HTTPS for web applications
- Securing internal service-to-service communication
- Managing certificates for ingress controllers'
WHERE id = '019b4000-3000-7000-8000-000000000002';

-- CloudNativePG
UPDATE zappstore.plugins SET
    description_short = 'PostgreSQL operator for Kubernetes',
    description = '## Overview

CloudNativePG is an open source operator designed to manage PostgreSQL workloads on Kubernetes, covering the full lifecycle of a PostgreSQL cluster.

## Key Features

- **High Availability**: Automated failover and self-healing capabilities
- **Backup & Recovery**: Continuous backup to object storage with point-in-time recovery
- **Declarative Configuration**: Manage clusters using Kubernetes-native resources
- **Connection Pooling**: Built-in PgBouncer integration

## Use Cases

- Production PostgreSQL databases on Kubernetes
- Database-as-a-Service platforms
- Microservices requiring relational databases'
WHERE id = '019b4000-3000-7000-8000-000000000003';

-- ECK operator
UPDATE zappstore.plugins SET
    description_short = 'Elasticsearch and Kibana on Kubernetes',
    description = '## Overview

Elastic Cloud on Kubernetes (ECK) automates the deployment, provisioning, management, and orchestration of Elasticsearch, Kibana, and the Elastic Stack on Kubernetes.

## Key Features

- **Full Stack Support**: Deploy Elasticsearch, Kibana, APM Server, Beats, and more
- **Automated Operations**: Rolling upgrades, scaling, and configuration changes
- **Security Built-in**: TLS encryption and user authentication out of the box
- **Resource Management**: Automatic memory and storage configuration

## Use Cases

- Log aggregation and analysis
- Application performance monitoring
- Full-text search infrastructure'
WHERE id = '019b4000-3000-7000-8000-000000000004';

-- Grafana
UPDATE zappstore.plugins SET
    description_short = 'Metrics visualization and alerting',
    description = '## Overview

Grafana is the open source analytics and monitoring solution for every database. It allows you to query, visualize, alert on and understand your metrics no matter where they are stored.

## Key Features

- **Visualization**: Create stunning dashboards with a variety of visualization options
- **Alerting**: Define alert rules and get notified when metrics exceed thresholds
- **Data Sources**: Connect to multiple data sources including Prometheus, InfluxDB, and more
- **Plugins**: Extend functionality with a rich ecosystem of plugins

## Use Cases

- Infrastructure monitoring
- Application performance monitoring
- Business analytics
- IoT data visualization'
WHERE id = '019b4000-3000-7000-8000-000000000005';

-- Istio Gateway
UPDATE zappstore.plugins SET
    description_short = 'Ingress gateway for service mesh',
    description = '## Overview

Istio Gateway provides a dedicated ingress gateway for managing inbound traffic to your service mesh, offering advanced traffic management and security features.

## Key Features

- **Traffic Management**: Advanced routing, load balancing, and traffic splitting
- **TLS Termination**: Automatic certificate management and TLS termination
- **Gateway API Support**: Native support for Kubernetes Gateway API
- **Observability**: Built-in metrics, logging, and tracing

## Use Cases

- API gateway for microservices
- Multi-cluster ingress
- Canary deployments and A/B testing'
WHERE id = '019b4000-3000-7000-8000-000000000006';

-- Istio
UPDATE zappstore.plugins SET
    description_short = 'Service mesh for Kubernetes',
    description = '## Overview

Istio extends Kubernetes to establish a programmable, application-aware network. Working with both Kubernetes and traditional workloads, Istio brings standard, universal traffic management, telemetry, and security to complex deployments.

## Key Features

- **Traffic Management**: Fine-grained control of traffic behavior with routing rules
- **Security**: Automatic mTLS, authentication, and authorization policies
- **Observability**: Distributed tracing, monitoring, and logging
- **Resilience**: Timeouts, retries, circuit breakers, and fault injection

## Use Cases

- Microservices communication security
- Traffic management and load balancing
- Observability across services
- Zero-trust networking'
WHERE id = '019b4000-3000-7000-8000-000000000007';

-- Keycloak
UPDATE zappstore.plugins SET
    description_short = 'Identity and access management',
    description = '## Overview

Keycloak is an open source Identity and Access Management solution aimed at modern applications and services. It provides single sign-on, identity brokering, and user federation.

## Key Features

- **Single Sign-On**: SSO and Single Sign-Out for browser applications
- **Identity Brokering**: Connect with external identity providers via OIDC or SAML
- **User Federation**: Sync users from LDAP and Active Directory
- **Fine-Grained Authorization**: Role-based and attribute-based access control

## Use Cases

- Centralized authentication for applications
- API security and OAuth 2.0 provider
- User management and self-service registration'
WHERE id = '019b4000-3000-7000-8000-000000000008';

-- Grafana Loki
UPDATE zappstore.plugins SET
    description_short = 'Log aggregation system',
    description = '## Overview

Loki is a horizontally scalable, highly available, multi-tenant log aggregation system inspired by Prometheus. It is designed to be cost effective and easy to operate.

## Key Features

- **Label-Based Indexing**: Index logs by labels, not content, for cost efficiency
- **LogQL**: Powerful query language similar to PromQL
- **Scalable**: Horizontally scalable components for any workload size
- **Grafana Integration**: Native integration with Grafana for visualization

## Use Cases

- Kubernetes log aggregation
- Application log analysis
- Debugging and troubleshooting
- Compliance and audit logging'
WHERE id = '019b4000-3000-7000-8000-000000000009';

-- Grafana Mimir
UPDATE zappstore.plugins SET
    description_short = 'Long-term Prometheus storage',
    description = '## Overview

Grafana Mimir is an open source, horizontally scalable, highly available, multi-tenant, long-term storage for Prometheus metrics.

## Key Features

- **Unlimited Retention**: Store metrics for years with object storage backends
- **High Availability**: Built-in replication and automatic failover
- **Multi-Tenancy**: Isolate metrics data between tenants
- **100% Prometheus Compatible**: Drop-in replacement for Prometheus remote storage

## Use Cases

- Long-term metrics storage
- Multi-cluster Prometheus aggregation
- Metrics-as-a-Service platforms
- Historical analysis and capacity planning'
WHERE id = '019b4000-3000-7000-8000-00000000000a';

-- Pinniped
UPDATE zappstore.plugins SET
    description_short = 'Kubernetes cluster authentication',
    description = '## Overview

Pinniped provides identity services for Kubernetes clusters, enabling users to authenticate using external identity providers and providing a consistent login experience across clusters.

## Key Features

- **External Identity Providers**: Connect to OIDC providers and LDAP directories
- **Consistent Experience**: Same login flow across all your clusters
- **Credential Exchange**: Secure token exchange for cluster access
- **Multi-Cluster Support**: Centralized identity management for multiple clusters

## Use Cases

- Enterprise Kubernetes authentication
- Multi-cluster identity management
- Integration with corporate identity providers'
WHERE id = '019b4000-3000-7000-8000-00000000000b';

-- Sealed Secrets
UPDATE zappstore.plugins SET
    description_short = 'Encrypted secrets for Git',
    description = '## Overview

Sealed Secrets allows you to encrypt your Kubernetes secrets so they can be safely stored in Git repositories. Only the controller running in your cluster can decrypt them.

## Key Features

- **Asymmetric Encryption**: Secrets encrypted with public key, decrypted only in-cluster
- **GitOps Friendly**: Store encrypted secrets alongside your application manifests
- **Scoped Secrets**: Namespace and name scoping for additional security
- **Key Rotation**: Support for key rotation and re-encryption

## Use Cases

- GitOps workflows with sensitive data
- Secure secret distribution
- Compliance with secret management policies'
WHERE id = '019b4000-3000-7000-8000-00000000000c';

-- Grafana Tempo
UPDATE zappstore.plugins SET
    description_short = 'Distributed tracing backend',
    description = '## Overview

Grafana Tempo is an open source, easy-to-use, and high-scale distributed tracing backend. Tempo requires only object storage to operate and is deeply integrated with Grafana.

## Key Features

- **Cost Effective**: Uses only object storage, no complex dependencies
- **TraceQL**: Powerful query language for exploring traces
- **High Scale**: Designed for high-volume trace ingestion
- **Open Standards**: Native support for Jaeger, Zipkin, and OpenTelemetry

## Use Cases

- Distributed system debugging
- Request flow visualization
- Performance optimization
- Root cause analysis'
WHERE id = '019b4000-3000-7000-8000-00000000000d';
