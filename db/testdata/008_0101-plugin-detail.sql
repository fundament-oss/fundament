-- Test data for plugin details (author info, repository, documentation links)

-- Update plugins with author and repository information
-- Alloy
UPDATE appstore.plugins SET
    author_name = 'Grafana Labs',
    author_url = 'https://grafana.com',
    repository_url = 'https://github.com/grafana/alloy'
WHERE id = '019b4000-3000-7000-8000-000000000001';

-- Cert-manager
UPDATE appstore.plugins SET
    author_name = 'cert-manager maintainers',
    author_url = 'https://cert-manager.io',
    repository_url = 'https://github.com/cert-manager/cert-manager'
WHERE id = '019b4000-3000-7000-8000-000000000002';

-- Cloudnative-pg
UPDATE appstore.plugins SET
    author_name = 'CloudNativePG Contributors',
    author_url = 'https://cloudnative-pg.io',
    repository_url = 'https://github.com/cloudnative-pg/cloudnative-pg'
WHERE id = '019b4000-3000-7000-8000-000000000003';

-- ECK-operator
UPDATE appstore.plugins SET
    author_name = 'Elastic',
    author_url = 'https://www.elastic.co',
    repository_url = 'https://github.com/elastic/cloud-on-k8s'
WHERE id = '019b4000-3000-7000-8000-000000000004';

-- Grafana
UPDATE appstore.plugins SET
    author_name = 'Grafana Labs',
    author_url = 'https://grafana.com',
    repository_url = 'https://github.com/grafana/grafana'
WHERE id = '019b4000-3000-7000-8000-000000000005';

-- Istio-gateway
UPDATE appstore.plugins SET
    author_name = 'Istio Authors',
    author_url = 'https://istio.io',
    repository_url = 'https://github.com/istio/istio'
WHERE id = '019b4000-3000-7000-8000-000000000006';

-- Istio
UPDATE appstore.plugins SET
    author_name = 'Istio Authors',
    author_url = 'https://istio.io',
    repository_url = 'https://github.com/istio/istio'
WHERE id = '019b4000-3000-7000-8000-000000000007';

-- Keycloak
UPDATE appstore.plugins SET
    author_name = 'Red Hat',
    author_url = 'https://www.keycloak.org',
    repository_url = 'https://github.com/keycloak/keycloak'
WHERE id = '019b4000-3000-7000-8000-000000000008';

-- Loki
UPDATE appstore.plugins SET
    author_name = 'Grafana Labs',
    author_url = 'https://grafana.com',
    repository_url = 'https://github.com/grafana/loki'
WHERE id = '019b4000-3000-7000-8000-000000000009';

-- Mimir
UPDATE appstore.plugins SET
    author_name = 'Grafana Labs',
    author_url = 'https://grafana.com',
    repository_url = 'https://github.com/grafana/mimir'
WHERE id = '019b4000-3000-7000-8000-00000000000a';

-- Pinniped
UPDATE appstore.plugins SET
    author_name = 'VMware',
    author_url = 'https://pinniped.dev',
    repository_url = 'https://github.com/vmware-tanzu/pinniped'
WHERE id = '019b4000-3000-7000-8000-00000000000b';

-- Sealed-secrets
UPDATE appstore.plugins SET
    author_name = 'Bitnami',
    author_url = 'https://bitnami.com',
    repository_url = 'https://github.com/bitnami-labs/sealed-secrets'
WHERE id = '019b4000-3000-7000-8000-00000000000c';

-- Tempo
UPDATE appstore.plugins SET
    author_name = 'Grafana Labs',
    author_url = 'https://grafana.com',
    repository_url = 'https://github.com/grafana/tempo'
WHERE id = '019b4000-3000-7000-8000-00000000000d';


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
    ('019b4000-6000-7000-8000-0000000000c2', '019b4000-3000-7000-8000-00000000000d', 'TraceQL Reference', 'TraceQL query reference', 'https://grafana.com/docs/tempo/latest/traceql/');
