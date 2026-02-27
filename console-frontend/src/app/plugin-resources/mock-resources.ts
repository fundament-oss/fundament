import type { KubeResource } from './types';

export type MockResourceMap = Record<string, Record<string, KubeResource[]>>;

export const MOCK_RESOURCES: MockResourceMap = {
  'cert-manager': {
    Certificate: [
      {
        apiVersion: 'cert-manager.io/v1',
        kind: 'Certificate',
        metadata: {
          name: 'web-tls-cert',
          namespace: 'default',
          uid: 'cert-1',
          creationTimestamp: '2026-02-01T11:58:00Z',
        },
        spec: {
          secretName: 'web-tls-secret',
          issuerRef: {
            name: 'letsencrypt-prod',
            kind: 'ClusterIssuer',
            group: 'cert-manager.io',
          },
          dnsNames: ['example.com', '*.example.com'],
          ipAddresses: [],
          duration: '2160h',
          renewBefore: '720h',
          privateKey: {
            algorithm: 'RSA',
            size: 2048,
            encoding: 'PKCS1',
            rotationPolicy: 'Never',
          },
        },
        status: {
          conditions: [
            {
              type: 'Ready',
              status: 'True',
              lastTransitionTime: '2026-02-01T12:00:00Z',
              message: 'Certificate is up to date and has not expired',
            },
          ],
          notAfter: '2026-08-01T00:00:00Z',
          notBefore: '2026-02-01T00:00:00Z',
          renewalTime: '2026-07-02T00:00:00Z',
          revision: 1,
        },
      },
      {
        apiVersion: 'cert-manager.io/v1',
        kind: 'Certificate',
        metadata: {
          name: 'api-tls',
          namespace: 'prod',
          uid: 'cert-2',
          creationTimestamp: '2026-01-15T10:28:00Z',
        },
        spec: {
          secretName: 'api-tls-secret',
          issuerRef: {
            name: 'letsencrypt-prod',
            kind: 'ClusterIssuer',
            group: 'cert-manager.io',
          },
          dnsNames: ['api.example.com'],
          ipAddresses: [],
          duration: '2160h',
          renewBefore: '720h',
          privateKey: {
            algorithm: 'ECDSA',
            size: 256,
            encoding: 'PKCS8',
            rotationPolicy: 'Always',
          },
        },
        status: {
          conditions: [
            {
              type: 'Ready',
              status: 'True',
              lastTransitionTime: '2026-01-15T10:30:00Z',
              message: 'Certificate is up to date and has not expired',
            },
          ],
          notAfter: '2026-06-01T00:00:00Z',
          notBefore: '2026-01-15T00:00:00Z',
          renewalTime: '2026-05-02T00:00:00Z',
          revision: 1,
        },
      },
      {
        apiVersion: 'cert-manager.io/v1',
        kind: 'Certificate',
        metadata: {
          name: 'internal-cert',
          namespace: 'internal',
          uid: 'cert-3',
          creationTimestamp: '2026-02-10T08:55:00Z',
        },
        spec: {
          secretName: 'internal-tls-secret',
          issuerRef: {
            name: 'ca-issuer',
            kind: 'Issuer',
          },
          dnsNames: ['internal.local'],
          ipAddresses: ['10.0.0.1'],
          duration: '8760h',
          renewBefore: '2160h',
          privateKey: {
            algorithm: 'RSA',
            size: 4096,
            encoding: 'PKCS1',
            rotationPolicy: 'Never',
          },
        },
        status: {
          conditions: [
            {
              type: 'Ready',
              status: 'False',
              lastTransitionTime: '2026-02-10T09:00:00Z',
              message: 'Issuer ca-issuer not found',
            },
          ],
        },
      },
    ],
    Issuer: [
      {
        apiVersion: 'cert-manager.io/v1',
        kind: 'Issuer',
        metadata: {
          name: 'ca-issuer',
          namespace: 'default',
          uid: 'issuer-1',
          creationTimestamp: '2026-01-20T00:00:00Z',
        },
        spec: {
          ca: {
            secretName: 'ca-key-pair',
          },
        },
        status: {
          conditions: [
            {
              type: 'Ready',
              status: 'True',
              lastTransitionTime: '2026-01-20T00:01:00Z',
              message: 'Signing CA verified',
            },
          ],
        },
      },
    ],
    ClusterIssuer: [
      {
        apiVersion: 'cert-manager.io/v1',
        kind: 'ClusterIssuer',
        metadata: {
          name: 'letsencrypt-prod',
          uid: 'clusterissuer-1',
          creationTimestamp: '2026-01-15T00:00:00Z',
        },
        spec: {
          acme: {
            server: 'https://acme-v02.api.letsencrypt.org/directory',
            email: 'admin@example.com',
            privateKeySecretRef: { name: 'letsencrypt-prod-account-key' },
            solvers: [{ http01: { ingress: { ingressClassName: 'nginx' } } }],
          },
        },
        status: {
          conditions: [
            {
              type: 'Ready',
              status: 'True',
              lastTransitionTime: '2026-01-15T00:05:00Z',
              message: 'The ACME account was registered with the ACME server',
            },
          ],
        },
      },
      {
        apiVersion: 'cert-manager.io/v1',
        kind: 'ClusterIssuer',
        metadata: {
          name: 'letsencrypt-staging',
          uid: 'clusterissuer-2',
          creationTimestamp: '2026-01-15T00:00:00Z',
        },
        spec: {
          acme: {
            server: 'https://acme-staging-v02.api.letsencrypt.org/directory',
            email: 'admin@example.com',
            privateKeySecretRef: { name: 'letsencrypt-staging-account-key' },
            solvers: [{ http01: { ingress: { ingressClassName: 'nginx' } } }],
          },
        },
        status: {
          conditions: [
            {
              type: 'Ready',
              status: 'True',
              lastTransitionTime: '2026-01-15T00:05:00Z',
              message: 'The ACME account was registered with the staging server',
            },
          ],
        },
      },
      {
        apiVersion: 'cert-manager.io/v1',
        kind: 'ClusterIssuer',
        metadata: {
          name: 'selfsigned',
          uid: 'clusterissuer-3',
          creationTimestamp: '2026-01-10T00:00:00Z',
        },
        spec: {
          selfSigned: {},
        },
        status: {
          conditions: [
            {
              type: 'Ready',
              status: 'True',
              lastTransitionTime: '2026-01-10T00:01:00Z',
              message: '',
            },
          ],
        },
      },
    ],
  },
  'sample-plugin': {
    SampleItem: [
      {
        apiVersion: 'sample.fundament.io/v1',
        kind: 'SampleItem',
        metadata: {
          name: 'frontend',
          namespace: 'default',
          uid: 'demo-1',
          creationTimestamp: '2026-02-20T09:00:00Z',
        },
        spec: {
          image: 'nginx:1.27',
          replicas: 3,
          port: 80,
        },
        status: {
          phase: 'Running',
        },
      },
      {
        apiVersion: 'sample.fundament.io/v1',
        kind: 'SampleItem',
        metadata: {
          name: 'backend-api',
          namespace: 'default',
          uid: 'demo-2',
          creationTimestamp: '2026-02-21T14:30:00Z',
        },
        spec: {
          image: 'myregistry.io/api:v2.3.1',
          replicas: 2,
          port: 8080,
        },
        status: {
          phase: 'Running',
        },
      },
      {
        apiVersion: 'sample.fundament.io/v1',
        kind: 'SampleItem',
        metadata: {
          name: 'worker',
          namespace: 'jobs',
          uid: 'demo-3',
          creationTimestamp: '2026-02-25T07:15:00Z',
        },
        spec: {
          image: 'myregistry.io/worker:latest',
          replicas: 1,
        },
        status: {
          phase: 'Pending',
        },
      },
    ],
  },
  cnpg: {
    Database: [
      {
        apiVersion: 'postgresql.cnpg.io/v1',
        kind: 'Database',
        metadata: {
          name: 'app-db',
          namespace: 'default',
          uid: 'db-1',
          creationTimestamp: '2026-02-10T14:30:00Z',
        },
        spec: {
          name: 'app_database',
          cluster: { name: 'pg-cluster-1' },
          owner: 'app_user',
          encoding: 'UTF8',
          ensure: 'present',
          databaseReclaimPolicy: 'retain',
          allowConnections: true,
          connectionLimit: -1,
        },
        status: {
          applied: true,
          message: 'Database is up to date',
          observedGeneration: 1,
        },
      },
      {
        apiVersion: 'postgresql.cnpg.io/v1',
        kind: 'Database',
        metadata: {
          name: 'analytics-db',
          namespace: 'analytics',
          uid: 'db-2',
          creationTimestamp: '2026-02-12T09:15:00Z',
        },
        spec: {
          name: 'analytics',
          cluster: { name: 'pg-cluster-1' },
          owner: 'analytics_user',
          encoding: 'UTF8',
          ensure: 'present',
          databaseReclaimPolicy: 'retain',
          allowConnections: true,
          connectionLimit: 50,
        },
        status: {
          applied: true,
          message: 'Database is up to date',
          observedGeneration: 1,
        },
      },
      {
        apiVersion: 'postgresql.cnpg.io/v1',
        kind: 'Database',
        metadata: {
          name: 'staging-db',
          namespace: 'staging',
          uid: 'db-3',
          creationTimestamp: '2026-02-15T11:00:00Z',
        },
        spec: {
          name: 'staging_app',
          cluster: { name: 'pg-cluster-staging' },
          owner: 'staging_user',
          encoding: 'UTF8',
          ensure: 'present',
          databaseReclaimPolicy: 'delete',
          allowConnections: true,
          connectionLimit: -1,
        },
        status: {
          applied: false,
          message: 'Cluster pg-cluster-staging not found',
          observedGeneration: 1,
        },
      },
    ],
  },
};
