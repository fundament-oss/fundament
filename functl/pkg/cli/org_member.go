package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"connectrpc.com/connect"

	"github.com/fundament-oss/fundament/functl/pkg/client"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
)

// OrgMemberCmd contains organization member subcommands.
type OrgMemberCmd struct {
	List             OrgMemberListCmd             `cmd:"" help:"List organization members."`
	Invite           OrgMemberInviteCmd           `cmd:"" help:"Invite a member to the organization."`
	UpdatePermission OrgMemberUpdatePermissionCmd `cmd:"" name:"update-permission" help:"Update a member's permission."`
	Remove           OrgMemberRemoveCmd           `cmd:"" help:"Remove a member from the organization."`
}

// OrgMemberListCmd handles listing organization members.
type OrgMemberListCmd struct {
	OrgID string `help:"Organization ID." required:"" name:"org"`
}

// Run executes the org member list command.
func (c *OrgMemberListCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig(WithOrg(c.OrgID))
	if err != nil {
		return err
	}

	resp, err := apiClient.Members().ListMembers(context.Background(), connect.NewRequest(&organizationv1.ListMembersRequest{}))
	if err != nil {
		return fmt.Errorf("failed to list members: %w", err)
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
	fmt.Fprintln(w, "USER ID\tNAME\tEMAIL\tPERMISSION\tSTATUS\tCREATED")
	for _, member := range members {
		created := ""
		if member.Created.IsValid() {
			created = member.Created.AsTime().Format(TimeFormat)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			member.UserId,
			member.Name,
			member.GetEmail(),
			member.Permission,
			member.Status,
			created,
		)
	}
	return w.Flush()
}

// OrgMemberInviteCmd handles inviting a member to the organization.
type OrgMemberInviteCmd struct {
	OrgID      string `help:"Organization ID." required:"" name:"org"`
	Email      string `help:"Email address of the user to invite." required:""`
	Permission string `help:"Permission for the member (admin or viewer)." required:"" enum:"admin,viewer"`
}

// Run executes the org member invite command.
func (c *OrgMemberInviteCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig(WithOrg(c.OrgID))
	if err != nil {
		return err
	}

	resp, err := apiClient.Invites().InviteMember(context.Background(), connect.NewRequest(&organizationv1.InviteMemberRequest{
		Email:      c.Email,
		Permission: c.Permission,
	}))
	if err != nil {
		return fmt.Errorf("failed to invite member: %w", err)
	}

	member := resp.Msg.Member

	if ctx.Output == OutputJSON {
		return PrintJSON(member)
	}

	fmt.Printf("Invited %s with permission %s\n", c.Email, c.Permission)
	return nil
}

// OrgMemberUpdatePermissionCmd handles updating a member's permission.
type OrgMemberUpdatePermissionCmd struct {
	OrgID      string `help:"Organization ID." required:"" name:"org"`
	UserID     string `help:"User ID of the member to update." required:"" name:"user-id"`
	Permission string `help:"New permission for the member (admin or viewer)." required:"" enum:"admin,viewer"`
}

// Run executes the org member update-permission command.
func (c *OrgMemberUpdatePermissionCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig(WithOrg(c.OrgID))
	if err != nil {
		return err
	}

	member, err := findOrgMember(apiClient, c.UserID)
	if err != nil {
		return err
	}

	_, err = apiClient.Members().UpdateMemberPermission(context.Background(), connect.NewRequest(&organizationv1.UpdateMemberPermissionRequest{
		Id:         member.Id,
		Permission: c.Permission,
	}))
	if err != nil {
		return fmt.Errorf("failed to update member permission: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]string{
			"user_id":    c.UserID,
			"permission": c.Permission,
		})
	}

	fmt.Printf("Updated member %s permission to %s\n", c.UserID, c.Permission)
	return nil
}

// OrgMemberRemoveCmd handles removing a member from the organization.
type OrgMemberRemoveCmd struct {
	OrgID  string `help:"Organization ID." required:"" name:"org"`
	UserID string `help:"User ID of the member to remove." required:"" name:"user-id"`
	Yes    bool   `help:"Skip confirmation prompt." short:"y"`
}

// Run executes the org member remove command.
func (c *OrgMemberRemoveCmd) Run(ctx *Context) error {
	apiClient, err := NewClientFromConfig(WithOrg(c.OrgID))
	if err != nil {
		return err
	}

	member, err := findOrgMember(apiClient, c.UserID)
	if err != nil {
		return err
	}

	if !c.Yes {
		fmt.Printf("Remove member %q (%s) from the organization? [y/N] ", member.Name, c.UserID)
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

	_, err = apiClient.Members().DeleteMember(context.Background(), connect.NewRequest(&organizationv1.DeleteMemberRequest{
		Id: member.Id,
	}))
	if err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}

	if ctx.Output == OutputJSON {
		return PrintJSON(map[string]string{
			"user_id": c.UserID,
		})
	}

	fmt.Printf("Removed member %s from the organization\n", c.UserID)
	return nil
}

// findOrgMember resolves an org member from a user ID.
func findOrgMember(apiClient *client.Client, userID string) (*organizationv1.Member, error) {
	resp, err := apiClient.Members().GetMember(context.Background(), connect.NewRequest(&organizationv1.GetMemberRequest{
		Lookup: &organizationv1.GetMemberRequest_UserId{UserId: userID},
	}))
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, fmt.Errorf("user %s is not a member of this organization", userID)
		}
		return nil, fmt.Errorf("failed to get member: %w", err)
	}

	return resp.Msg.Member, nil
}
