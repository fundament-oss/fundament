package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &PluginInstallationResource{}
var _ resource.ResourceWithConfigure = &PluginInstallationResource{}
var _ resource.ResourceWithImportState = &PluginInstallationResource{}

type PluginInstallationResource struct {
	client *FundamentClient
}

type PluginInstallationResourceModel struct {
	ID         types.String `tfsdk:"id"`
	ClusterID  types.String `tfsdk:"cluster_id"`
	PluginName types.String `tfsdk:"plugin_name"`
	Image      types.String `tfsdk:"image"`
	Phase      types.String `tfsdk:"phase"`
}

type pluginInstallationMetadata struct {
	Name string `json:"name"`
}

type pluginInstallationSpec struct {
	PluginName string `json:"pluginName"`
	Image      string `json:"image"`
}

type pluginInstallationStatus struct {
	Phase string `json:"phase"`
}

type pluginInstallationCRD struct {
	APIVersion string                     `json:"apiVersion"`
	Kind       string                     `json:"kind"`
	Metadata   pluginInstallationMetadata `json:"metadata"`
	Spec       pluginInstallationSpec     `json:"spec"`
	Status     pluginInstallationStatus   `json:"status,omitempty"`
}

func NewPluginInstallationResource() resource.Resource {
	return &PluginInstallationResource{}
}

func (r *PluginInstallationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_installation"
}

func (r *PluginInstallationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a plugin installation on a Fundament cluster via the kube-api-proxy.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Composite identifier in the form {cluster_id}/{plugin_name}.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Description: "The ID of the cluster to install the plugin on. Changing this value forces a replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"plugin_name": schema.StringAttribute{
				Description: "The name of the plugin to install. Must match a plugin in the Fundament catalog. Changing this value forces a replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"image": schema.StringAttribute{
				Description: "The container image reference for the plugin (e.g. ghcr.io/fundament/grafana:v10.2.0). Must be set explicitly. Changing this value forces a replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"phase": schema.StringAttribute{
				Description: "The current phase of the plugin installation as reported by the plugin controller.",
				Computed:    true,
			},
		},
	}
}

func (r *PluginInstallationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*FundamentClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *FundamentClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *PluginInstallationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PluginInstallationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError("Client Not Configured", "The Fundament client was not configured. Please report this issue to the provider developers.")
		return
	}

	if r.client.KubeProxyURL == "" {
		resp.Diagnostics.AddError(
			"Kube API Proxy Not Configured",
			"kube_api_proxy_url must be set in the provider configuration or via the FUNDAMENT_KUBE_API_PROXY_URL environment variable to manage plugin installations.",
		)
		return
	}

	clusterID := plan.ClusterID.ValueString()
	pluginName := plan.PluginName.ValueString()
	image := plan.Image.ValueString()

	ctx, cancel := context.WithTimeout(ctx, 40*time.Minute)
	defer cancel()

	tflog.Debug(ctx, "Waiting for cluster to be running before installing plugin", map[string]any{"cluster_id": clusterID})

	if err := waitForClusterRunning(ctx, r.client, clusterID); err != nil {
		resp.Diagnostics.AddError(
			"Cluster Not Ready",
			fmt.Sprintf("Cluster %q did not reach RUNNING state: %s", clusterID, err.Error()),
		)
		return
	}

	crd := pluginInstallationCRD{
		APIVersion: "plugins.fundament.io/v1",
		Kind:       "PluginInstallation",
		Metadata:   pluginInstallationMetadata{Name: pluginName},
		Spec: pluginInstallationSpec{
			PluginName: pluginName,
			Image:      image,
		},
	}

	body, err := json.Marshal(crd)
	if err != nil {
		resp.Diagnostics.AddError("JSON Marshal Error", err.Error())
		return
	}

	postURL := r.listURL(clusterID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(body))
	if err != nil {
		resp.Diagnostics.AddError("Unable to Build Request", err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := r.client.KubeProxyHTTPClient.Do(httpReq)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create Plugin Installation", err.Error())
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == http.StatusConflict {
		existingCRD, errMsg := r.fetchCRD(ctx, clusterID, pluginName)
		if errMsg != "" {
			resp.Diagnostics.AddError("Unable to Read Existing Plugin Installation on 409", errMsg)
			return
		}
		if existingCRD.Spec.Image != image {
			resp.Diagnostics.AddError(
				"Plugin Installation Already Exists With Different Image",
				fmt.Sprintf("A plugin installation for %q already exists on cluster %q with image %q. Import it with `terraform import fundament_plugin_installation.<name> %s/%s` or delete it manually.", pluginName, clusterID, existingCRD.Spec.Image, clusterID, pluginName),
			)
			return
		}
		tflog.Info(ctx, "Plugin installation already exists with matching image, treating as idempotent success", map[string]any{"plugin_name": pluginName})
	} else if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		resp.Diagnostics.AddError(
			"Unable to Create Plugin Installation",
			fmt.Sprintf("kube-api-proxy returned HTTP %d: %s", httpResp.StatusCode, string(respBody)),
		)
		return
	}

	plan.ID = types.StringValue(clusterID + "/" + pluginName)

	tflog.Debug(ctx, "Polling plugin installation until phase is set", map[string]any{"plugin_name": pluginName})

	for {
		crdState, errMsg := r.fetchCRD(ctx, clusterID, pluginName)
		if errMsg != "" {
			resp.Diagnostics.AddError("Unable to Poll Plugin Installation Phase", errMsg)
			return
		}

		if crdState.Status.Phase != "" {
			plan.Phase = types.StringValue(crdState.Status.Phase)
			break
		}

		t := time.NewTimer(10 * time.Second)
		select {
		case <-ctx.Done():
			t.Stop()
			resp.Diagnostics.AddError("Timeout Waiting for Plugin Phase", "Context deadline exceeded while waiting for plugin installation phase to be set.")
			return
		case <-t.C:
		}
	}

	tflog.Info(ctx, "Created plugin installation", map[string]any{"id": plan.ID.ValueString(), "phase": plan.Phase.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PluginInstallationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PluginInstallationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError("Client Not Configured", "The Fundament client was not configured. Please report this issue to the provider developers.")
		return
	}

	if r.client.KubeProxyURL == "" {
		resp.Diagnostics.AddError("Kube API Proxy Not Configured", "kube_api_proxy_url must be set to manage plugin installations.")
		return
	}

	clusterID := state.ClusterID.ValueString()
	pluginName := state.PluginName.ValueString()

	getURL := r.resourceURL(clusterID, pluginName)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Build Request", err.Error())
		return
	}

	httpResp, err := r.client.KubeProxyHTTPClient.Do(httpReq)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Read Plugin Installation", err.Error())
		return
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == http.StatusNotFound {
		tflog.Info(ctx, "Plugin installation not found, removing from state", map[string]any{"plugin_name": pluginName})
		resp.State.RemoveResource(ctx)
		return
	}

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		resp.Diagnostics.AddError("Unable to Read Plugin Installation", fmt.Sprintf("kube-api-proxy returned HTTP %d: %s", httpResp.StatusCode, string(respBody)))
		return
	}

	var crd pluginInstallationCRD
	if err := json.NewDecoder(httpResp.Body).Decode(&crd); err != nil {
		resp.Diagnostics.AddError("Unable to Parse Plugin Installation Response", err.Error())
		return
	}

	state.Phase = types.StringValue(crd.Status.Phase)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is not implemented — all attributes have RequiresReplace.
func (r *PluginInstallationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "All plugin_installation attributes require replacement; Update should never be called.")
}

func (r *PluginInstallationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PluginInstallationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError("Client Not Configured", "The Fundament client was not configured. Please report this issue to the provider developers.")
		return
	}

	if r.client.KubeProxyURL == "" {
		resp.Diagnostics.AddError("Kube API Proxy Not Configured", "kube_api_proxy_url must be set to manage plugin installations.")
		return
	}

	clusterID := state.ClusterID.ValueString()
	pluginName := state.PluginName.ValueString()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// Check whether the cluster is in a state where its Kubernetes API is reachable.
	getClusterReq := connect.NewRequest(organizationv1.GetClusterRequest_builder{
		ClusterId: clusterID,
	}.Build())

	clusterResp, err := r.client.ClusterService.GetCluster(ctx, getClusterReq)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			tflog.Info(ctx, "Cluster not found, skipping plugin CRD deletion", map[string]any{"cluster_id": clusterID})
			return
		}
		resp.Diagnostics.AddError("Unable to Check Cluster Status Before Plugin Deletion", err.Error())
		return
	}

	clusterStatus := clusterResp.Msg.GetCluster().GetStatus()
	switch clusterStatus {
	case organizationv1.ClusterStatus_CLUSTER_STATUS_ERROR,
		organizationv1.ClusterStatus_CLUSTER_STATUS_DELETING,
		organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPING,
		organizationv1.ClusterStatus_CLUSTER_STATUS_STOPPED:
		tflog.Info(ctx, "Cluster Kubernetes API unreachable, skipping CRD deletion", map[string]any{
			"cluster_id": clusterID,
			"status":     clusterStatusToString(clusterStatus),
		})
		return
	}

	deleteURL := r.resourceURL(clusterID, pluginName)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Build Request", err.Error())
		return
	}

	httpResp, err := r.client.KubeProxyHTTPClient.Do(httpReq)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Delete Plugin Installation", err.Error())
		return
	}
	defer httpResp.Body.Close()

	switch httpResp.StatusCode {
	case http.StatusOK, http.StatusAccepted, http.StatusNoContent, http.StatusNotFound:
		tflog.Info(ctx, "Deleted plugin installation", map[string]any{"plugin_name": pluginName})
	default:
		respBody, _ := io.ReadAll(httpResp.Body)
		resp.Diagnostics.AddError("Unable to Delete Plugin Installation", fmt.Sprintf("kube-api-proxy returned HTTP %d: %s", httpResp.StatusCode, string(respBody)))
	}
}

func (r *PluginInstallationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Import ID must be in the form {cluster_id}/{plugin_name}, got: %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("plugin_name"), parts[1])...)
}

func (r *PluginInstallationResource) listURL(clusterID string) string {
	return fmt.Sprintf("%s/clusters/%s/apis/plugins.fundament.io/v1/plugininstallations",
		strings.TrimRight(r.client.KubeProxyURL, "/"), clusterID)
}

func (r *PluginInstallationResource) resourceURL(clusterID, pluginName string) string {
	return r.listURL(clusterID) + "/" + pluginName
}

// fetchCRD performs a GET and returns the parsed CRD, or a non-empty error string on failure.
func (r *PluginInstallationResource) fetchCRD(ctx context.Context, clusterID, pluginName string) (*pluginInstallationCRD, string) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, r.resourceURL(clusterID, pluginName), nil)
	if err != nil {
		return nil, err.Error()
	}

	httpResp, err := r.client.KubeProxyHTTPClient.Do(httpReq)
	if err != nil {
		return nil, err.Error()
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Sprintf("kube-api-proxy returned HTTP %d: %s", httpResp.StatusCode, string(body))
	}

	var crd pluginInstallationCRD
	if err := json.NewDecoder(httpResp.Body).Decode(&crd); err != nil {
		return nil, err.Error()
	}

	return &crd, ""
}
