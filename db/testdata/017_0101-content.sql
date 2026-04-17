-- Seed image values for plugins (added by migration 017)
UPDATE appstore.plugins SET image = 'docker.io/grafana/alloy:v1.8.3'                             WHERE name = 'Grafana Alloy';
UPDATE appstore.plugins SET image = 'quay.io/jetstack/cert-manager-controller:v1.17.2'           WHERE name = 'cert-manager';
UPDATE appstore.plugins SET image = 'ghcr.io/cloudnative-pg/cloudnative-pg:v1.25.1'              WHERE name = 'CloudNativePG';
UPDATE appstore.plugins SET image = 'docker.elastic.co/eck/eck-operator:2.16.0'                  WHERE name = 'ECK operator';
UPDATE appstore.plugins SET image = 'docker.io/grafana/grafana:11.6.0'                           WHERE name = 'Grafana';
UPDATE appstore.plugins SET image = 'docker.io/istio/pilot:1.24.3'                               WHERE name = 'Istio Gateway';
UPDATE appstore.plugins SET image = 'docker.io/istio/pilot:1.24.3'                               WHERE name = 'Istio';
UPDATE appstore.plugins SET image = 'quay.io/keycloak/keycloak:26.2.0'                           WHERE name = 'Keycloak';
UPDATE appstore.plugins SET image = 'docker.io/grafana/loki:3.4.3'                               WHERE name = 'Grafana Loki';
UPDATE appstore.plugins SET image = 'docker.io/grafana/mimir:2.15.0'                             WHERE name = 'Grafana Mimir';
UPDATE appstore.plugins SET image = 'ghcr.io/vmware-tanzu/pinniped/pinniped-concierge:v0.35.0'   WHERE name = 'Pinniped';
UPDATE appstore.plugins SET image = 'ghcr.io/bitnami-labs/sealed-secrets:v0.28.1'                WHERE name = 'Sealed Secrets';
UPDATE appstore.plugins SET image = 'docker.io/grafana/tempo:2.7.2'                              WHERE name = 'Grafana Tempo';
