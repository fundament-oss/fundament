data "fundament_project" "example" {
  name = "my-project"
}

data "fundament_project_members" "all" {
  project_id = data.fundament_project.example.id
}
