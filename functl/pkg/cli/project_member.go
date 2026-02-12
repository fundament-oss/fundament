package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"connectrpc.com/connect"

	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

// ProjectMemberCmd contains project member subcommands.
type ProjectMemberCmd struct {
	List       ProjectMemberListCmd       `cmd:"" help:"List members of a project."`
	Add        ProjectMemberAddCmd        `cmd:"" help:"Add a member to a project."`
	UpdateRole ProjectMemberUpdateRoleCmd `cmd:"" name:"update-role" help:"Update a project member's role."`
	Remove     ProjectMemberRemoveCmd     `cmd:"" help:"Remove a member from a project."`
}

// ProjectMemberListCmd handles listing project members.
type ProjectMemberListCmd struct {
	ProjectID string `arg:"" help:"Project ID to list members for."`
}

// Run executes the project member list command.
func (c *ProjectMemberListCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	resp, err := apiClient.Projects().ListProjectMembers(context.Background(), connect.NewRequest(&organizationv1.ListProjectMembersRequest{
		ProjectId: c.ProjectID,
	}))
	if err != nil {
		return fmt.Errorf("failed to list project members: %w", err)
	}

	members := resp.Msg.Members

	if ctx.Output == OutputJSON {
		return PrintJSON(members)
	}

	if len(members) == 0 {
		fmt.Println("No members found")
		return nil
	}

	w := NewTableWriter()
	fmt.Fprintln(w, "USER ID\tUSER NAME\tROLE\tCREATED")
	for _, member := range members {
		created := ""
		if member.Created.IsValid() {
			created = member.Created.AsTime().Format(TimeFormat)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			member.UserId,
			member.UserName,
			formatMemberRole(member.Role),
			created,
		)
	}
	return w.Flush()
}

// ProjectMemberAddCmd handles adding a member to a project.
type ProjectMemberAddCmd struct {
	ProjectID string `arg:"" help:"Project ID to add a member to."`
	UserID    string `help:"User ID to add." required:""`
	Role      string `help:"Role for the member (admin or viewer)." required:"" enum:"admin,viewer"`
}

// Run executes the project member add command.
func (c *ProjectMemberAddCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	role, err := parseMemberRole(c.Role)
	if err != nil {
		return err
	}

	resp, err := apiClient.Projects().AddProjectMember(context.Background(), connect.NewRequest(&organizationv1.AddProjectMemberRequest{
		ProjectId: c.ProjectID,
		UserId:    c.UserID,
		Role:      role,
	}))
	if err != nil {
		return fmt.Errorf("failed to add project member: %w", err)
	}

	memberID := resp.Msg.MemberId

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]string{
			"member_id":  memberID,
			"project_id": c.ProjectID,
			"user_id":    c.UserID,
			"role":       c.Role,
		})
	}

	fmt.Printf("Added member %s to project %s with role %s\n", c.UserID, c.ProjectID, c.Role)
	return nil
}

// ProjectMemberUpdateRoleCmd handles updating a project member's role.
type ProjectMemberUpdateRoleCmd struct {
	ProjectID string `arg:"" help:"Project ID."`
	UserID    string `help:"User ID of the member to update." required:""`
	Role      string `help:"New role for the member (admin or viewer)." required:"" enum:"admin,viewer"`
}

// Run executes the project member update-role command.
func (c *ProjectMemberUpdateRoleCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	projectsClient := apiClient.Projects()

	member, err := findProjectMember(context.Background(), projectsClient, c.ProjectID, c.UserID)
	if err != nil {
		return err
	}

	role, err := parseMemberRole(c.Role)
	if err != nil {
		return err
	}

	_, err = projectsClient.UpdateProjectMemberRole(context.Background(), connect.NewRequest(&organizationv1.UpdateProjectMemberRoleRequest{
		MemberId: member.Id,
		Role:     role,
	}))
	if err != nil {
		return fmt.Errorf("failed to update project member role: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]string{
			"project_id": c.ProjectID,
			"user_id":    c.UserID,
			"role":       c.Role,
		})
	}

	fmt.Printf("Updated member %s role to %s in project %s\n", c.UserID, c.Role, c.ProjectID)
	return nil
}

// ProjectMemberRemoveCmd handles removing a member from a project.
type ProjectMemberRemoveCmd struct {
	ProjectID string `arg:"" help:"Project ID."`
	UserID    string `help:"User ID of the member to remove." required:""`
	Yes       bool   `help:"Skip confirmation prompt." short:"y"`
}

// Run executes the project member remove command.
func (c *ProjectMemberRemoveCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig()
	if err != nil {
		return err
	}

	projectsClient := apiClient.Projects()

	member, err := findProjectMember(context.Background(), projectsClient, c.ProjectID, c.UserID)
	if err != nil {
		return err
	}

	if !c.Yes {
		fmt.Printf("Remove member %q (%s) from project %s? [y/N] ", member.UserName, c.UserID, c.ProjectID)
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		if strings.TrimSpace(strings.ToLower(input)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	_, err = projectsClient.RemoveProjectMember(context.Background(), connect.NewRequest(&organizationv1.RemoveProjectMemberRequest{
		MemberId: member.Id,
	}))
	if err != nil {
		return fmt.Errorf("failed to remove project member: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]string{
			"project_id": c.ProjectID,
			"user_id":    c.UserID,
		})
	}

	fmt.Printf("Removed member %s from project %s\n", c.UserID, c.ProjectID)
	return nil
}

// findProjectMember resolves a project member from a project ID and user ID.
func findProjectMember(ctx context.Context, client organizationv1connect.ProjectServiceClient, projectID, userID string) (*organizationv1.ProjectMember, error) {
	resp, err := client.ListProjectMembers(ctx, connect.NewRequest(&organizationv1.ListProjectMembersRequest{
		ProjectId: projectID,
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to list project members: %w", err)
	}

	for _, member := range resp.Msg.Members {
		if member.UserId == userID {
			return member, nil
		}
	}

	return nil, fmt.Errorf("user %s is not a member of project %s", userID, projectID)
}

// parseMemberRole converts a CLI role string to the protobuf enum value.
func parseMemberRole(role string) (organizationv1.ProjectMemberRole, error) {
	switch strings.ToLower(role) {
	case "admin":
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN, nil
	case "viewer":
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER, nil
	default:
		return organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_UNSPECIFIED, fmt.Errorf("invalid role %q: must be admin or viewer", role)
	}
}

// formatMemberRole converts a protobuf role enum to a human-readable string.
func formatMemberRole(role organizationv1.ProjectMemberRole) string {
	switch role {
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_ADMIN:
		return "admin"
	case organizationv1.ProjectMemberRole_PROJECT_MEMBER_ROLE_VIEWER:
		return "viewer"
	default:
		return "unknown"
	}
}
