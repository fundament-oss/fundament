-- Test data for appstore plugins, tags, and categories

-- Categories
INSERT INTO appstore.categories (id, name) VALUES
    ('019b4000-4000-7000-8000-000000000001', 'observability'),
    ('019b4000-4000-7000-8000-000000000002', 'security'),
    ('019b4000-4000-7000-8000-000000000003', 'networking'),
    ('019b4000-4000-7000-8000-000000000004', 'database'),
    ('019b4000-4000-7000-8000-000000000005', 'identity');

-- Tags
INSERT INTO appstore.tags (id, name) VALUES
    ('019b4000-5000-7000-8000-000000000001', 'metrics'),
    ('019b4000-5000-7000-8000-000000000002', 'logging'),
    ('019b4000-5000-7000-8000-000000000003', 'tracing'),
    ('019b4000-5000-7000-8000-000000000004', 'certificates'),
    ('019b4000-5000-7000-8000-000000000005', 'service-mesh'),
    ('019b4000-5000-7000-8000-000000000006', 'postgresql'),
    ('019b4000-5000-7000-8000-000000000007', 'elasticsearch'),
    ('019b4000-5000-7000-8000-000000000008', 'authentication'),
    ('019b4000-5000-7000-8000-000000000009', 'secrets'),
    ('019b4000-5000-7000-8000-00000000000a', 'grafana-stack');

-- Plugins
INSERT INTO appstore.plugins (id, name, description) VALUES
    ('019b4000-3000-7000-8000-000000000001', 'alloy', 'Grafana Alloy is a flexible, high performance OpenTelemetry Collector distribution'),
    ('019b4000-3000-7000-8000-000000000002', 'cert-manager', 'Automatically provision and manage TLS certificates in Kubernetes'),
    ('019b4000-3000-7000-8000-000000000003', 'cloudnative-pg', 'CloudNativePG is an open source operator for PostgreSQL workloads'),
    ('019b4000-3000-7000-8000-000000000004', 'eck-operator', 'Elastic Cloud on Kubernetes - run Elasticsearch and Kibana on Kubernetes'),
    ('019b4000-3000-7000-8000-000000000005', 'grafana', 'Query, visualize, alert on, and explore your metrics, logs, and traces'),
    ('019b4000-3000-7000-8000-000000000006', 'istio-gateway', 'Istio ingress gateway for managing inbound traffic to your mesh'),
    ('019b4000-3000-7000-8000-000000000007', 'istio', 'Connect, secure, control, and observe services with Istio service mesh'),
    ('019b4000-3000-7000-8000-000000000008', 'keycloak', 'Open source identity and access management solution'),
    ('019b4000-3000-7000-8000-000000000009', 'loki', 'Horizontally-scalable, highly-available log aggregation system'),
    ('019b4000-3000-7000-8000-00000000000a', 'mimir', 'Horizontally scalable, highly available, long-term storage for Prometheus metrics'),
    ('019b4000-3000-7000-8000-00000000000b', 'pinniped', 'Authentication for Kubernetes clusters'),
    ('019b4000-3000-7000-8000-00000000000c', 'sealed-secrets', 'Encrypt your secrets and safely store them in Git'),
    ('019b4000-3000-7000-8000-00000000000d', 'tempo', 'High-scale distributed tracing backend');

-- Plugin-Category associations
INSERT INTO appstore.categories_plugins (plugin_id, category_id) VALUES
    -- alloy -> observability
    ('019b4000-3000-7000-8000-000000000001', '019b4000-4000-7000-8000-000000000001'),
    -- cert-manager -> security
    ('019b4000-3000-7000-8000-000000000002', '019b4000-4000-7000-8000-000000000002'),
    -- cloudnative-pg -> database
    ('019b4000-3000-7000-8000-000000000003', '019b4000-4000-7000-8000-000000000004'),
    -- eck-operator -> database
    ('019b4000-3000-7000-8000-000000000004', '019b4000-4000-7000-8000-000000000004'),
    -- grafana -> observability
    ('019b4000-3000-7000-8000-000000000005', '019b4000-4000-7000-8000-000000000001'),
    -- istio-gateway -> networking
    ('019b4000-3000-7000-8000-000000000006', '019b4000-4000-7000-8000-000000000003'),
    -- istio -> networking
    ('019b4000-3000-7000-8000-000000000007', '019b4000-4000-7000-8000-000000000003'),
    -- keycloak -> identity
    ('019b4000-3000-7000-8000-000000000008', '019b4000-4000-7000-8000-000000000005'),
    -- loki -> observability
    ('019b4000-3000-7000-8000-000000000009', '019b4000-4000-7000-8000-000000000001'),
    -- mimir -> observability
    ('019b4000-3000-7000-8000-00000000000a', '019b4000-4000-7000-8000-000000000001'),
    -- pinniped -> identity
    ('019b4000-3000-7000-8000-00000000000b', '019b4000-4000-7000-8000-000000000005'),
    -- sealed-secrets -> security
    ('019b4000-3000-7000-8000-00000000000c', '019b4000-4000-7000-8000-000000000002'),
    -- tempo -> observability
    ('019b4000-3000-7000-8000-00000000000d', '019b4000-4000-7000-8000-000000000001');

-- Plugin-Tag associations
INSERT INTO appstore.plugins_tags (plugin_id, tag_id) VALUES
    -- alloy: metrics, logging, tracing, grafana-stack
    ('019b4000-3000-7000-8000-000000000001', '019b4000-5000-7000-8000-000000000001'),
    ('019b4000-3000-7000-8000-000000000001', '019b4000-5000-7000-8000-000000000002'),
    ('019b4000-3000-7000-8000-000000000001', '019b4000-5000-7000-8000-000000000003'),
    ('019b4000-3000-7000-8000-000000000001', '019b4000-5000-7000-8000-00000000000a'),
    -- cert-manager: certificates
    ('019b4000-3000-7000-8000-000000000002', '019b4000-5000-7000-8000-000000000004'),
    -- cloudnative-pg: postgresql
    ('019b4000-3000-7000-8000-000000000003', '019b4000-5000-7000-8000-000000000006'),
    -- eck-operator: elasticsearch
    ('019b4000-3000-7000-8000-000000000004', '019b4000-5000-7000-8000-000000000007'),
    -- grafana: metrics, logging, tracing, grafana-stack
    ('019b4000-3000-7000-8000-000000000005', '019b4000-5000-7000-8000-000000000001'),
    ('019b4000-3000-7000-8000-000000000005', '019b4000-5000-7000-8000-000000000002'),
    ('019b4000-3000-7000-8000-000000000005', '019b4000-5000-7000-8000-000000000003'),
    ('019b4000-3000-7000-8000-000000000005', '019b4000-5000-7000-8000-00000000000a'),
    -- istio-gateway: service-mesh
    ('019b4000-3000-7000-8000-000000000006', '019b4000-5000-7000-8000-000000000005'),
    -- istio: service-mesh
    ('019b4000-3000-7000-8000-000000000007', '019b4000-5000-7000-8000-000000000005'),
    -- keycloak: authentication
    ('019b4000-3000-7000-8000-000000000008', '019b4000-5000-7000-8000-000000000008'),
    -- loki: logging, grafana-stack
    ('019b4000-3000-7000-8000-000000000009', '019b4000-5000-7000-8000-000000000002'),
    ('019b4000-3000-7000-8000-000000000009', '019b4000-5000-7000-8000-00000000000a'),
    -- mimir: metrics, grafana-stack
    ('019b4000-3000-7000-8000-00000000000a', '019b4000-5000-7000-8000-000000000001'),
    ('019b4000-3000-7000-8000-00000000000a', '019b4000-5000-7000-8000-00000000000a'),
    -- pinniped: authentication
    ('019b4000-3000-7000-8000-00000000000b', '019b4000-5000-7000-8000-000000000008'),
    -- sealed-secrets: secrets
    ('019b4000-3000-7000-8000-00000000000c', '019b4000-5000-7000-8000-000000000009'),
    -- tempo: tracing, grafana-stack
    ('019b4000-3000-7000-8000-00000000000d', '019b4000-5000-7000-8000-000000000003'),
    ('019b4000-3000-7000-8000-00000000000d', '019b4000-5000-7000-8000-00000000000a');
