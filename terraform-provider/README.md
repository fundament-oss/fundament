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
  token    = var.fundament_token  # Or use FUNDAMENT_TOKEN environment variable
}
```

#### Arguments

| Name | Description | Required |
|------|-------------|----------|
| `endpoint` | The URL of the Fundament organization API | Yes |
| `token` | JWT token for authentication. Can also be set via `FUNDAMENT_TOKEN` environment variable | Yes |

### Authentication

The provider uses JWT tokens for authentication. You can obtain a token by:

1. Logging into the Fundament console and extracting the token from the `fundament_auth` cookie
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
   FUNDAMENT_TOKEN=your-jwt-token tofu plan
   ```

### Running Acceptance Tests

Acceptance tests run against a real Fundament API. To run them:

```bash
export TF_ACC=1
export FUNDAMENT_ENDPOINT="http://organization.127.0.0.1.nip.io:8080"
export FUNDAMENT_TOKEN="your-jwt-token"
# Optional: for project filter tests
export FUNDAMENT_TEST_PROJECT_ID="your-project-uuid"

just terraform-provider::test
```

## Future Development

The following resources and data sources are planned for future releases:

- `fundament_cluster` resource (CRUD operations)
- `fundament_project` resource and data source
- `fundament_node_pool` resource
- `fundament_namespace` resource
