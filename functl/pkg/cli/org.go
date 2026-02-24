package cli

// OrgCmd contains organization subcommands.
type OrgCmd struct {
	Member OrgMemberCmd `cmd:"" help:"Manage organization members."`
}
