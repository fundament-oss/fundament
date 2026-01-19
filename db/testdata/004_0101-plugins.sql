-- Test data for zappstore plugins, tags, and categories

-- Categories
INSERT INTO zappstore.categories (id, name) VALUES
    ('019b4000-4000-7000-8000-000000000001', 'Observability'),
    ('019b4000-4000-7000-8000-000000000002', 'Security'),
    ('019b4000-4000-7000-8000-000000000003', 'Networking'),
    ('019b4000-4000-7000-8000-000000000004', 'Database'),
    ('019b4000-4000-7000-8000-000000000005', 'Identity');

-- Tags
INSERT INTO zappstore.tags (id, name) VALUES
    ('019b4000-5000-7000-8000-000000000001', 'Metrics'),
    ('019b4000-5000-7000-8000-000000000002', 'Logging'),
    ('019b4000-5000-7000-8000-000000000003', 'Tracing'),
    ('019b4000-5000-7000-8000-000000000004', 'Certificates'),
    ('019b4000-5000-7000-8000-000000000005', 'Service mesh'),
    ('019b4000-5000-7000-8000-000000000006', 'PostgreSQL'),
    ('019b4000-5000-7000-8000-000000000007', 'Elasticsearch'),
    ('019b4000-5000-7000-8000-000000000008', 'Authentication'),
    ('019b4000-5000-7000-8000-000000000009', 'Secrets'),
    ('019b4000-5000-7000-8000-00000000000a', 'Grafana stack');

-- Plugins
INSERT INTO zappstore.plugins (id, name, description) VALUES
    ('019b4000-3000-7000-8000-000000000001', 'Grafana Alloy', 'Grafana Grafana Alloy is a flexible, high performance OpenTelemetry Collector distribution'),
    ('019b4000-3000-7000-8000-000000000002', 'cert-manager', 'Automatically provision and manage TLS certificates in Kubernetes'),
    ('019b4000-3000-7000-8000-000000000003', 'CloudNativePG', 'CloudNativePG is an open source operator for PostgreSQL workloads'),
    ('019b4000-3000-7000-8000-000000000004', 'ECK operator', 'Elastic Cloud on Kubernetes - run Elasticsearch and Kibana on Kubernetes'),
    ('019b4000-3000-7000-8000-000000000005', 'Grafana', 'Query, visualize, alert on, and explore your metrics, logs, and traces'),
    ('019b4000-3000-7000-8000-000000000006', 'Istio Gateway', 'Istio ingress gateway for managing inbound traffic to your mesh'),
    ('019b4000-3000-7000-8000-000000000007', 'Istio', 'Connect, secure, control, and observe services with Istio service mesh'),
    ('019b4000-3000-7000-8000-000000000008', 'Keycloak', 'Open source Identity and access management solution'),
    ('019b4000-3000-7000-8000-000000000009', 'Grafana Loki', 'Horizontally-scalable, highly-available log aggregation system'),
    ('019b4000-3000-7000-8000-00000000000a', 'Grafana Mimir', 'Horizontally scalable, highly available, long-term storage for Prometheus metrics'),
    ('019b4000-3000-7000-8000-00000000000b', 'Pinniped', 'Authentication for Kubernetes clusters'),
    ('019b4000-3000-7000-8000-00000000000c', 'Sealed Secrets', 'Encrypt your secrets and safely store them in Git'),
    ('019b4000-3000-7000-8000-00000000000d', 'Grafana Tempo', 'High-scale distributed tracing backend');

-- Plugin-Category associations
INSERT INTO zappstore.categories_plugins (plugin_id, category_id) VALUES
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
    ('019b4000-3000-7000-8000-00000000000d', '019b4000-4000-7000-8000-000000000001');

-- Plugin-Tag associations
INSERT INTO zappstore.plugins_tags (plugin_id, tag_id) VALUES
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
    ('019b4000-3000-7000-8000-00000000000d', '019b4000-5000-7000-8000-00000000000a');
