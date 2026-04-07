package kube

// Mock resource instance lists, mirroring the former frontend mock-resources.ts

const mockPluginInstallationListJSON = `{
  "apiVersion": "plugins.fundament.io/v1",
  "kind": "PluginInstallationList",
  "metadata": {"resourceVersion": "1"},
  "items": [
    {
      "apiVersion": "plugins.fundament.io/v1",
      "kind": "PluginInstallation",
      "metadata": {"name": "cert-manager", "namespace": "plugin-cert-manager"},
      "spec": {"pluginName": "cert-manager", "version": "v1.17.2", "image": "mock"},
      "status": {"phase": "Running", "ready": true, "pluginVersion": "v1.17.2"}
    },
    {
      "apiVersion": "plugins.fundament.io/v1",
      "kind": "PluginInstallation",
      "metadata": {"name": "cnpg", "namespace": "plugin-cnpg"},
      "spec": {"pluginName": "cnpg", "version": "v1.25.1", "image": "mock"},
      "status": {"phase": "Running", "ready": true, "pluginVersion": "v1.25.1"}
    }
  ]
}`

const mockCertManagerDefinitionJSON = `{
  "apiVersion": "plugins.fundament.io/v1",
  "name": "cert-manager",
  "displayName": "Cert Manager",
  "version": "v1.17.2",
  "description": "Automated TLS certificate management for Kubernetes using cert-manager.",
  "author": "Fundament",
  "icon": "shield-check",
  "menu": {
    "project": [
      {"crd": "clusterissuers.cert-manager.io", "label": "Cluster Issuers", "icon": "shield-check"},
      {"crd": "certificates.cert-manager.io", "label": "Certificates", "icon": "certificate"},
      {"crd": "certificaterequests.cert-manager.io", "label": "Certificate Requests", "icon": "folder"}
    ]
  },
  "crds": ["clusterissuers.cert-manager.io", "certificates.cert-manager.io", "certificaterequests.cert-manager.io"]
}`

const mockCnpgDefinitionJSON = `{
  "apiVersion": "plugins.fundament.io/v1",
  "name": "cnpg",
  "displayName": "CNPG Databases",
  "version": "v1.25.1",
  "description": "Manage PostgreSQL databases via CloudNativePG.",
  "author": "Fundament",
  "icon": "database",
  "menu": {
    "project": [
      {"crd": "databases.postgresql.cnpg.io", "label": "Databases", "icon": "database"}
    ]
  },
  "crds": ["databases.postgresql.cnpg.io", "backups.postgresql.cnpg.io", "subscriptions.postgresql.cnpg.io"]
}`

const mockCertificateListJSON = `{
  "apiVersion": "cert-manager.io/v1",
  "kind": "CertificateList",
  "metadata": {"resourceVersion": "1"},
  "items": [
    {
      "apiVersion": "cert-manager.io/v1",
      "kind": "Certificate",
      "metadata": {"name": "web-tls-cert", "namespace": "default", "uid": "cert-1", "creationTimestamp": "2026-02-01T11:58:00Z"},
      "spec": {
        "secretName": "web-tls-secret",
        "issuerRef": {"name": "letsencrypt-prod", "kind": "ClusterIssuer", "group": "cert-manager.io"},
        "dnsNames": ["example.com", "*.example.com"],
        "duration": "2160h",
        "renewBefore": "720h",
        "privateKey": {"algorithm": "RSA", "size": 2048, "encoding": "PKCS1", "rotationPolicy": "Never"}
      },
      "status": {
        "conditions": [{"type": "Ready", "status": "True", "lastTransitionTime": "2026-02-01T12:00:00Z", "message": "Certificate is up to date and has not expired"}],
        "notAfter": "2026-08-01T00:00:00Z",
        "notBefore": "2026-02-01T00:00:00Z",
        "renewalTime": "2026-07-02T00:00:00Z",
        "revision": 1
      }
    },
    {
      "apiVersion": "cert-manager.io/v1",
      "kind": "Certificate",
      "metadata": {"name": "api-tls", "namespace": "prod", "uid": "cert-2", "creationTimestamp": "2026-01-15T10:28:00Z"},
      "spec": {
        "secretName": "api-tls-secret",
        "issuerRef": {"name": "letsencrypt-prod", "kind": "ClusterIssuer", "group": "cert-manager.io"},
        "dnsNames": ["api.example.com"],
        "duration": "2160h",
        "renewBefore": "720h",
        "privateKey": {"algorithm": "ECDSA", "size": 256, "encoding": "PKCS8", "rotationPolicy": "Always"}
      },
      "status": {
        "conditions": [{"type": "Ready", "status": "True", "lastTransitionTime": "2026-01-15T10:30:00Z", "message": "Certificate is up to date and has not expired"}],
        "notAfter": "2026-06-01T00:00:00Z",
        "notBefore": "2026-01-15T00:00:00Z",
        "renewalTime": "2026-05-02T00:00:00Z",
        "revision": 1
      }
    },
    {
      "apiVersion": "cert-manager.io/v1",
      "kind": "Certificate",
      "metadata": {"name": "internal-cert", "namespace": "internal", "uid": "cert-3", "creationTimestamp": "2026-02-10T08:55:00Z"},
      "spec": {
        "secretName": "internal-tls-secret",
        "issuerRef": {"name": "ca-issuer", "kind": "Issuer"},
        "dnsNames": ["internal.local"],
        "duration": "8760h",
        "renewBefore": "2160h",
        "privateKey": {"algorithm": "RSA", "size": 4096, "encoding": "PKCS1", "rotationPolicy": "Never"}
      },
      "status": {
        "conditions": [{"type": "Ready", "status": "False", "lastTransitionTime": "2026-02-10T09:00:00Z", "message": "Issuer ca-issuer not found"}]
      }
    }
  ]
}`

const mockIssuerListJSON = `{
  "apiVersion": "cert-manager.io/v1",
  "kind": "IssuerList",
  "metadata": {"resourceVersion": "1"},
  "items": [
    {
      "apiVersion": "cert-manager.io/v1",
      "kind": "Issuer",
      "metadata": {"name": "ca-issuer", "namespace": "default", "uid": "issuer-1", "creationTimestamp": "2026-01-20T00:00:00Z"},
      "spec": {"ca": {"secretName": "ca-key-pair"}},
      "status": {
        "conditions": [{"type": "Ready", "status": "True", "lastTransitionTime": "2026-01-20T00:01:00Z", "message": "Signing CA verified"}]
      }
    }
  ]
}`

const mockClusterIssuerListJSON = `{
  "apiVersion": "cert-manager.io/v1",
  "kind": "ClusterIssuerList",
  "metadata": {"resourceVersion": "1"},
  "items": [
    {
      "apiVersion": "cert-manager.io/v1",
      "kind": "ClusterIssuer",
      "metadata": {"name": "letsencrypt-prod", "uid": "clusterissuer-1", "creationTimestamp": "2026-01-15T00:00:00Z"},
      "spec": {
        "acme": {
          "server": "https://acme-v02.api.letsencrypt.org/directory",
          "email": "admin@example.com",
          "privateKeySecretRef": {"name": "letsencrypt-prod-account-key"}
        }
      },
      "status": {
        "conditions": [{"type": "Ready", "status": "True", "lastTransitionTime": "2026-01-15T00:05:00Z", "message": "The ACME account was registered with the ACME server"}]
      }
    },
    {
      "apiVersion": "cert-manager.io/v1",
      "kind": "ClusterIssuer",
      "metadata": {"name": "letsencrypt-staging", "uid": "clusterissuer-2", "creationTimestamp": "2026-01-15T00:00:00Z"},
      "spec": {
        "acme": {
          "server": "https://acme-staging-v02.api.letsencrypt.org/directory",
          "email": "admin@example.com",
          "privateKeySecretRef": {"name": "letsencrypt-staging-account-key"}
        }
      },
      "status": {
        "conditions": [{"type": "Ready", "status": "True", "lastTransitionTime": "2026-01-15T00:05:00Z", "message": "The ACME account was registered with the staging server"}]
      }
    },
    {
      "apiVersion": "cert-manager.io/v1",
      "kind": "ClusterIssuer",
      "metadata": {"name": "selfsigned", "uid": "clusterissuer-3", "creationTimestamp": "2026-01-10T00:00:00Z"},
      "spec": {"selfSigned": {}},
      "status": {
        "conditions": [{"type": "Ready", "status": "True", "lastTransitionTime": "2026-01-10T00:01:00Z", "message": ""}]
      }
    }
  ]
}`

const mockDatabaseListJSON = `{
  "apiVersion": "postgresql.cnpg.io/v1",
  "kind": "DatabaseList",
  "metadata": {"resourceVersion": "1"},
  "items": [
    {
      "apiVersion": "postgresql.cnpg.io/v1",
      "kind": "Database",
      "metadata": {"name": "app-db", "namespace": "default", "uid": "db-1", "creationTimestamp": "2026-02-10T14:30:00Z"},
      "spec": {
        "name": "app_database",
        "cluster": {"name": "pg-cluster-1"},
        "owner": "app_user",
        "encoding": "UTF8",
        "ensure": "present",
        "databaseReclaimPolicy": "retain",
        "allowConnections": true,
        "connectionLimit": -1
      },
      "status": {"applied": true, "message": "Database is up to date", "observedGeneration": 1}
    },
    {
      "apiVersion": "postgresql.cnpg.io/v1",
      "kind": "Database",
      "metadata": {"name": "analytics-db", "namespace": "analytics", "uid": "db-2", "creationTimestamp": "2026-02-12T09:15:00Z"},
      "spec": {
        "name": "analytics",
        "cluster": {"name": "pg-cluster-1"},
        "owner": "analytics_user",
        "encoding": "UTF8",
        "ensure": "present",
        "databaseReclaimPolicy": "retain",
        "allowConnections": true,
        "connectionLimit": 50
      },
      "status": {"applied": true, "message": "Database is up to date", "observedGeneration": 1}
    },
    {
      "apiVersion": "postgresql.cnpg.io/v1",
      "kind": "Database",
      "metadata": {"name": "staging-db", "namespace": "staging", "uid": "db-3", "creationTimestamp": "2026-02-15T11:00:00Z"},
      "spec": {
        "name": "staging_app",
        "cluster": {"name": "pg-cluster-staging"},
        "owner": "staging_user",
        "encoding": "UTF8",
        "ensure": "present",
        "databaseReclaimPolicy": "delete",
        "allowConnections": true,
        "connectionLimit": -1
      },
      "status": {"applied": false, "message": "Cluster pg-cluster-staging not found", "observedGeneration": 1}
    }
  ]
}`

const mockBackupListJSON = `{
  "apiVersion": "postgresql.cnpg.io/v1",
  "kind": "BackupList",
  "metadata": {"resourceVersion": "1"},
  "items": [
    {
      "apiVersion": "postgresql.cnpg.io/v1",
      "kind": "Backup",
      "metadata": {"name": "backup-20260301", "namespace": "default", "uid": "backup-1", "creationTimestamp": "2026-03-01T02:00:00Z"},
      "spec": {"cluster": {"name": "pg-cluster-1"}, "method": "barmanObjectStore"},
      "status": {"phase": "completed", "startedAt": "2026-03-01T02:00:05Z", "stoppedAt": "2026-03-01T02:04:32Z"}
    }
  ]
}`

const mockSubscriptionListJSON = `{
  "apiVersion": "postgresql.cnpg.io/v1",
  "kind": "SubscriptionList",
  "metadata": {"resourceVersion": "1"},
  "items": []
}`

const mockDemoItemListJSON = `{
  "apiVersion": "demo.fundament.io/v1",
  "kind": "DemoItemList",
  "metadata": {"resourceVersion": "1"},
  "items": [
    {
      "apiVersion": "demo.fundament.io/v1",
      "kind": "DemoItem",
      "metadata": {"name": "hello-world", "namespace": "default", "uid": "demoitem-1", "creationTimestamp": "2026-03-01T00:00:00Z"},
      "spec": {"message": "Hello, World!"}
    },
    {
      "apiVersion": "demo.fundament.io/v1",
      "kind": "DemoItem",
      "metadata": {"name": "greetings", "namespace": "default", "uid": "demoitem-2", "creationTimestamp": "2026-03-15T00:00:00Z"},
      "spec": {"message": "Greetings from the demo plugin!"}
    }
  ]
}`
