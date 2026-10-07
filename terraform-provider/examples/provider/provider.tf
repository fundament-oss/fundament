terraform {
  required_providers {
    fundament = {
      source = "fundament/fundament"
    }
  }
}

# The API key comes from the FUNDAMENT_API_KEY environment variable.
provider "fundament" {
  endpoint        = "https://organization.fundament.localhost:8443"
  organization_id = "019b4000-0000-7000-8000-000000000001"
}
