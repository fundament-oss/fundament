export type CertificateStatus = 'Ready' | 'NotReady' | 'Expired';

export type IssuerKind = 'Issuer' | 'ClusterIssuer';

export type IssuerType = 'ACME' | 'CA' | 'SelfSigned' | 'Vault';

export type SolverType = 'HTTP01' | 'DNS01';

export interface CertificateCondition {
  type: string;
  status: 'True' | 'False' | 'Unknown';
  lastTransitionTime: string;
  message: string;
}

export interface CertificateEvent {
  id: string;
  type: 'Issued' | 'Requested' | 'Created' | 'Renewal' | 'Failed';
  message: string;
  timestamp: string;
}

export interface Certificate {
  id: string;
  name: string;
  namespace: string;
  secretName: string;
  issuerName: string;
  issuerKind: IssuerKind;
  dnsNames: string[];
  ipAddresses: string[];
  uris: string[];
  status: CertificateStatus;
  duration: string;
  renewBefore: string;
  notBefore: string;
  notAfter: string;
  nextRenewal: string;
  privateKey: {
    algorithm: string;
    size: number;
    encoding: string;
    rotationPolicy: string;
  };
  conditions: CertificateCondition[];
  events: CertificateEvent[];
  created: string;
}

export interface Solver {
  type: SolverType;
  ingressClass?: string;
  provider?: string;
  selector?: string;
}

export interface Issuer {
  id: string;
  name: string;
  namespace: string;
  kind: IssuerKind;
  type: IssuerType;
  status: 'Ready' | 'NotReady';
  created: string;
  acme?: {
    server: string;
    email: string;
    privateKeySecret: string;
    solvers: Solver[];
  };
  ca?: {
    secretName: string;
  };
  conditions: CertificateCondition[];
}

export const MOCK_CERTIFICATES: Certificate[] = [
  {
    id: '1',
    name: 'web-tls-cert',
    namespace: 'default',
    secretName: 'web-tls-secret',
    issuerName: 'letsencrypt-prod',
    issuerKind: 'ClusterIssuer',
    dnsNames: ['example.com', '*.example.com'],
    ipAddresses: [],
    uris: [],
    status: 'Ready',
    duration: '2160h',
    renewBefore: '720h',
    notBefore: '2026-02-01T00:00:00Z',
    notAfter: '2026-08-01T00:00:00Z',
    nextRenewal: '2026-07-02T00:00:00Z',
    privateKey: {
      algorithm: 'RSA',
      size: 2048,
      encoding: 'PKCS1',
      rotationPolicy: 'Never',
    },
    conditions: [
      {
        type: 'Ready',
        status: 'True',
        lastTransitionTime: '2026-02-01T12:00:00Z',
        message: 'Certificate is up to date and has not expired',
      },
      {
        type: 'Issuing',
        status: 'False',
        lastTransitionTime: '2026-02-01T12:00:00Z',
        message: '',
      },
    ],
    events: [
      {
        id: 'e1',
        type: 'Issued',
        message: 'Certificate issued successfully',
        timestamp: '2026-02-01T12:00:00Z',
      },
      {
        id: 'e2',
        type: 'Requested',
        message: 'Certificate request created',
        timestamp: '2026-02-01T11:59:00Z',
      },
      {
        id: 'e3',
        type: 'Created',
        message: 'Certificate resource created',
        timestamp: '2026-02-01T11:58:00Z',
      },
    ],
    created: '2026-02-01T11:58:00Z',
  },
  {
    id: '2',
    name: 'api-tls',
    namespace: 'prod',
    secretName: 'api-tls-secret',
    issuerName: 'letsencrypt-prod',
    issuerKind: 'ClusterIssuer',
    dnsNames: ['api.example.com'],
    ipAddresses: [],
    uris: [],
    status: 'Ready',
    duration: '2160h',
    renewBefore: '720h',
    notBefore: '2026-01-15T00:00:00Z',
    notAfter: '2026-06-01T00:00:00Z',
    nextRenewal: '2026-05-02T00:00:00Z',
    privateKey: {
      algorithm: 'ECDSA',
      size: 256,
      encoding: 'PKCS8',
      rotationPolicy: 'Always',
    },
    conditions: [
      {
        type: 'Ready',
        status: 'True',
        lastTransitionTime: '2026-01-15T10:30:00Z',
        message: 'Certificate is up to date and has not expired',
      },
    ],
    events: [
      {
        id: 'e4',
        type: 'Issued',
        message: 'Certificate issued successfully',
        timestamp: '2026-01-15T10:30:00Z',
      },
      {
        id: 'e5',
        type: 'Created',
        message: 'Certificate resource created',
        timestamp: '2026-01-15T10:28:00Z',
      },
    ],
    created: '2026-01-15T10:28:00Z',
  },
  {
    id: '3',
    name: 'internal-cert',
    namespace: 'internal',
    secretName: 'internal-tls-secret',
    issuerName: 'ca-issuer',
    issuerKind: 'Issuer',
    dnsNames: ['internal.local'],
    ipAddresses: ['10.0.0.1'],
    uris: [],
    status: 'NotReady',
    duration: '8760h',
    renewBefore: '2160h',
    notBefore: '',
    notAfter: '',
    nextRenewal: '',
    privateKey: {
      algorithm: 'RSA',
      size: 4096,
      encoding: 'PKCS1',
      rotationPolicy: 'Never',
    },
    conditions: [
      {
        type: 'Ready',
        status: 'False',
        lastTransitionTime: '2026-02-10T09:00:00Z',
        message: 'Issuer ca-issuer not found',
      },
      {
        type: 'Issuing',
        status: 'True',
        lastTransitionTime: '2026-02-10T09:00:00Z',
        message: 'Waiting for issuer to become ready',
      },
    ],
    events: [
      {
        id: 'e6',
        type: 'Failed',
        message: 'Failed to issue certificate: issuer ca-issuer not found',
        timestamp: '2026-02-10T09:00:00Z',
      },
      {
        id: 'e7',
        type: 'Created',
        message: 'Certificate resource created',
        timestamp: '2026-02-10T08:55:00Z',
      },
    ],
    created: '2026-02-10T08:55:00Z',
  },
  {
    id: '4',
    name: 'old-staging-cert',
    namespace: 'staging',
    secretName: 'staging-tls-secret',
    issuerName: 'letsencrypt-staging',
    issuerKind: 'ClusterIssuer',
    dnsNames: ['staging.example.com'],
    ipAddresses: [],
    uris: [],
    status: 'Expired',
    duration: '2160h',
    renewBefore: '720h',
    notBefore: '2025-06-01T00:00:00Z',
    notAfter: '2025-12-01T00:00:00Z',
    nextRenewal: '',
    privateKey: {
      algorithm: 'RSA',
      size: 2048,
      encoding: 'PKCS1',
      rotationPolicy: 'Never',
    },
    conditions: [
      {
        type: 'Ready',
        status: 'False',
        lastTransitionTime: '2025-12-01T00:00:00Z',
        message: 'Certificate has expired',
      },
    ],
    events: [
      {
        id: 'e8',
        type: 'Failed',
        message: 'Certificate renewal failed: ACME challenge timed out',
        timestamp: '2025-11-01T10:00:00Z',
      },
      {
        id: 'e9',
        type: 'Issued',
        message: 'Certificate issued successfully',
        timestamp: '2025-06-01T12:00:00Z',
      },
    ],
    created: '2025-06-01T11:58:00Z',
  },
];

export const MOCK_ISSUERS: Issuer[] = [
  {
    id: '1',
    name: 'letsencrypt-prod',
    namespace: '',
    kind: 'ClusterIssuer',
    type: 'ACME',
    status: 'Ready',
    created: '2026-01-15T00:00:00Z',
    acme: {
      server: 'https://acme-v02.api.letsencrypt.org/directory',
      email: 'admin@example.com',
      privateKeySecret: 'letsencrypt-prod-account-key',
      solvers: [
        { type: 'HTTP01', ingressClass: 'nginx' },
        { type: 'DNS01', provider: 'cloudflare', selector: '*.example.com' },
      ],
    },
    conditions: [
      {
        type: 'Ready',
        status: 'True',
        lastTransitionTime: '2026-01-15T00:05:00Z',
        message: 'The ACME account was registered with the ACME server',
      },
    ],
  },
  {
    id: '2',
    name: 'letsencrypt-staging',
    namespace: '',
    kind: 'ClusterIssuer',
    type: 'ACME',
    status: 'Ready',
    created: '2026-01-15T00:00:00Z',
    acme: {
      server: 'https://acme-staging-v02.api.letsencrypt.org/directory',
      email: 'admin@example.com',
      privateKeySecret: 'letsencrypt-staging-account-key',
      solvers: [{ type: 'HTTP01', ingressClass: 'nginx' }],
    },
    conditions: [
      {
        type: 'Ready',
        status: 'True',
        lastTransitionTime: '2026-01-15T00:05:00Z',
        message: 'The ACME account was registered with the ACME server',
      },
    ],
  },
  {
    id: '3',
    name: 'ca-issuer',
    namespace: 'default',
    kind: 'Issuer',
    type: 'CA',
    status: 'Ready',
    created: '2026-01-20T00:00:00Z',
    ca: {
      secretName: 'ca-key-pair',
    },
    conditions: [
      {
        type: 'Ready',
        status: 'True',
        lastTransitionTime: '2026-01-20T00:01:00Z',
        message: 'Signing CA verified',
      },
    ],
  },
  {
    id: '4',
    name: 'selfsigned',
    namespace: '',
    kind: 'ClusterIssuer',
    type: 'SelfSigned',
    status: 'Ready',
    created: '2026-01-10T00:00:00Z',
    conditions: [
      {
        type: 'Ready',
        status: 'True',
        lastTransitionTime: '2026-01-10T00:01:00Z',
        message: '',
      },
    ],
  },
];
