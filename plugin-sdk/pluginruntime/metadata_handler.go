package pluginruntime

import (
	"context"

	"connectrpc.com/connect"

	pb "github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/pluginruntime/metadata/proto/gen/v1/pluginmetadatav1connect"
)

func ptr[T any](v T) *T { return &v }

// metadataHandler implements the PluginMetadataService Connect RPC service
// using pluginruntime types directly (no intermediate conversion types).
type metadataHandler struct {
	pluginmetadatav1connect.UnimplementedPluginMetadataServiceHandler
	getStatus     func() PluginStatus
	getDefinition func() PluginDefinition
	uninstall     func(context.Context) error
}

// NewMetadataHandler creates a metadata handler that serves plugin status and
// definition via the PluginMetadataService Connect RPC service.
func NewMetadataHandler(statusFn func() PluginStatus, defFn func() PluginDefinition, uninstallFn func(context.Context) error) *metadataHandler {
	return &metadataHandler{
		getStatus:     statusFn,
		getDefinition: defFn,
		uninstall:     uninstallFn,
	}
}

func (h *metadataHandler) GetStatus(_ context.Context, _ *connect.Request[pb.GetStatusRequest]) (*connect.Response[pb.GetStatusResponse], error) {
	status := h.getStatus()
	def := h.getDefinition()
	return connect.NewResponse(&pb.GetStatusResponse{
		Phase:   ptr(string(status.Phase)),
		Message: ptr(status.Message),
		Version: ptr(def.Metadata.Version),
	}), nil
}

func (h *metadataHandler) GetDefinition(_ context.Context, _ *connect.Request[pb.GetDefinitionRequest]) (*connect.Response[pb.GetDefinitionResponse], error) {
	def := h.getDefinition()

	orgMenu := make([]*pb.MenuEntry, len(def.Menu.Organization))
	for i, entry := range def.Menu.Organization {
		orgMenu[i] = &pb.MenuEntry{
			Crd:    ptr(entry.CRD),
			List:   ptr(entry.List),
			Detail: ptr(entry.Detail),
			Create: ptr(entry.Create),
			Icon:   ptr(entry.Icon),
		}
	}

	projectMenu := make([]*pb.MenuEntry, len(def.Menu.Project))
	for i, entry := range def.Menu.Project {
		projectMenu[i] = &pb.MenuEntry{
			Crd:    ptr(entry.CRD),
			List:   ptr(entry.List),
			Detail: ptr(entry.Detail),
			Create: ptr(entry.Create),
			Icon:   ptr(entry.Icon),
		}
	}

	rbacRules := make([]*pb.PolicyRule, len(def.Permissions.RBAC))
	for i, rule := range def.Permissions.RBAC {
		rbacRules[i] = &pb.PolicyRule{
			ApiGroups: rule.APIGroups,
			Resources: rule.Resources,
			Verbs:     rule.Verbs,
		}
	}

	customComponents := make(map[string]*pb.ComponentMapping, len(def.CustomComponents))
	for k, v := range def.CustomComponents {
		customComponents[k] = &pb.ComponentMapping{
			List:   ptr(v.List),
			Detail: ptr(v.Detail),
		}
	}

	uiHints := make(map[string]*pb.UIHint, len(def.UIHints))
	for k, v := range def.UIHints {
		formGroups := make([]*pb.FormGroup, len(v.FormGroups))
		for i, fg := range v.FormGroups {
			formGroups[i] = &pb.FormGroup{
				Name:   ptr(fg.Name),
				Fields: fg.Fields,
			}
		}

		statusValues := make(map[string]*pb.StatusValue, len(v.StatusMapping.Values))
		for sk, sv := range v.StatusMapping.Values {
			statusValues[sk] = &pb.StatusValue{
				Badge: ptr(sv.Badge),
				Label: ptr(sv.Label),
			}
		}

		uiHints[k] = &pb.UIHint{
			FormGroups: formGroups,
			StatusMapping: &pb.StatusMapping{
				JsonPath: ptr(v.StatusMapping.JSONPath),
				Values:   statusValues,
			},
		}
	}

	return connect.NewResponse(&pb.GetDefinitionResponse{
		Name:        ptr(def.Metadata.Name),
		Version:     ptr(def.Metadata.Version),
		Description: ptr(def.Metadata.Description),
		DisplayName: ptr(def.Metadata.DisplayName),
		Author:      ptr(def.Metadata.Author),
		License:     ptr(def.Metadata.License),
		Icon:        ptr(def.Metadata.Icon),
		Urls: &pb.PluginURLs{
			Homepage:      ptr(def.Metadata.URLs.Homepage),
			Repository:    ptr(def.Metadata.URLs.Repository),
			Documentation: ptr(def.Metadata.URLs.Documentation),
		},
		Tags: def.Metadata.Tags,
		Permissions: &pb.Permissions{
			Capabilities: def.Permissions.Capabilities,
			Rbac:         rbacRules,
		},
		Menu: &pb.MenuDefinition{
			Organization: orgMenu,
			Project:      projectMenu,
		},
		CustomComponents: customComponents,
		UiHints:          uiHints,
		Crds:             def.CRDs,
	}), nil
}

func (h *metadataHandler) RequestUninstall(ctx context.Context, _ *connect.Request[pb.RequestUninstallRequest]) (*connect.Response[pb.RequestUninstallResponse], error) {
	if err := h.uninstall(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.RequestUninstallResponse{}), nil
}
