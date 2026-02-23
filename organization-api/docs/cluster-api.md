# Cluster API

The Cluster API provides CRUD operations for managing Kubernetes clusters. It uses Connect RPC (gRPC-compatible) over HTTP.

## Base URL

```
http://localhost:8081
```

## Authentication

All requests require a JWT Bearer token in the `Authorization` header.

```bash
export TOKEN="your-jwt-token"
```

## Endpoints

All endpoints use `POST` with JSON bodies. The Content-Type should be `application/json`.

---

### List Clusters

Lists all clusters for the current tenant.

```bash
curl -X POST http://localhost:8081/organization.v1.ClusterService/ListClusters \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{}'
```

**With project filter:**

```bash
curl -X POST http://localhost:8081/organization.v1.ClusterService/ListClusters \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "projectId": "550e8400-e29b-41d4-a716-446655440000"
  }'
```

**Response:**

```json
{
  "clusters": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440001",
      "name": "production-cluster",
      "status": "CLUSTER_STATUS_RUNNING",
      "region": "eu-west-1",
      "projectCount": 3,
      "nodePoolCount": 2
    }
  ]
}
```

---

### Get Cluster

Retrieves detailed information about a specific cluster.

```bash
curl -X POST http://localhost:8081/organization.v1.ClusterService/GetCluster \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "clusterId": "550e8400-e29b-41d4-a716-446655440001"
  }'
```

**Response:**

```json
{
  "cluster": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "production-cluster",
    "region": "eu-west-1",
    "kubernetesVersion": "1.28",
    "status": "CLUSTER_STATUS_RUNNING",
    "created": "2025-01-15T10:30:00Z",
    "resourceUsage": {
      "cpu": { "used": 0, "total": 0, "unit": "cores" },
      "memory": { "used": 0, "total": 0, "unit": "GB" },
      "disk": { "used": 0, "total": 0, "unit": "GB" },
      "pods": { "used": 0, "total": 0, "unit": "pods" }
    },
    "nodePools": [],
    "members": [],
    "projects": []
  }
}
```

---

### Create Cluster

Creates a new cluster.

```bash
curl -X POST http://localhost:8081/organization.v1.ClusterService/CreateCluster \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "my-new-cluster",
    "region": "eu-west-1",
    "kubernetesVersion": "1.28"
  }'
```

**With node pools and plugins:**

```bash
curl -X POST http://localhost:8081/organization.v1.ClusterService/CreateCluster \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "my-new-cluster",
    "region": "eu-west-1",
    "kubernetesVersion": "1.28",
    "nodePools": [
      {
        "name": "default-pool",
        "machineType": "n1-standard-4",
        "nodeCount": 3,
        "minNodes": 1,
        "maxNodes": 10,
        "autoscalingEnabled": true
      }
    ],
    "pluginIds": ["plugin-1", "plugin-2"],
    "pluginPreset": "havenplus"
  }'
```

**Response:**

```json
{
  "clusterId": "550e8400-e29b-41d4-a716-446655440002",
  "status": "CLUSTER_STATUS_PROVISIONING"
}
```

---

### Update Cluster

Updates an existing cluster's configuration.

```bash
curl -X POST http://localhost:8081/organization.v1.ClusterService/UpdateCluster \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "clusterId": "550e8400-e29b-41d4-a716-446655440001",
    "kubernetesVersion": "1.29"
  }'
```

**With node pools:**

```bash
curl -X POST http://localhost:8081/organization.v1.ClusterService/UpdateCluster \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "clusterId": "550e8400-e29b-41d4-a716-446655440001",
    "kubernetesVersion": "1.29",
    "nodePools": [
      {
        "name": "default-pool",
        "machineType": "n1-standard-8",
        "nodeCount": 5,
        "minNodes": 2,
        "maxNodes": 20,
        "autoscalingEnabled": true
      }
    ]
  }'
```

**Response:**

```json
{
  "cluster": {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "production-cluster",
    "region": "eu-west-1",
    "kubernetesVersion": "1.29",
    "status": "CLUSTER_STATUS_UPGRADING",
    "created": "2025-01-15T10:30:00Z",
    "resourceUsage": { ... },
    "nodePools": [],
    "members": [],
    "projects": []
  }
}
```

---

### Delete Cluster

Deletes a cluster (soft delete).

```bash
curl -X POST http://localhost:8081/organization.v1.ClusterService/DeleteCluster \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "clusterId": "550e8400-e29b-41d4-a716-446655440001"
  }'
```

**Response:**

```json
{
  "success": true
}
```

---

### Get Cluster Activity

Retrieves activity logs for a cluster.

```bash
curl -X POST http://localhost:8081/organization.v1.ClusterService/GetClusterActivity \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "clusterId": "550e8400-e29b-41d4-a716-446655440001"
  }'
```

**Response:**

```json
{
  "activities": []
}
```

> Note: Activity logging is not yet implemented and returns an empty array.

---

### Get Kubeconfig

Downloads the kubeconfig for cluster access.

```bash
curl -X POST http://localhost:8081/organization.v1.ClusterService/GetKubeconfig \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "clusterId": "550e8400-e29b-41d4-a716-446655440001"
  }'
```

**Response:**

```json
{
  "kubeconfigContent": "apiVersion: v1\nkind: Config\nclusters:\n- cluster:\n    server: https://cluster-api.example.com\n  name: my-cluster\n..."
}
```

> Note: Kubeconfig generation is not yet implemented and returns a placeholder.

---

## Cluster Status Values

| Status | Description |
|--------|-------------|
| `CLUSTER_STATUS_UNSPECIFIED` | Status not set |
| `CLUSTER_STATUS_PROVISIONING` | Cluster is being created |
| `CLUSTER_STATUS_STARTING` | Cluster is starting up |
| `CLUSTER_STATUS_RUNNING` | Cluster is healthy and running |
| `CLUSTER_STATUS_UPGRADING` | Cluster is being upgraded |
| `CLUSTER_STATUS_ERROR` | Cluster encountered an error |
| `CLUSTER_STATUS_STOPPING` | Cluster is shutting down |
| `CLUSTER_STATUS_STOPPED` | Cluster is stopped |

## Error Responses

Errors are returned in Connect RPC format:

```json
{
  "code": "invalid_argument",
  "message": "validation error: name is required"
}
```

Common error codes:
- `invalid_argument` - Request validation failed
- `not_found` - Cluster does not exist
- `unauthenticated` - Missing or invalid JWT token
- `permission_denied` - Not authorized to access this cluster
