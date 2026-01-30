# OpenTofu Provider for Fundament

This OpenTofu provider allows you to interact with the Fundament organization API to manage and query your Kubernetes clusters.

## Requirements

- [OpenTofu](https://opentofu.org/docs/intro/install/) >= 1.11
- [Go](https://golang.org/doc/install) >= 1.25 (for building from source)
- A running Fundament instance

## Building the Provider

```bash
# From the terraform-provider directory
just terraform-provider::build
```

## Using the Provider

### Provider Configuration

```hcl
terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

provider "fundament" {
  endpoint = "http://organization.127.0.0.1.nip.io:8080"
  api_key  = var.fundament_api_key  # Or use FUNDAMENT_API_KEY environment variable
}
```

#### Arguments

| Name | Description | Required |
|------|-------------|----------|
| `endpoint` | The URL of the Fundament organization API | Yes |
| `api_key` | API key for authentication. Can also be set via `FUNDAMENT_API_KEY` environment variable. Mutually exclusive with `token`. | No* |
| `token` | JWT token for authentication. Can also be set via `FUNDAMENT_TOKEN` environment variable. Mutually exclusive with `api_key`. | No* |
| `authn_endpoint` | The URL of the Fundament authentication API (for API key exchange). Can also be set via `FUNDAMENT_AUTHN_ENDPOINT` environment variable. If not provided, it's automatically derived from the organization endpoint. | No |

\* Either `api_key` or `token` must be provided.

### Authentication

The provider supports two authentication methods:

#### API Key (Recommended)

API keys provide a more convenient authentication method that automatically handles token exchange and refresh:

```hcl
provider "fundament" {
  endpoint = "http://organization.127.0.0.1.nip.io:8080"
  api_key  = var.fundament_api_key
}
```

Or using environment variables:

```bash
export FUNDAMENT_API_KEY="your-api-key"
```

The provider automatically exchanges the API key for a JWT token and refreshes it as needed.

#### JWT Token

You can also authenticate directly with a JWT token:

```hcl
provider "fundament" {
  endpoint = "http://organization.127.0.0.1.nip.io:8080"
  token    = var.fundament_token
}
```

You can obtain a token by:

1. Logging into the Fundament console and extracting the token from the `fundament_auth` cookie (possibly remove the part starting with the last dot, since there should only be 3 segments in the JWT)
2. Using the authn-api's password grant flow

The token contains your organization ID and is used for all API requests.

## Data Sources

### fundament_clusters

Fetches the list of clusters for your organization.

#### Example Usage

```hcl
# List all clusters
data "fundament_clusters" "all" {}

output "cluster_names" {
  value = [for c in data.fundament_clusters.all.clusters : c.name]
}

# Filter by project
data "fundament_clusters" "project_clusters" {
  project_id = "your-project-uuid"
}
```

#### Argument Reference

| Name | Description | Required |
|------|-------------|----------|
| `project_id` | Filter clusters by project ID | No |

#### Attribute Reference

| Name | Description |
|------|-------------|
| `clusters` | List of clusters |
| `clusters.id` | The unique identifier of the cluster |
| `clusters.name` | The name of the cluster |
| `clusters.status` | The status of the cluster (`running`, `provisioning`, `stopped`, etc.) |
| `clusters.region` | The region where the cluster is deployed |

### fundament_cluster (data source)

Fetches a single cluster by ID.

#### Example Usage

```hcl
# Look up an existing cluster
data "fundament_cluster" "existing" {
  id = "your-cluster-uuid"
}

output "cluster_name" {
  value = data.fundament_cluster.existing.name
}
```

#### Argument Reference

| Name | Description | Required |
|------|-------------|----------|
| `id` | The unique identifier of the cluster to look up | Yes |

#### Attribute Reference

| Name | Description |
|------|-------------|
| `name` | The name of the cluster |
| `region` | The region where the cluster is deployed |
| `kubernetes_version` | The Kubernetes version of the cluster |
| `status` | The current status of the cluster (`provisioning`, `starting`, `running`, `upgrading`, `error`, `stopping`, `stopped`) |

## Resources

### fundament_cluster

Manages a Kubernetes cluster in Fundament.

#### Example Usage

```hcl
# Create a new cluster
resource "fundament_cluster" "example" {
  name               = "my-cluster"
  region             = "eu-west-1"
  kubernetes_version = "1.28"
}

# Reference the cluster ID
output "cluster_id" {
  value = fundament_cluster.example.id
}
```

#### Argument Reference

| Name | Description | Required | Forces Replacement |
|------|-------------|----------|-------------------|
| `name` | The name of the cluster. Must be unique within the organization. | Yes | Yes |
| `region` | The region where the cluster will be deployed. | Yes | Yes |
| `kubernetes_version` | The Kubernetes version for the cluster. Can be updated to upgrade the cluster. | Yes | No |

#### Attribute Reference

| Name | Description |
|------|-------------|
| `id` | The unique identifier of the cluster. |
| `status` | The current status of the cluster (`provisioning`, `starting`, `running`, `upgrading`, `error`, `stopping`, `stopped`). |

#### Import

Clusters can be imported using the cluster ID:

```bash
tofu import fundament_cluster.example <cluster-id>
```

## Development

### Running Tests

```bash
just terraform-test
```

### Cleaning Build Artifacts

```bash
just terraform-clean
```

### Testing Locally

1. Start the Fundament development environment:
   ```bash
   just dev
   ```

2. Build and install the provider locally:
   ```bash
   just terraform-provider::install
   ```

3. Navigate to the example directory and run tofu:
   ```bash
   cd terraform-provider/examples/data-sources/clusters
   FUNDAMENT_API_KEY=your-api-key tofu plan
   ```

### Running Acceptance Tests

Acceptance tests run against a real Fundament API. To run them:

```bash
export TF_ACC=1
export FUNDAMENT_ENDPOINT="http://organization.127.0.0.1.nip.io:8080"
export FUNDAMENT_API_KEY="your-api-key"  # Or use FUNDAMENT_TOKEN instead
# Optional: for project filter tests
export FUNDAMENT_TEST_PROJECT_ID="your-project-uuid"
# Optional: for cluster data source tests
export FUNDAMENT_TEST_CLUSTER_ID="your-cluster-uuid"

just terraform-provider::test
```

#### Debugging Acceptance Tests

To run a specific test with verbose output:

```bash
TF_ACC=1 go test -v -run TestAccClusterDataSource ./internal/provider/
```

To enable Terraform debug logging:

```bash
export TF_LOG=DEBUG
TF_ACC=1 go test -v ./internal/provider/
```

Available log levels: `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`.

## Future Development

The following resources and data sources are planned for future releases:

- `fundament_project` resource and data source
- `fundament_node_pool` resource
- `fundament_namespace` resource
