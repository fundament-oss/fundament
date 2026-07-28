package organization

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	db "github.com/fundament-oss/fundament/organization-api/pkg/db/gen"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

func (s *Server) GetPluginDefinition(
	ctx context.Context,
	req *connect.Request[organizationv1.GetPluginDefinitionRequest],
) (*connect.Response[organizationv1.GetPluginDefinitionResponse], error) {
	name := req.Msg.GetPluginName()
	version := req.Msg.GetPluginVersion()
	if name == "" || version == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("plugin_name and plugin_version are required"))
	}
	row, err := s.queries.PluginDefinitionGetActive(ctx, db.PluginDefinitionGetActiveParams{
		Name: name, PluginVersion: version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("plugin definition %s@%s not found", name, version))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get plugin definition: %w", err))
	}
	parsed, err := pluginruntime.ParseDefinition(row.Manifest)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("parse stored manifest: %w", err))
	}
	return connect.NewResponse(organizationv1.GetPluginDefinitionResponse_builder{
		Manifest:   row.Manifest,
		Hash:       row.Hash,
		Definition: pluginDefinitionToProto(&parsed),
	}.Build()), nil
}

func pluginDefinitionToProto(def *pluginruntime.PluginDefinition) *organizationv1.PluginDefinition {
	rbac := make([]*organizationv1.PluginPolicyRule, 0, len(def.Spec.Permissions.RBAC))
	for _, r := range def.Spec.Permissions.RBAC {
		rbac = append(rbac, organizationv1.PluginPolicyRule_builder{
			ApiGroups: r.APIGroups, Resources: r.Resources, Verbs: r.Verbs,
		}.Build())
	}
	mapMenu := func(entries []pluginruntime.MenuEntry) []*organizationv1.PluginMenuEntry {
		out := make([]*organizationv1.PluginMenuEntry, 0, len(entries))
		for _, e := range entries {
			out = append(out, organizationv1.PluginMenuEntry_builder{
				Crd: e.CRD, List: e.List, Detail: e.Detail, Icon: e.Icon,
			}.Build())
		}
		return out
	}
	components := make(map[string]*organizationv1.PluginComponentMapping, len(def.Spec.CustomComponents))
	for k, c := range def.Spec.CustomComponents {
		components[k] = organizationv1.PluginComponentMapping_builder{List: c.List, Detail: c.Detail, Create: c.Create}.Build()
	}
	allowed := make([]*organizationv1.PluginAllowedResource, 0, len(def.Spec.AllowedResources))
	for _, a := range def.Spec.AllowedResources {
		allowed = append(allowed, organizationv1.PluginAllowedResource_builder{
			Group: a.Group, Version: a.Version, Resource: a.Resource, Verbs: a.Verbs,
		}.Build())
	}
	uiHints := make(map[string]*organizationv1.PluginUIHint, len(def.Spec.UIHints))
	for k, h := range def.Spec.UIHints {
		formGroups := make([]*organizationv1.PluginFormGroup, 0, len(h.FormGroups))
		for _, fg := range h.FormGroups {
			formGroups = append(formGroups, organizationv1.PluginFormGroup_builder{
				Name: fg.Name, Fields: fg.Fields,
			}.Build())
		}
		statusValues := make(map[string]*organizationv1.PluginStatusValue, len(h.StatusMapping.Values))
		for svKey, svVal := range h.StatusMapping.Values {
			statusValues[svKey] = organizationv1.PluginStatusValue_builder{
				Badge: svVal.Badge, Label: svVal.Label,
			}.Build()
		}
		uiHints[k] = organizationv1.PluginUIHint_builder{
			FormGroups: formGroups,
			StatusMapping: organizationv1.PluginStatusMapping_builder{
				JsonPath: h.StatusMapping.JSONPath,
				Values:   statusValues,
			}.Build(),
		}.Build()
	}
	return organizationv1.PluginDefinition_builder{
		Metadata: organizationv1.PluginDefinitionMetadata_builder{
			Name: def.Metadata.Name, DisplayName: def.Metadata.DisplayName, Version: def.Metadata.Version,
			Description: def.Metadata.Description, Author: def.Metadata.Author, License: def.Metadata.License,
			Icon: def.Metadata.Icon, Tags: def.Metadata.Tags,
			Urls: organizationv1.PluginURLs_builder{
				Homepage: def.Metadata.URLs.Homepage, Repository: def.Metadata.URLs.Repository, Documentation: def.Metadata.URLs.Documentation,
			}.Build(),
		}.Build(),
		Image:            def.Spec.Image,
		ImagePullPolicy:  def.Spec.ImagePullPolicy,
		Permissions:      organizationv1.PluginPermissions_builder{Capabilities: def.Spec.Permissions.Capabilities, Rbac: rbac}.Build(),
		Menu:             organizationv1.PluginMenu_builder{Organization: mapMenu(def.Spec.Menu.Organization), Project: mapMenu(def.Spec.Menu.Project)}.Build(),
		Crds:             def.Spec.CRDs,
		CustomComponents: components,
		AllowedResources: allowed,
		UiHints:          uiHints,
	}.Build()
}
