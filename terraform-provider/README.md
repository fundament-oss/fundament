# OpenTofu Provider for Fundament

This OpenTofu provider allows you to interact with the Fundament organization API to manage and query your Kubernetes clusters.

## Requirements

- [OpenTofu](https://opentofu.org/docs/intro/install/) >= 1.11
- [Go](https://golang.org/doc/install) >= 1.27 (for building from source)
- A running Fundament instance

## Installation

### Download a prebuilt package

The provider is not (yet) published to a provider registry. Every push to master publishes packages for Linux (amd64, arm64), macOS (Apple Silicon) and Windows (amd64) to the rolling [`terraform-provider-latest`](https://github.com/fundament-oss/fundament/releases/tag/terraform-provider-latest) release, named in the registry layout that OpenTofu and Terraform understand for local plugin mirrors. Drop the zip, unchanged, into the implied plugin directory and `tofu init` picks it up without any CLI configuration:

```bash
# Pick your platform: linux_amd64, linux_arm64 or darwin_arm64
PLATFORM=linux_amd64
MIRROR=~/.terraform.d/plugins/registry.opentofu.org/fundament/fundament
mkdir -p "$MIRROR"
curl -fsSL -o "$MIRROR/terraform-provider-fundament_0.1.0_${PLATFORM}.zip" \
  "https://github.com/fundament-oss/fundament/releases/download/terraform-provider-latest/terraform-provider-fundament_0.1.0_${PLATFORM}.zip"
```

On Windows (PowerShell):

```powershell
$mirror = "$env:APPDATA\terraform.d\plugins\registry.opentofu.org\fundament\fundament"
New-Item -ItemType Directory -Force $mirror | Out-Null
Invoke-WebRequest -Uri https://github.com/fundament-oss/fundament/releases/download/terraform-provider-latest/terraform-provider-fundament_0.1.0_windows_amd64.zip -OutFile "$mirror\terraform-provider-fundament_0.1.0_windows_amd64.zip"
```

Do not rename or unpack the zip: the filename is how OpenTofu discovers the provider version and platform. For Terraform instead of OpenTofu, use the same layout under `registry.terraform.io/fundament/fundament/`.

Checksums are published alongside the packages as `terraform-provider-fundament_0.1.0_SHA256SUMS`. `tofu init` reports the provider as `(unauthenticated)` because the packages are not GPG-signed.

The rolling release always carries provider version `0.1.0`, so `tofu init` alone will keep using the copy it already unpacked. To pick up a newer build after re-downloading the zip:

```bash
rm .terraform.lock.hcl
tofu init -upgrade
```

### Build from source

```bash
just terraform-provider::install
```

This builds the provider and installs it as version `0.1.0` for your platform in the local plugin directory, replacing any downloaded package for that platform.

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
  endpoint        = "https://organization.fundament.localhost:8443"
  organization_id = "019b4000-0000-7000-8000-000000000001"
  api_key         = var.fundament_api_key  # Or use FUNDAMENT_API_KEY environment variable
}
```

#### Arguments

| Name | Description | Required |
|------|-------------|----------|
| `endpoint` | The URL of the Fundament organization API. Can also be set via the `FUNDAMENT_ENDPOINT` environment variable. | Yes |
| `organization_id` | The ID of the organization to manage, shown in the console under **General**. Can also be set via the `FUNDAMENT_ORGANIZATION_ID` environment variable. | Yes |
| `api_key` | API key for authentication. Can also be set via the `FUNDAMENT_API_KEY` environment variable. | Yes |
| `authn_endpoint` | The URL of the Fundament authentication API (for API key exchange). Can also be set via the `FUNDAMENT_AUTHN_ENDPOINT` environment variable. If not provided, it's derived from `endpoint` by replacing `organization` with `authn`. | No |
| `kube_api_proxy_url` | The URL of the Kubernetes API proxy, needed only for `fundament_plugin_installation`. Can also be set via the `FUNDAMENT_KUBE_API_PROXY_URL` environment variable. | No |

### Authentication

#### API Key

API keys provide a convenient authentication method that automatically handles token exchange and refresh:

```hcl
provider "fundament" {
  endpoint        = "https://organization.fundament.localhost:8443"
  organization_id = "019b4000-0000-7000-8000-000000000001"
  api_key         = var.fundament_api_key
}
```

Or using environment variables:

```bash
export FUNDAMENT_API_KEY="your-api-key"
```

The provider automatically exchanges the API key for a JWT token and refreshes it as needed.


## Reference

The [reference](docs/index.md) describes every resource and data source with its arguments, attributes and an example. It is generated from the provider's schema and from [`examples/`](examples/) by `just terraform-provider::docs`, which `just generate` runs; CI fails when it is out of date. Describe a new argument in its schema `Description`, not here.

## Development

### Running Tests

```bash
just terraform-provider::test
```

### Cleaning Build Artifacts

```bash
just terraform-provider::clean
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

3. Put the provider configuration and an example in a directory and run tofu:
   ```bash
   mkdir /tmp/try-fundament
   cp terraform-provider/examples/provider/provider.tf terraform-provider/examples/data-sources/fundament_clusters/data-source.tf /tmp/try-fundament
   cd /tmp/try-fundament
   FUNDAMENT_API_KEY=your-api-key tofu init
   FUNDAMENT_API_KEY=your-api-key tofu plan
   ```

### Checking the Examples

```bash
just terraform-provider::validate-examples
```

This runs `tofu validate` on every example and on every configuration on the [docs page](../docs/user/opentofu-provider.md) that declares its providers; CI runs it on every pull request.

### Running Acceptance Tests

Acceptance tests run against a real Fundament API: the local development environment in mock mode (`just dev`). They log in as the test user alice, create an API key and use the first organization, so they need no further settings:

```bash
just terraform-provider::test-acc
```

They create clusters in the test region `eu-west-1`, which a local Gardener rejects; run them against mock mode.

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
