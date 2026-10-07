data "fundament_project" "example" {
  name = "my-project"
}

data "fundament_organization_members" "all" {}

resource "fundament_project_member" "example" {
  project_id = data.fundament_project.example.id
  user_id    = one([for m in data.fundament_organization_members.all.members : m.user_id if m.email == "colleague@example.com"])
  permission = "viewer"
}
