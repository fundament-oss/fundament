data "fundament_organization_members" "all" {}

output "member_emails" {
  value = [for m in data.fundament_organization_members.all.members : m.email]
}
