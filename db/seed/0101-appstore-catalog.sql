-- Appstore plugin catalog — reference data.
--
-- Applied in ALL environments (incl. production) by the db-migrations Job
-- (charts/fundament/templates/db-migrations.yaml), independently of
-- TREK_INSERT_TEST_DATA. This file is the source of truth for the plugin
-- catalog; the dev-only tenant fixtures (orgs/users/clusters) stay in
-- testdata/001_0101-content.sql.
--
-- Each plugin, category, tag, preset and documentation-link row is an idempotent
-- upsert keyed on the stable UUIDs below, so the Job can run this unconditionally
-- on every deploy: a fresh database gets the catalog, and an existing one has
-- those rows reconciled to whatever this file says (added plugins inserted,
-- edited descriptions/links refreshed).
--
-- The association rows (categories_plugins, plugins_tags, preset_plugins) are
-- insert-only: new links are added, but removing a link, re-categorizing a
-- plugin, or dropping one from a preset is NOT reconciled here and needs a
-- dedicated migration.

-- Categories
INSERT INTO appstore.categories (id, name) VALUES
    ('019b4000-4000-7000-8000-000000000001', 'Observability'),
    ('019b4000-4000-7000-8000-000000000002', 'Security'),
    ('019b4000-4000-7000-8000-000000000003', 'Networking'),
    ('019b4000-4000-7000-8000-000000000004', 'Database'),
    ('019b4000-4000-7000-8000-000000000005', 'Identity')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name;

-- Tags
INSERT INTO appstore.tags (id, name) VALUES
    ('019b4000-5000-7000-8000-000000000001', 'Metrics'),
    ('019b4000-5000-7000-8000-000000000002', 'Logging'),
    ('019b4000-5000-7000-8000-000000000003', 'Tracing'),
    ('019b4000-5000-7000-8000-000000000004', 'Certificates'),
    ('019b4000-5000-7000-8000-000000000005', 'Service mesh'),
    ('019b4000-5000-7000-8000-000000000006', 'PostgreSQL'),
    ('019b4000-5000-7000-8000-000000000007', 'Elasticsearch'),
    ('019b4000-5000-7000-8000-000000000008', 'Authentication'),
    ('019b4000-5000-7000-8000-000000000009', 'Secrets'),
    ('019b4000-5000-7000-8000-00000000000a', 'Grafana stack')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name;

-- Plugins
INSERT INTO appstore.plugins (id, name, display_name, description_short, description, author_name, author_url, repository_url, image) VALUES
    ('019b4000-3000-7000-8000-000000000001', 'grafana-alloy', 'Grafana Alloy', 'OpenTelemetry Collector distribution', '## Overview

Grafana Alloy is a flexible, high-performance OpenTelemetry Collector distribution with native Prometheus support.

## Key Features

- **OpenTelemetry Native**: Full support for OTLP and OpenTelemetry instrumentation
- **Prometheus Compatible**: Scrape Prometheus metrics and remote write to any compatible backend
- **Flexible Pipelines**: Build custom telemetry pipelines with a rich component library
- **Grafana Integration**: Seamless integration with Loki, Mimir, Tempo, and Grafana

## Use Cases

- Unified telemetry collection for metrics, logs, and traces
- Prometheus metrics collection and forwarding
- OpenTelemetry instrumentation aggregation', 'Grafana Labs', 'https://grafana.com', 'https://github.com/grafana/alloy', 'docker.io/grafana/alloy:v1.8.3'),
    ('019b4000-3000-7000-8000-000000000002', 'cert-manager', 'cert-manager', 'Automatic TLS certificate management', '## Overview

cert-manager adds certificates and certificate issuers as resource types in Kubernetes clusters, and simplifies the process of obtaining, renewing and using those certificates.

## Key Features

- **Automated Issuance**: Automatically provision TLS certificates from various issuers
- **Multiple Issuers**: Support for Let''s Encrypt, HashiCorp Vault, Venafi, and more
- **Auto-Renewal**: Certificates are automatically renewed before expiry
- **Ingress Integration**: Native integration with Kubernetes Ingress resources

## Use Cases

- Automatic HTTPS for web applications
- Securing internal service-to-service communication
- Managing certificates for ingress controllers', 'cert-manager maintainers', 'https://cert-manager.io', 'https://github.com/cert-manager/cert-manager', 'quay.io/jetstack/cert-manager-controller:v1.17.2'),
    ('019b4000-3000-7000-8000-000000000003', 'cloudnative-pg', 'CloudNativePG', 'PostgreSQL operator for Kubernetes', '## Overview

CloudNativePG is an open source operator designed to manage PostgreSQL workloads on Kubernetes, covering the full lifecycle of a PostgreSQL cluster.

## Key Features

- **High Availability**: Automated failover and self-healing capabilities
- **Backup & Recovery**: Continuous backup to object storage with point-in-time recovery
- **Declarative Configuration**: Manage clusters using Kubernetes-native resources
- **Connection Pooling**: Built-in PgBouncer integration

## Use Cases

- Production PostgreSQL databases on Kubernetes
- Database-as-a-Service platforms
- Microservices requiring relational databases', 'CloudNativePG Contributors', 'https://cloudnative-pg.io', 'https://github.com/cloudnative-pg/cloudnative-pg', 'ghcr.io/cloudnative-pg/cloudnative-pg:v1.25.1'),
    ('019b4000-3000-7000-8000-000000000004', 'eck-operator', 'ECK operator', 'Elasticsearch and Kibana on Kubernetes', '## Overview

Elastic Cloud on Kubernetes (ECK) automates the deployment, provisioning, management, and orchestration of Elasticsearch, Kibana, and the Elastic Stack on Kubernetes.

## Key Features

- **Full Stack Support**: Deploy Elasticsearch, Kibana, APM Server, Beats, and more
- **Automated Operations**: Rolling upgrades, scaling, and configuration changes
- **Security Built-in**: TLS encryption and user authentication out of the box
- **Resource Management**: Automatic memory and storage configuration

## Use Cases

- Log aggregation and analysis
- Application performance monitoring
- Full-text search infrastructure', 'Elastic', 'https://www.elastic.co', 'https://github.com/elastic/cloud-on-k8s', 'docker.elastic.co/eck/eck-operator:2.16.0'),
    ('019b4000-3000-7000-8000-000000000005', 'grafana', 'Grafana', 'Metrics visualization and alerting', '## Overview

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
- IoT data visualization', 'Grafana Labs', 'https://grafana.com', 'https://github.com/grafana/grafana', 'docker.io/grafana/grafana:11.6.0'),
    ('019b4000-3000-7000-8000-000000000006', 'istio-gateway', 'Istio Gateway', 'Ingress gateway for service mesh', '## Overview

Istio Gateway provides a dedicated ingress gateway for managing inbound traffic to your service mesh, offering advanced traffic management and security features.

## Key Features

- **Traffic Management**: Advanced routing, load balancing, and traffic splitting
- **TLS Termination**: Automatic certificate management and TLS termination
- **Gateway API Support**: Native support for Kubernetes Gateway API
- **Observability**: Built-in metrics, logging, and tracing

## Use Cases

- API gateway for microservices
- Multi-cluster ingress
- Canary deployments and A/B testing', 'Istio Authors', 'https://istio.io', 'https://github.com/istio/istio', 'docker.io/istio/pilot:1.24.3'),
    ('019b4000-3000-7000-8000-000000000007', 'istio', 'Istio', 'Service mesh for Kubernetes', '## Overview

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
- Zero-trust networking', 'Istio Authors', 'https://istio.io', 'https://github.com/istio/istio', 'docker.io/istio/pilot:1.24.3'),
    ('019b4000-3000-7000-8000-000000000008', 'keycloak', 'Keycloak', 'Identity and access management', '## Overview

Keycloak is an open source Identity and Access Management solution aimed at modern applications and services. It provides single sign-on, identity brokering, and user federation.

## Key Features

- **Single Sign-On**: SSO and Single Sign-Out for browser applications
- **Identity Brokering**: Connect with external identity providers via OIDC or SAML
- **User Federation**: Sync users from LDAP and Active Directory
- **Fine-Grained Authorization**: Role-based and attribute-based access control

## Use Cases

- Centralized authentication for applications
- API security and OAuth 2.0 provider
- User management and self-service registration', 'Red Hat', 'https://www.keycloak.org', 'https://github.com/keycloak/keycloak', 'quay.io/keycloak/keycloak:26.2.0'),
    ('019b4000-3000-7000-8000-000000000009', 'grafana-loki', 'Grafana Loki', 'Log aggregation system', '## Overview

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
- Compliance and audit logging', 'Grafana Labs', 'https://grafana.com', 'https://github.com/grafana/loki', 'docker.io/grafana/loki:3.4.3'),
    ('019b4000-3000-7000-8000-00000000000a', 'grafana-mimir', 'Grafana Mimir', 'Long-term Prometheus storage', '## Overview

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
- Historical analysis and capacity planning', 'Grafana Labs', 'https://grafana.com', 'https://github.com/grafana/mimir', 'docker.io/grafana/mimir:2.15.0'),
    ('019b4000-3000-7000-8000-00000000000b', 'pinniped', 'Pinniped', 'Kubernetes cluster authentication', '## Overview

Pinniped provides identity services for Kubernetes clusters, enabling users to authenticate using external identity providers and providing a consistent login experience across clusters.

## Key Features

- **External Identity Providers**: Connect to OIDC providers and LDAP directories
- **Consistent Experience**: Same login flow across all your clusters
- **Credential Exchange**: Secure token exchange for cluster access
- **Multi-Cluster Support**: Centralized identity management for multiple clusters

## Use Cases

- Enterprise Kubernetes authentication
- Multi-cluster identity management
- Integration with corporate identity providers', 'VMware', 'https://pinniped.dev', 'https://github.com/vmware-tanzu/pinniped', 'ghcr.io/vmware-tanzu/pinniped/pinniped-concierge:v0.35.0'),
    ('019b4000-3000-7000-8000-00000000000c', 'sealed-secrets', 'Sealed Secrets', 'Encrypted secrets for Git', '## Overview

Sealed Secrets allows you to encrypt your Kubernetes secrets so they can be safely stored in Git repositories. Only the controller running in your cluster can decrypt them.

## Key Features

- **Asymmetric Encryption**: Secrets encrypted with public key, decrypted only in-cluster
- **GitOps Friendly**: Store encrypted secrets alongside your application manifests
- **Scoped Secrets**: Namespace and name scoping for additional security
- **Key Rotation**: Support for key rotation and re-encryption

## Use Cases

- GitOps workflows with sensitive data
- Secure secret distribution
- Compliance with secret management policies', 'Bitnami', 'https://bitnami.com', 'https://github.com/bitnami-labs/sealed-secrets', 'ghcr.io/bitnami-labs/sealed-secrets:v0.28.1'),
    ('019b4000-3000-7000-8000-00000000000d', 'grafana-tempo', 'Grafana Tempo', 'Distributed tracing backend', '## Overview

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
- Root cause analysis', 'Grafana Labs', 'https://grafana.com', 'https://github.com/grafana/tempo', 'docker.io/grafana/tempo:2.7.2'),
    ('019b4000-3000-7000-8000-00000000000e', 'openfsc', 'OpenFSC', 'Federated Service Connectivity', '## Overview

OpenFSC brings Federated Service Connectivity (FSC) to Kubernetes. It installs the standalone openfsc-operator; each team declares an FSCInstallation in its namespace to run an OpenFSC peer there.

## Key Features

- **Federated Connectivity**: Join an existing FSC group or act as your own group Directory
- **Per-Namespace Peers**: Each team runs its own OpenFSC peer via a namespaced FSCInstallation
- **Operator-Managed**: The openfsc-operator reconciles peers, gateways, and certificates
- **Common Ground**: Built on the FSC standard for trusted data exchange between organizations

## Use Cases

- Inter-organization service connectivity
- Common Ground / data exchange between government bodies
- Federated APIs across trust domains', 'Fundament', 'https://fsc-standaard.nl', 'https://gitlab.com/rinis-oss/fsc/open-fsc', '')
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    display_name = EXCLUDED.display_name,
    description_short = EXCLUDED.description_short,
    description = EXCLUDED.description,
    author_name = EXCLUDED.author_name,
    author_url = EXCLUDED.author_url,
    repository_url = EXCLUDED.repository_url,
    image = EXCLUDED.image;

-- Plugin-Category associations
INSERT INTO appstore.categories_plugins (plugin_id, category_id) VALUES
    -- Grafana Alloy -> Observability
    ('019b4000-3000-7000-8000-000000000001', '019b4000-4000-7000-8000-000000000001'),
    -- cert-manager -> Security
    ('019b4000-3000-7000-8000-000000000002', '019b4000-4000-7000-8000-000000000002'),
    -- CloudNativePG -> Database
    ('019b4000-3000-7000-8000-000000000003', '019b4000-4000-7000-8000-000000000004'),
    -- ECK operator -> Database
    ('019b4000-3000-7000-8000-000000000004', '019b4000-4000-7000-8000-000000000004'),
    -- Grafana -> Observability
    ('019b4000-3000-7000-8000-000000000005', '019b4000-4000-7000-8000-000000000001'),
    -- Istio Gateway -> Networking
    ('019b4000-3000-7000-8000-000000000006', '019b4000-4000-7000-8000-000000000003'),
    -- Istio -> Networking
    ('019b4000-3000-7000-8000-000000000007', '019b4000-4000-7000-8000-000000000003'),
    -- Keycloak -> Identity
    ('019b4000-3000-7000-8000-000000000008', '019b4000-4000-7000-8000-000000000005'),
    -- Grafana Loki -> Observability
    ('019b4000-3000-7000-8000-000000000009', '019b4000-4000-7000-8000-000000000001'),
    -- Grafana Mimir -> Observability
    ('019b4000-3000-7000-8000-00000000000a', '019b4000-4000-7000-8000-000000000001'),
    -- Pinniped -> Identity
    ('019b4000-3000-7000-8000-00000000000b', '019b4000-4000-7000-8000-000000000005'),
    -- Sealed Secrets -> Security
    ('019b4000-3000-7000-8000-00000000000c', '019b4000-4000-7000-8000-000000000002'),
    -- Grafana Tempo -> Observability
    ('019b4000-3000-7000-8000-00000000000d', '019b4000-4000-7000-8000-000000000001'),
    -- OpenFSC -> Networking
    ('019b4000-3000-7000-8000-00000000000e', '019b4000-4000-7000-8000-000000000003')
ON CONFLICT (plugin_id, category_id) DO NOTHING;

-- Plugin-Tag associations
INSERT INTO appstore.plugins_tags (plugin_id, tag_id) VALUES
    -- Grafana Alloy: Metrics, Logging, Tracing, Grafana stack
    ('019b4000-3000-7000-8000-000000000001', '019b4000-5000-7000-8000-000000000001'),
    ('019b4000-3000-7000-8000-000000000001', '019b4000-5000-7000-8000-000000000002'),
    ('019b4000-3000-7000-8000-000000000001', '019b4000-5000-7000-8000-000000000003'),
    ('019b4000-3000-7000-8000-000000000001', '019b4000-5000-7000-8000-00000000000a'),
    -- cert-manager: Certificates
    ('019b4000-3000-7000-8000-000000000002', '019b4000-5000-7000-8000-000000000004'),
    -- CloudNativePG: PostgreSQL
    ('019b4000-3000-7000-8000-000000000003', '019b4000-5000-7000-8000-000000000006'),
    -- ECK operator: Elasticsearch
    ('019b4000-3000-7000-8000-000000000004', '019b4000-5000-7000-8000-000000000007'),
    -- Grafana: Metrics, Logging, Tracing, Grafana stack
    ('019b4000-3000-7000-8000-000000000005', '019b4000-5000-7000-8000-000000000001'),
    ('019b4000-3000-7000-8000-000000000005', '019b4000-5000-7000-8000-000000000002'),
    ('019b4000-3000-7000-8000-000000000005', '019b4000-5000-7000-8000-000000000003'),
    ('019b4000-3000-7000-8000-000000000005', '019b4000-5000-7000-8000-00000000000a'),
    -- Istio Gateway: Service mesh
    ('019b4000-3000-7000-8000-000000000006', '019b4000-5000-7000-8000-000000000005'),
    -- Istio: Service mesh
    ('019b4000-3000-7000-8000-000000000007', '019b4000-5000-7000-8000-000000000005'),
    -- Keycloak: Authentication
    ('019b4000-3000-7000-8000-000000000008', '019b4000-5000-7000-8000-000000000008'),
    -- Grafana Loki: Logging, Grafana stack
    ('019b4000-3000-7000-8000-000000000009', '019b4000-5000-7000-8000-000000000002'),
    ('019b4000-3000-7000-8000-000000000009', '019b4000-5000-7000-8000-00000000000a'),
    -- Grafana Mimir: Metrics, Grafana stack
    ('019b4000-3000-7000-8000-00000000000a', '019b4000-5000-7000-8000-000000000001'),
    ('019b4000-3000-7000-8000-00000000000a', '019b4000-5000-7000-8000-00000000000a'),
    -- Pinniped: Authentication
    ('019b4000-3000-7000-8000-00000000000b', '019b4000-5000-7000-8000-000000000008'),
    -- Sealed Secrets: Secrets
    ('019b4000-3000-7000-8000-00000000000c', '019b4000-5000-7000-8000-000000000009'),
    -- Grafana Tempo: Tracing, Grafana stack
    ('019b4000-3000-7000-8000-00000000000d', '019b4000-5000-7000-8000-000000000003'),
    ('019b4000-3000-7000-8000-00000000000d', '019b4000-5000-7000-8000-00000000000a'),
    -- OpenFSC: Service mesh
    ('019b4000-3000-7000-8000-00000000000e', '019b4000-5000-7000-8000-000000000005')
ON CONFLICT (plugin_id, tag_id) DO NOTHING;

-- Test data for presets
INSERT INTO appstore.presets (id, name, description) VALUES
    ('019b4000-4000-7000-8000-000000000001', 'Haven+', 'Full Haven+ stack with all plugins enabled'),
    ('019b4000-4000-7000-8000-000000000002', 'Observability', 'Monitoring and logging stack')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description;

-- Haven+ preset: all plugins
INSERT INTO appstore.preset_plugins (preset_id, plugin_id) VALUES
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-000000000001'), -- Grafana Alloy
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-000000000002'), -- cert-manager
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-000000000003'), -- CloudNativePG
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-000000000004'), -- ECK operator
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-000000000005'), -- Grafana
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-000000000006'), -- Istio Gateway
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-000000000007'), -- Istio
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-000000000008'), -- Keycloak
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-000000000009'), -- Grafana Loki
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-00000000000a'), -- Grafana Mimir
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-00000000000b'), -- Pinniped
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-00000000000c'), -- Sealed Secrets
    ('019b4000-4000-7000-8000-000000000001', '019b4000-3000-7000-8000-00000000000d') -- Grafana Tempo
ON CONFLICT (preset_id, plugin_id) DO NOTHING;

-- Observability preset: Grafana, Grafana Loki, Grafana Mimir, Grafana Tempo, Grafana Alloy
INSERT INTO appstore.preset_plugins (preset_id, plugin_id) VALUES
    ('019b4000-4000-7000-8000-000000000002', '019b4000-3000-7000-8000-000000000001'), -- Grafana Alloy
    ('019b4000-4000-7000-8000-000000000002', '019b4000-3000-7000-8000-000000000005'), -- Grafana
    ('019b4000-4000-7000-8000-000000000002', '019b4000-3000-7000-8000-000000000009'), -- Grafana Loki
    ('019b4000-4000-7000-8000-000000000002', '019b4000-3000-7000-8000-00000000000a'), -- Grafana Mimir
    ('019b4000-4000-7000-8000-000000000002', '019b4000-3000-7000-8000-00000000000d') -- Grafana Tempo
ON CONFLICT (preset_id, plugin_id) DO NOTHING;

-- Documentation links (using 019b4000-6000-7000-8000-* prefix for doc link IDs)
INSERT INTO appstore.plugin_documentation_links (id, plugin_id, title, url_name, url) VALUES
    -- Alloy documentation
    ('019b4000-6000-7000-8000-000000000001', '019b4000-3000-7000-8000-000000000001', 'Documentation', 'Official documentation', 'https://grafana.com/docs/alloy/latest/'),
    ('019b4000-6000-7000-8000-000000000002', '019b4000-3000-7000-8000-000000000001', 'Getting Started', 'Get started guide', 'https://grafana.com/docs/alloy/latest/get-started/'),
    ('019b4000-6000-7000-8000-000000000003', '019b4000-3000-7000-8000-000000000001', 'Configuration Reference', 'Configuration reference', 'https://grafana.com/docs/alloy/latest/reference/'),

    -- Cert-manager documentation
    ('019b4000-6000-7000-8000-000000000010', '019b4000-3000-7000-8000-000000000002', 'Documentation', 'Official documentation', 'https://cert-manager.io/docs/'),
    ('019b4000-6000-7000-8000-000000000011', '019b4000-3000-7000-8000-000000000002', 'Installation Guide', 'Installation guide', 'https://cert-manager.io/docs/installation/'),
    ('019b4000-6000-7000-8000-000000000012', '019b4000-3000-7000-8000-000000000002', 'Issuer Configuration', 'Issuer configuration', 'https://cert-manager.io/docs/configuration/'),

    -- CloudNativePG documentation
    ('019b4000-6000-7000-8000-000000000020', '019b4000-3000-7000-8000-000000000003', 'Documentation', 'Official documentation', 'https://cloudnative-pg.io/documentation/'),
    ('019b4000-6000-7000-8000-000000000021', '019b4000-3000-7000-8000-000000000003', 'Quickstart', 'Quickstart guide', 'https://cloudnative-pg.io/documentation/current/quickstart/'),
    ('019b4000-6000-7000-8000-000000000022', '019b4000-3000-7000-8000-000000000003', 'Architecture', 'Architecture overview', 'https://cloudnative-pg.io/documentation/current/architecture/'),

    -- ECK-operator documentation
    ('019b4000-6000-7000-8000-000000000030', '019b4000-3000-7000-8000-000000000004', 'Documentation', 'Official documentation', 'https://www.elastic.co/guide/en/cloud-on-k8s/current/index.html'),
    ('019b4000-6000-7000-8000-000000000031', '019b4000-3000-7000-8000-000000000004', 'Quickstart', 'Quickstart guide', 'https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-quickstart.html'),

    -- Grafana documentation
    ('019b4000-6000-7000-8000-000000000040', '019b4000-3000-7000-8000-000000000005', 'Documentation', 'Official documentation', 'https://grafana.com/docs/grafana/latest/'),
    ('019b4000-6000-7000-8000-000000000041', '019b4000-3000-7000-8000-000000000005', 'Getting Started', 'Get started guide', 'https://grafana.com/docs/grafana/latest/getting-started/'),
    ('019b4000-6000-7000-8000-000000000042', '019b4000-3000-7000-8000-000000000005', 'Dashboards', 'Dashboard documentation', 'https://grafana.com/docs/grafana/latest/dashboards/'),

    -- Istio-gateway documentation
    ('019b4000-6000-7000-8000-000000000050', '019b4000-3000-7000-8000-000000000006', 'Gateway Documentation', 'Gateway documentation', 'https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/'),
    ('019b4000-6000-7000-8000-000000000051', '019b4000-3000-7000-8000-000000000006', 'Gateway API', 'Gateway API guide', 'https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/'),

    -- Istio documentation
    ('019b4000-6000-7000-8000-000000000060', '019b4000-3000-7000-8000-000000000007', 'Documentation', 'Official documentation', 'https://istio.io/latest/docs/'),
    ('019b4000-6000-7000-8000-000000000061', '019b4000-3000-7000-8000-000000000007', 'Getting Started', 'Get started guide', 'https://istio.io/latest/docs/setup/getting-started/'),
    ('019b4000-6000-7000-8000-000000000062', '019b4000-3000-7000-8000-000000000007', 'Traffic Management', 'Traffic management concepts', 'https://istio.io/latest/docs/concepts/traffic-management/'),

    -- Keycloak documentation
    ('019b4000-6000-7000-8000-000000000070', '019b4000-3000-7000-8000-000000000008', 'Documentation', 'Official documentation', 'https://www.keycloak.org/documentation'),
    ('019b4000-6000-7000-8000-000000000071', '019b4000-3000-7000-8000-000000000008', 'Getting Started', 'Kubernetes quickstart', 'https://www.keycloak.org/getting-started/getting-started-kube'),
    ('019b4000-6000-7000-8000-000000000072', '019b4000-3000-7000-8000-000000000008', 'Admin Guide', 'Server administration guide', 'https://www.keycloak.org/docs/latest/server_admin/'),

    -- Loki documentation
    ('019b4000-6000-7000-8000-000000000080', '019b4000-3000-7000-8000-000000000009', 'Documentation', 'Official documentation', 'https://grafana.com/docs/loki/latest/'),
    ('019b4000-6000-7000-8000-000000000081', '019b4000-3000-7000-8000-000000000009', 'Getting Started', 'Get started guide', 'https://grafana.com/docs/loki/latest/get-started/'),
    ('019b4000-6000-7000-8000-000000000082', '019b4000-3000-7000-8000-000000000009', 'LogQL Reference', 'LogQL query reference', 'https://grafana.com/docs/loki/latest/logql/'),

    -- Mimir documentation
    ('019b4000-6000-7000-8000-000000000090', '019b4000-3000-7000-8000-00000000000a', 'Documentation', 'Official documentation', 'https://grafana.com/docs/mimir/latest/'),
    ('019b4000-6000-7000-8000-000000000091', '019b4000-3000-7000-8000-00000000000a', 'Getting Started', 'Get started guide', 'https://grafana.com/docs/mimir/latest/get-started/'),
    ('019b4000-6000-7000-8000-000000000092', '019b4000-3000-7000-8000-00000000000a', 'Architecture', 'Architecture reference', 'https://grafana.com/docs/mimir/latest/references/architecture/'),

    -- Pinniped documentation
    ('019b4000-6000-7000-8000-0000000000a0', '019b4000-3000-7000-8000-00000000000b', 'Documentation', 'Official documentation', 'https://pinniped.dev/docs/'),
    ('019b4000-6000-7000-8000-0000000000a1', '019b4000-3000-7000-8000-00000000000b', 'Getting Started', 'How-to guides', 'https://pinniped.dev/docs/howto/'),
    ('019b4000-6000-7000-8000-0000000000a2', '019b4000-3000-7000-8000-00000000000b', 'Architecture', 'Architecture overview', 'https://pinniped.dev/docs/background/architecture/'),

    -- Sealed-secrets documentation
    ('019b4000-6000-7000-8000-0000000000b0', '019b4000-3000-7000-8000-00000000000c', 'Documentation', 'GitHub README', 'https://github.com/bitnami-labs/sealed-secrets#readme'),
    ('019b4000-6000-7000-8000-0000000000b1', '019b4000-3000-7000-8000-00000000000c', 'Installation', 'Installation instructions', 'https://github.com/bitnami-labs/sealed-secrets#installation'),

    -- Tempo documentation
    ('019b4000-6000-7000-8000-0000000000c0', '019b4000-3000-7000-8000-00000000000d', 'Documentation', 'Official documentation', 'https://grafana.com/docs/tempo/latest/'),
    ('019b4000-6000-7000-8000-0000000000c1', '019b4000-3000-7000-8000-00000000000d', 'Getting Started', 'Get started guide', 'https://grafana.com/docs/tempo/latest/getting-started/'),
    ('019b4000-6000-7000-8000-0000000000c2', '019b4000-3000-7000-8000-00000000000d', 'TraceQL Reference', 'TraceQL query reference', 'https://grafana.com/docs/tempo/latest/traceql/')
ON CONFLICT (id) DO UPDATE SET
    plugin_id = EXCLUDED.plugin_id,
    title = EXCLUDED.title,
    url_name = EXCLUDED.url_name,
    url = EXCLUDED.url;
