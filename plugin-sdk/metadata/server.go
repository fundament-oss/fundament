package metadata

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	pb "github.com/fundament-oss/fundament/plugin-sdk/metadata/proto/gen/v1"
	"github.com/fundament-oss/fundament/plugin-sdk/metadata/proto/gen/v1/pluginmetadatav1connect"
)

// Status holds the current plugin status as simple strings.
type Status struct {
	Phase   string
	Message string
}

// Definition holds the plugin definition metadata.
type Definition struct {
	Name             string
	DisplayName      string
	Version          string
	Description      string
	Author           string
	License          string
	Icon             string
	URLs             URLsDef
	Tags             []string
	Permissions      PermissionsDef
	Menu             Menu
	CustomComponents map[string]ComponentMappingDef
	UIHints          map[string]UIHintDef
	CRDs             []string
}

// URLsDef holds links related to the plugin.
type URLsDef struct {
	Homepage      string
	Repository    string
	Documentation string
}

// PermissionsDef declares what a plugin needs from the platform.
type PermissionsDef struct {
	Capabilities []string
	RBAC         []PolicyRuleDef
}

// PolicyRuleDef matches the Kubernetes RBAC PolicyRule structure.
type PolicyRuleDef struct {
	APIGroups []string
	Resources []string
	Verbs     []string
}

// Menu describes console UI entries.
type Menu struct {
	Organization []MenuEntryDef
	Project      []MenuEntryDef
}

// MenuEntryDef maps a CRD to console UI pages.
type MenuEntryDef struct {
	CRD    string
	List   bool
	Detail bool
	Create bool
	Icon   string
}

// ComponentMappingDef maps a CRD to custom UI component names.
type ComponentMappingDef struct {
	List   string
	Detail string
}

// UIHintDef provides form layout and status display hints for a CRD.
type UIHintDef struct {
	FormGroups    []FormGroupDef
	StatusMapping StatusMappingDef
}

// FormGroupDef groups related fields in a create/edit form.
type FormGroupDef struct {
	Name   string
	Fields []string
}

// StatusMappingDef maps a JSON path to status badge display values.
type StatusMappingDef struct {
	JSONPath string
	Values   map[string]StatusValueDef
}

// StatusValueDef describes how a status value is displayed.
type StatusValueDef struct {
	Badge string
	Label string
}

// StatusFunc returns the current plugin status.
type StatusFunc func() Status

// DefinitionFunc returns the plugin definition.
type DefinitionFunc func() Definition

// Server implements the PluginMetadataService Connect RPC service.
type Server struct {
	pluginmetadatav1connect.UnimplementedPluginMetadataServiceHandler
	getStatusFn     StatusFunc
	getDefinitionFn DefinitionFunc
}

// NewServer creates a metadata server that calls the provided functions
// to obtain current status and definition data.
func NewServer(statusFn StatusFunc, defFn DefinitionFunc) *Server {
	return &Server{
		getStatusFn:     statusFn,
		getDefinitionFn: defFn,
	}
}

func (s *Server) GetStatus(_ context.Context, _ *connect.Request[pb.GetStatusRequest]) (*connect.Response[pb.GetStatusResponse], error) {
	status := s.getStatusFn()
	def := s.getDefinitionFn()
	return connect.NewResponse(&pb.GetStatusResponse{
		Phase:   proto.String(status.Phase),
		Message: proto.String(status.Message),
		Version: proto.String(def.Version),
	}), nil
}

func (s *Server) GetDefinition(_ context.Context, _ *connect.Request[pb.GetDefinitionRequest]) (*connect.Response[pb.GetDefinitionResponse], error) {
	def := s.getDefinitionFn()

	orgMenu := make([]*pb.MenuEntry, len(def.Menu.Organization))
	for i, entry := range def.Menu.Organization {
		orgMenu[i] = &pb.MenuEntry{
			Crd:    proto.String(entry.CRD),
			List:   proto.Bool(entry.List),
			Detail: proto.Bool(entry.Detail),
			Create: proto.Bool(entry.Create),
			Icon:   proto.String(entry.Icon),
		}
	}

	projectMenu := make([]*pb.MenuEntry, len(def.Menu.Project))
	for i, entry := range def.Menu.Project {
		projectMenu[i] = &pb.MenuEntry{
			Crd:    proto.String(entry.CRD),
			List:   proto.Bool(entry.List),
			Detail: proto.Bool(entry.Detail),
			Create: proto.Bool(entry.Create),
			Icon:   proto.String(entry.Icon),
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
			List:   proto.String(v.List),
			Detail: proto.String(v.Detail),
		}
	}

	uiHints := make(map[string]*pb.UIHint, len(def.UIHints))
	for k, v := range def.UIHints {
		formGroups := make([]*pb.FormGroup, len(v.FormGroups))
		for i, fg := range v.FormGroups {
			formGroups[i] = &pb.FormGroup{
				Name:   proto.String(fg.Name),
				Fields: fg.Fields,
			}
		}

		statusValues := make(map[string]*pb.StatusValue, len(v.StatusMapping.Values))
		for sk, sv := range v.StatusMapping.Values {
			statusValues[sk] = &pb.StatusValue{
				Badge: proto.String(sv.Badge),
				Label: proto.String(sv.Label),
			}
		}

		uiHints[k] = &pb.UIHint{
			FormGroups: formGroups,
			StatusMapping: &pb.StatusMapping{
				JsonPath: proto.String(v.StatusMapping.JSONPath),
				Values:   statusValues,
			},
		}
	}

	return connect.NewResponse(&pb.GetDefinitionResponse{
		Name:        proto.String(def.Name),
		Version:     proto.String(def.Version),
		Description: proto.String(def.Description),
		DisplayName: proto.String(def.DisplayName),
		Author:      proto.String(def.Author),
		License:     proto.String(def.License),
		Icon:        proto.String(def.Icon),
		Urls: &pb.PluginURLs{
			Homepage:      proto.String(def.URLs.Homepage),
			Repository:    proto.String(def.URLs.Repository),
			Documentation: proto.String(def.URLs.Documentation),
		},
		Tags: def.Tags,
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
