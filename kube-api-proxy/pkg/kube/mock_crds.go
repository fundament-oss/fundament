package kube

import "strings"

// mockCRDForName returns the JSON for a specific CRD by its k8s metadata.name.
// Returns ("", false) for unknown names so callers can return a proper 404.
// If you add a CRD to mockCRDListJSON, add the corresponding case here too.
func mockCRDForName(name string) (string, bool) {
	switch name {
	case "certificates.cert-manager.io":
		return mockCertificateCRD, true
	case "issuers.cert-manager.io":
		return mockIssuerCRD, true
	case "clusterissuers.cert-manager.io":
		return mockClusterIssuerCRD, true
	case "databases.postgresql.cnpg.io":
		return mockDatabaseCRD, true
	case "backups.postgresql.cnpg.io":
		return mockBackupCRD, true
	case "subscriptions.postgresql.cnpg.io":
		return mockSubscriptionCRD, true
	case "demoitems.demo.fundament.io":
		return mockDemoItemCRD, true
	default:
		return "", false
	}
}

// mockCRDList is the response for GET /apis/apiextensions.k8s.io/v1/customresourcedefinitions
var mockCRDListJSON = `{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinitionList","metadata":{"resourceVersion":"1"},"items":[` +
	strings.Join([]string{
		mockCertificateCRD,
		mockIssuerCRD,
		mockClusterIssuerCRD,
		mockDatabaseCRD,
		mockBackupCRD,
		mockSubscriptionCRD,
		mockDemoItemCRD,
	}, ",") + `]}`

const mockCertificateCRD = `{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinition",
  "metadata": {"name": "certificates.cert-manager.io"},
  "spec": {
    "group": "cert-manager.io",
    "names": {"kind": "Certificate", "plural": "certificates", "singular": "certificate"},
    "scope": "Namespaced",
    "versions": [{
      "name": "v1",
      "served": true,
      "storage": true,
      "additionalPrinterColumns": [
        {"jsonPath": ".status.conditions[?(@.type == \"Ready\")].status", "name": "Ready", "type": "string"},
        {"jsonPath": ".spec.secretName", "name": "Secret", "type": "string"},
        {"jsonPath": ".spec.issuerRef.name", "name": "Issuer", "priority": 1, "type": "string"},
        {"jsonPath": ".status.conditions[?(@.type == \"Ready\")].message", "name": "Status", "priority": 1, "type": "string"},
        {"jsonPath": ".metadata.creationTimestamp", "name": "Age", "type": "date"}
      ],
      "schema": {
        "openAPIV3Schema": {
          "description": "A Certificate resource ensures an up to date signed X.509 certificate is stored in a Kubernetes Secret.",
          "type": "object",
          "properties": {
            "apiVersion": {"type": "string"},
            "kind": {"type": "string"},
            "metadata": {"type": "object"},
            "spec": {
              "type": "object",
              "required": ["issuerRef", "secretName"],
              "properties": {
                "secretName": {"description": "Name of the Secret resource that will be created and managed by this Certificate.", "type": "string"},
                "commonName": {"description": "Requested common name X509 certificate subject attribute.", "type": "string"},
                "dnsNames": {"description": "Requested DNS subject alternative names.", "type": "array", "items": {"type": "string"}},
                "ipAddresses": {"description": "Requested IP address subject alternative names.", "type": "array", "items": {"type": "string"}},
                "duration": {"description": "Requested lifetime of the Certificate.", "type": "string"},
                "renewBefore": {"description": "How long before expiry cert-manager should renew the certificate.", "type": "string"},
                "issuerRef": {
                  "description": "Reference to the issuer responsible for issuing the certificate.",
                  "type": "object",
                  "required": ["name"],
                  "properties": {
                    "name": {"description": "Name of the issuer.", "type": "string"},
                    "kind": {"description": "Kind of the issuer.", "type": "string"},
                    "group": {"description": "Group of the issuer.", "type": "string"}
                  }
                },
                "privateKey": {
                  "description": "Private key options.",
                  "type": "object",
                  "properties": {
                    "algorithm": {"description": "Private key algorithm.", "type": "string", "enum": ["RSA", "ECDSA", "Ed25519"]},
                    "size": {"description": "Key bit size of the private key.", "type": "integer"},
                    "encoding": {"description": "Private key encoding.", "type": "string", "enum": ["PKCS1", "PKCS8"]},
                    "rotationPolicy": {"description": "Controls how private keys should be regenerated.", "type": "string", "enum": ["Never", "Always"]}
                  }
                },
                "isCA": {"description": "Whether this certificate is a CA certificate.", "type": "boolean"}
              }
            },
            "status": {
              "type": "object",
              "properties": {
                "conditions": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "properties": {
                      "type": {"type": "string"},
                      "status": {"type": "string", "enum": ["True", "False", "Unknown"]},
                      "lastTransitionTime": {"type": "string", "format": "date-time"},
                      "message": {"type": "string"},
                      "reason": {"type": "string"}
                    }
                  }
                },
                "notAfter": {"type": "string", "format": "date-time"},
                "notBefore": {"type": "string", "format": "date-time"},
                "renewalTime": {"type": "string", "format": "date-time"},
                "revision": {"type": "integer"}
              }
            }
          }
        }
      }
    }]
  }
}`

const mockIssuerCRD = `{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinition",
  "metadata": {"name": "issuers.cert-manager.io"},
  "spec": {
    "group": "cert-manager.io",
    "names": {"kind": "Issuer", "plural": "issuers", "singular": "issuer"},
    "scope": "Namespaced",
    "versions": [{
      "name": "v1",
      "served": true,
      "storage": true,
      "additionalPrinterColumns": [
        {"jsonPath": ".status.conditions[?(@.type == \"Ready\")].status", "name": "Ready", "type": "string"},
        {"jsonPath": ".status.conditions[?(@.type == \"Ready\")].message", "name": "Status", "priority": 1, "type": "string"},
        {"jsonPath": ".metadata.creationTimestamp", "name": "Age", "type": "date"}
      ],
      "schema": {
        "openAPIV3Schema": {
          "description": "An Issuer represents a certificate issuing authority.",
          "type": "object",
          "properties": {
            "apiVersion": {"type": "string"},
            "kind": {"type": "string"},
            "metadata": {"type": "object"},
            "spec": {
              "type": "object",
              "properties": {
                "acme": {
                  "description": "ACME configures this issuer to communicate with an ACME server.",
                  "type": "object",
                  "properties": {
                    "server": {"description": "URL of the ACME server directory endpoint.", "type": "string"},
                    "email": {"description": "Email address for the ACME account.", "type": "string"},
                    "privateKeySecretRef": {
                      "description": "Reference to a Secret containing the ACME account private key.",
                      "type": "object",
                      "required": ["name"],
                      "properties": {"name": {"description": "Name of the Secret.", "type": "string"}}
                    }
                  }
                },
                "ca": {
                  "description": "CA configures this issuer to sign certificates using a signing CA keypair.",
                  "type": "object",
                  "properties": {"secretName": {"description": "Name of the Secret containing the CA signing keypair.", "type": "string"}}
                },
                "selfSigned": {"description": "SelfSigned configures this issuer to self-sign certificates.", "type": "object", "properties": {}}
              }
            },
            "status": {
              "type": "object",
              "properties": {
                "conditions": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "properties": {
                      "type": {"type": "string"},
                      "status": {"type": "string"},
                      "lastTransitionTime": {"type": "string", "format": "date-time"},
                      "message": {"type": "string"}
                    }
                  }
                }
              }
            }
          }
        }
      }
    }]
  }
}`

const mockClusterIssuerCRD = `{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinition",
  "metadata": {"name": "clusterissuers.cert-manager.io"},
  "spec": {
    "group": "cert-manager.io",
    "names": {"kind": "ClusterIssuer", "plural": "clusterissuers", "singular": "clusterissuer"},
    "scope": "Cluster",
    "versions": [{
      "name": "v1",
      "served": true,
      "storage": true,
      "additionalPrinterColumns": [
        {"jsonPath": ".status.conditions[?(@.type == \"Ready\")].status", "name": "Ready", "type": "string"},
        {"jsonPath": ".status.conditions[?(@.type == \"Ready\")].message", "name": "Status", "priority": 1, "type": "string"},
        {"jsonPath": ".metadata.creationTimestamp", "name": "Age", "type": "date"}
      ],
      "schema": {
        "openAPIV3Schema": {
          "description": "A ClusterIssuer represents a cluster-wide certificate issuing authority.",
          "type": "object",
          "properties": {
            "apiVersion": {"type": "string"},
            "kind": {"type": "string"},
            "metadata": {"type": "object"},
            "spec": {
              "type": "object",
              "properties": {
                "acme": {
                  "description": "ACME configures this issuer to communicate with an ACME server.",
                  "type": "object",
                  "properties": {
                    "server": {"description": "URL of the ACME server directory endpoint.", "type": "string"},
                    "email": {"description": "Email address for the ACME account.", "type": "string"},
                    "privateKeySecretRef": {
                      "description": "Reference to a Secret containing the ACME account private key.",
                      "type": "object",
                      "required": ["name"],
                      "properties": {"name": {"description": "Name of the Secret.", "type": "string"}}
                    }
                  }
                },
                "ca": {
                  "description": "CA configures this issuer to sign certificates using a signing CA keypair.",
                  "type": "object",
                  "properties": {"secretName": {"description": "Name of the Secret containing the CA signing keypair.", "type": "string"}}
                },
                "selfSigned": {"description": "SelfSigned configures this issuer to self-sign certificates.", "type": "object", "properties": {}}
              }
            },
            "status": {
              "type": "object",
              "properties": {
                "conditions": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "properties": {
                      "type": {"type": "string"},
                      "status": {"type": "string"},
                      "lastTransitionTime": {"type": "string", "format": "date-time"},
                      "message": {"type": "string"}
                    }
                  }
                }
              }
            }
          }
        }
      }
    }]
  }
}`

const mockDatabaseCRD = `{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinition",
  "metadata": {"name": "databases.postgresql.cnpg.io"},
  "spec": {
    "group": "postgresql.cnpg.io",
    "names": {"kind": "Database", "plural": "databases", "singular": "database"},
    "scope": "Namespaced",
    "versions": [{
      "name": "v1",
      "served": true,
      "storage": true,
      "additionalPrinterColumns": [
        {"jsonPath": ".metadata.creationTimestamp", "name": "Age", "type": "date"},
        {"jsonPath": ".spec.cluster.name", "name": "Cluster", "type": "string"},
        {"jsonPath": ".spec.name", "name": "PG Name", "type": "string"},
        {"jsonPath": ".status.applied", "name": "Applied", "type": "boolean"},
        {"jsonPath": ".status.message", "name": "Message", "type": "string", "description": "Latest reconciliation message"}
      ],
      "schema": {
        "openAPIV3Schema": {
          "description": "Database is the Schema for the databases API",
          "type": "object",
          "required": ["metadata", "spec"],
          "properties": {
            "apiVersion": {"type": "string"},
            "kind": {"type": "string"},
            "metadata": {"type": "object"},
            "spec": {
              "description": "Specification of the desired Database.",
              "type": "object",
              "required": ["cluster", "name", "owner"],
              "properties": {
                "name": {"description": "The name of the database to create inside PostgreSQL.", "type": "string"},
                "cluster": {
                  "description": "The name of the PostgreSQL cluster hosting the database.",
                  "type": "object",
                  "properties": {"name": {"description": "Name of the cluster.", "type": "string"}}
                },
                "owner": {"description": "The role name of the user who owns the database.", "type": "string"},
                "encoding": {"description": "Character set encoding to use in the database.", "type": "string"},
                "allowConnections": {"description": "If false then no one can connect to this database.", "type": "boolean"},
                "connectionLimit": {"description": "How many concurrent connections can be made. -1 means no limit.", "type": "integer"},
                "ensure": {
                  "description": "Ensure the database is present or absent.",
                  "type": "string",
                  "default": "present",
                  "enum": ["present", "absent"]
                },
                "databaseReclaimPolicy": {
                  "description": "The policy for end-of-life maintenance of this database.",
                  "type": "string",
                  "default": "retain",
                  "enum": ["delete", "retain"]
                }
              }
            },
            "status": {
              "description": "Most recently observed status of the Database.",
              "type": "object",
              "properties": {
                "applied": {"description": "Applied is true if the database was reconciled correctly.", "type": "boolean"},
                "message": {"description": "Message is the reconciliation output message.", "type": "string"},
                "observedGeneration": {"description": "A sequence number representing the latest desired state that was synchronized.", "type": "integer", "format": "int64"}
              }
            }
          }
        }
      }
    }]
  }
}`

const mockBackupCRD = `{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinition",
  "metadata": {"name": "backups.postgresql.cnpg.io"},
  "spec": {
    "group": "postgresql.cnpg.io",
    "names": {"kind": "Backup", "plural": "backups", "singular": "backup"},
    "scope": "Namespaced",
    "versions": [{
      "name": "v1",
      "served": true,
      "storage": true,
      "additionalPrinterColumns": [
        {"jsonPath": ".metadata.creationTimestamp", "name": "Age", "type": "date"},
        {"jsonPath": ".spec.cluster.name", "name": "Cluster", "type": "string"},
        {"jsonPath": ".spec.method", "name": "Method", "type": "string"},
        {"jsonPath": ".status.phase", "name": "Phase", "type": "string"},
        {"jsonPath": ".status.error", "name": "Error", "type": "string"}
      ],
      "schema": {
        "openAPIV3Schema": {
          "description": "A Backup resource is a request for a PostgreSQL backup.",
          "type": "object",
          "required": ["metadata", "spec"],
          "properties": {
            "apiVersion": {"type": "string"},
            "kind": {"type": "string"},
            "metadata": {"type": "object"},
            "spec": {
              "description": "Specification of the desired behavior of the backup.",
              "type": "object",
              "required": ["cluster"],
              "properties": {
                "cluster": {
                  "description": "The cluster to backup",
                  "type": "object",
                  "properties": {"name": {"description": "Name of the referent.", "type": "string"}}
                },
                "method": {
                  "description": "The backup method to be used.",
                  "type": "string",
                  "default": "barmanObjectStore",
                  "enum": ["barmanObjectStore", "volumeSnapshot", "plugin"]
                },
                "online": {"description": "Whether the default type of backup with volume snapshots is online/hot.", "type": "boolean"}
              }
            },
            "status": {
              "type": "object",
              "properties": {
                "phase": {"description": "The last backup status", "type": "string"},
                "error": {"description": "The detected error", "type": "string"},
                "startedAt": {"description": "When the backup was started", "type": "string", "format": "date-time"},
                "stoppedAt": {"description": "When the backup was terminated", "type": "string", "format": "date-time"}
              }
            }
          }
        }
      }
    }]
  }
}`

const mockSubscriptionCRD = `{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinition",
  "metadata": {"name": "subscriptions.postgresql.cnpg.io"},
  "spec": {
    "group": "postgresql.cnpg.io",
    "names": {"kind": "Subscription", "plural": "subscriptions", "singular": "subscription"},
    "scope": "Namespaced",
    "versions": [{
      "name": "v1",
      "served": true,
      "storage": true,
      "additionalPrinterColumns": [
        {"jsonPath": ".metadata.creationTimestamp", "name": "Age", "type": "date"},
        {"jsonPath": ".spec.cluster.name", "name": "Cluster", "type": "string"},
        {"jsonPath": ".spec.name", "name": "PG Name", "type": "string"},
        {"jsonPath": ".status.applied", "name": "Applied", "type": "boolean"},
        {"jsonPath": ".status.message", "name": "Message", "type": "string", "description": "Latest reconciliation message"}
      ],
      "schema": {
        "openAPIV3Schema": {
          "description": "Subscription is the Schema for the subscriptions API",
          "type": "object",
          "required": ["metadata", "spec"],
          "properties": {
            "apiVersion": {"type": "string"},
            "kind": {"type": "string"},
            "metadata": {"type": "object"},
            "spec": {
              "description": "SubscriptionSpec defines the desired state of Subscription",
              "type": "object",
              "required": ["cluster", "dbname", "externalClusterName", "name", "publicationName"],
              "properties": {
                "name": {"description": "The name of the subscription inside PostgreSQL", "type": "string"},
                "cluster": {
                  "description": "The name of the PostgreSQL cluster that identifies the subscriber",
                  "type": "object",
                  "properties": {"name": {"description": "Name of the referent.", "type": "string"}}
                },
                "dbname": {"description": "The name of the database where the publication will be installed", "type": "string"},
                "externalClusterName": {"description": "The name of the external cluster with the publication", "type": "string"},
                "publicationName": {"description": "The name of the publication inside the PostgreSQL database", "type": "string"},
                "subscriptionReclaimPolicy": {
                  "description": "The policy for end-of-life maintenance of this subscription",
                  "type": "string",
                  "default": "retain",
                  "enum": ["delete", "retain"]
                }
              }
            },
            "status": {
              "type": "object",
              "properties": {
                "applied": {"description": "Applied is true if the subscription was reconciled correctly", "type": "boolean"},
                "message": {"description": "Message is the reconciliation output message", "type": "string"},
                "observedGeneration": {"description": "A sequence number representing the latest desired state", "type": "integer", "format": "int64"}
              }
            }
          }
        }
      }
    }]
  }
}`

const mockDemoItemCRD = `{
  "apiVersion": "apiextensions.k8s.io/v1",
  "kind": "CustomResourceDefinition",
  "metadata": {"name": "demoitems.demo.fundament.io"},
  "spec": {
    "group": "demo.fundament.io",
    "names": {"kind": "DemoItem", "plural": "demoitems", "singular": "demoitem"},
    "scope": "Namespaced",
    "versions": [{
      "name": "v1",
      "served": true,
      "storage": true,
      "schema": {
        "openAPIV3Schema": {
          "type": "object",
          "properties": {
            "apiVersion": {"type": "string"},
            "kind": {"type": "string"},
            "metadata": {"type": "object"},
            "spec": {
              "type": "object",
              "properties": {
                "message": {"description": "A greeting message", "type": "string"}
              }
            }
          }
        }
      }
    }]
  }
}`
