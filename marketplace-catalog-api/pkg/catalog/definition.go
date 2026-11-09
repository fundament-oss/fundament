package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	marketplacev1 "github.com/fundament-oss/fundament/marketplace-api/pkg/proto/gen/marketplace/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime"
)

// definitionFromManifest parses a stored manifest for the console. The decode
// tolerates unknown fields, as configSchemaFromManifest does: a manifest from a
// newer plugin-sdk must not lose its menu. Icon digests resolve to the stored
// logo and menu icons.
func definitionFromManifest(manifest []byte, logo string, menuIcons []byte) (*marketplacev1.PluginDefinition, error) {
	var def pluginruntime.PluginDefinition
	if err := yaml.Unmarshal(manifest, &def); err != nil {
		return nil, fmt.Errorf("parsing plugin definition: %w", err)
	}
	if def.Metadata.Name == "" {
		return nil, errors.New("plugin definition is missing metadata.name")
	}
	return pluginDefinitionToProto(&def, logo, decodeMenuIcons(menuIcons)), nil
}

// decodeMenuIcons parses the stored digest-to-data-URI map; an unreadable value
// yields an empty map so the definition is still served.
func decodeMenuIcons(raw []byte) map[string]string {
	var icons map[string]string
	if err := json.Unmarshal(raw, &icons); err != nil {
		return map[string]string{}
	}
	return icons
}

// resolveIcon maps a digest to its data URI and passes built-in names through.
// Anything else (unresolved or malformed digest, URL, path, data: URI) becomes "".
func resolveIcon(value string, uris map[string]string) string {
	switch {
	case pluginruntime.IsIconDigest(value):
		return uris[value]
	case strings.HasPrefix(value, "sha256:"), strings.HasPrefix(value, "data:"), pluginruntime.IsIconFileRef(value):
		return ""
	default:
		return value
	}
}

func pluginDefinitionToProto(def *pluginruntime.PluginDefinition, logo string, menuIcons map[string]string) *marketplacev1.PluginDefinition {
	rbac := make([]*marketplacev1.PluginPolicyRule, 0, len(def.Spec.Permissions.RBAC))
	for _, r := range def.Spec.Permissions.RBAC {
		rbac = append(rbac, marketplacev1.PluginPolicyRule_builder{
			ApiGroups: r.APIGroups, Resources: r.Resources, Verbs: r.Verbs,
		}.Build())
	}
	icon := resolveIcon(def.Metadata.Icon, map[string]string{def.Metadata.Icon: logo})
	mapMenu := func(entries []pluginruntime.MenuEntry) []*marketplacev1.PluginMenuEntry {
		out := make([]*marketplacev1.PluginMenuEntry, 0, len(entries))
		for _, e := range entries {
			e.Icon = resolveIcon(e.Icon, menuIcons)
			out = append(out, marketplacev1.PluginMenuEntry_builder{
				Crd: e.CRD, List: e.List, Detail: e.Detail, Icon: e.Icon, Label: e.Label,
				SingularLabel: e.SingularLabel,
			}.Build())
		}
		return out
	}
	components := make(map[string]*marketplacev1.PluginComponentMapping, len(def.Spec.CustomComponents))
	for k, c := range def.Spec.CustomComponents {
		components[k] = marketplacev1.PluginComponentMapping_builder{
			List: c.List, Detail: c.Detail, Create: c.Create, Edit: c.Edit,
		}.Build()
	}
	allowed := make([]*marketplacev1.PluginAllowedResource, 0, len(def.Spec.AllowedResources))
	for _, a := range def.Spec.AllowedResources {
		allowed = append(allowed, marketplacev1.PluginAllowedResource_builder{
			Group: a.Group, Version: a.Version, Resource: a.Resource, Verbs: a.Verbs,
		}.Build())
	}
	uiHints := make(map[string]*marketplacev1.PluginUIHint, len(def.Spec.UIHints))
	for k, h := range def.Spec.UIHints {
		formGroups := make([]*marketplacev1.PluginFormGroup, 0, len(h.FormGroups))
		for _, fg := range h.FormGroups {
			formGroups = append(formGroups, marketplacev1.PluginFormGroup_builder{
				Name: fg.Name, Fields: fg.Fields,
			}.Build())
		}
		statusValues := make(map[string]*marketplacev1.PluginStatusValue, len(h.StatusMapping.Values))
		for svKey, svVal := range h.StatusMapping.Values {
			statusValues[svKey] = marketplacev1.PluginStatusValue_builder{
				Badge: svVal.Badge, Label: svVal.Label,
			}.Build()
		}
		uiHints[k] = marketplacev1.PluginUIHint_builder{
			FormGroups: formGroups,
			StatusMapping: marketplacev1.PluginStatusMapping_builder{
				JsonPath: h.StatusMapping.JSONPath,
				Values:   statusValues,
			}.Build(),
		}.Build()
	}
	return marketplacev1.PluginDefinition_builder{
		Metadata: marketplacev1.PluginDefinitionMetadata_builder{
			Name: def.Metadata.Name, DisplayName: def.Metadata.DisplayName, Version: def.Metadata.Version,
			Description: def.Metadata.Description, Author: def.Metadata.Author, License: def.Metadata.License,
			Icon: icon, Tags: def.Metadata.Tags,
			Urls: marketplacev1.PluginURLs_builder{
				Homepage: def.Metadata.URLs.Homepage, Repository: def.Metadata.URLs.Repository, Documentation: def.Metadata.URLs.Documentation,
			}.Build(),
		}.Build(),
		Image:            def.Spec.Image,
		ImagePullPolicy:  def.Spec.ImagePullPolicy,
		Permissions:      marketplacev1.PluginPermissions_builder{Capabilities: def.Spec.Permissions.Capabilities, Rbac: rbac}.Build(),
		Menu:             marketplacev1.PluginMenu_builder{Organization: mapMenu(def.Spec.Menu.Organization), Project: mapMenu(def.Spec.Menu.Project)}.Build(),
		Crds:             def.Spec.CRDs,
		CustomComponents: components,
		AllowedResources: allowed,
		UiHints:          uiHints,
	}.Build()
}
