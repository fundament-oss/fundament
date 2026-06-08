package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"connectrpc.com/connect"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
	ID             types.String `tfsdk:"id"`
	ClusterID      types.String `tfsdk:"cluster_id"`
	PluginName     types.String `tfsdk:"plugin_name"`
	PluginVersion  types.String `tfsdk:"plugin_version"`
	DefinitionHash types.String `tfsdk:"definition_hash"`
	Image          types.String `tfsdk:"image"`
	Phase          types.String `tfsdk:"phase"`
}

type pluginInstallationMetadata struct {
	Name string `json:"name"`
}

// pluginDefinitionRef is the immutable, content-addressed pin to the published
// PluginDefinition the installer consented to (FUN-17). The plugin-controller
// resolves the definition by DefinitionHash and materialises the plugin SA's
// RBAC from it.
type pluginDefinitionRef struct {
	PluginName     string `json:"pluginName"`
	PluginVersion  string `json:"pluginVersion"`
	DefinitionHash string `json:"definitionHash"`
}

type pluginInstallationSpec struct {
	Image         string              `json:"image"`
	DefinitionRef pluginDefinitionRef `json:"definitionRef"`
}

type pluginInstallationStatus struct {
	Phase   string `json:"phase"`
	Message string `json:"message,omitempty"`
}

const pluginInstallationAPIVersion = "plugins.fundament.io/v1"

// definitionHashRegex validates definition_hash: "sha256:" followed by a
// 64-character lowercase hex digest, or the placeholder "sha256:unknown" used
// until the marketplace (FUN-11) supplies real content hashes.
var definitionHashRegex = regexp.MustCompile(`^sha256:([a-f0-9]{64}|unknown)$`)

type pluginInstallationCRD struct {
	APIVersion string                     `json:"apiVersion"`
	Kind       string                     `json:"kind"`
	Metadata   pluginInstallationMetadata `json:"metadata"`
	Spec       pluginInstallationSpec     `json:"spec"`
	Status     pluginInstallationStatus   `json:"status"`
}

// pluginInstallationCreatePayload is used for POST requests; omits status so the
// API server does not receive an empty status object.
type pluginInstallationCreatePayload struct {
	APIVersion string                     `json:"apiVersion"`
	Kind       string                     `json:"kind"`
	Metadata   pluginInstallationMetadata `json:"metadata"`
	Spec       pluginInstallationSpec     `json:"spec"`
}

func NewPluginInstallationResource() resource.Resource {
	return &PluginInstallationResource{}
}

func (r *PluginInstallationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin_installation"
}

func (r *PluginInstallationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a plugin installation on a Fundament cluster via the kube-api-proxy. " +
			"Create waits up to 20 minutes for the cluster to reach RUNNING and for the plugin to reach the Running phase. " +
			"Delete is skipped (no-op) when the cluster is not in a state where its Kubernetes API is reachable (e.g. stopped, deleting); the CRD will be removed when the cluster is destroyed.",
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
				Description: "The name of the plugin to install. Must match a plugin in the Fundament catalog. Used as the installation's metadata.name and as definitionRef.pluginName. Changing this value forces a replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"plugin_version": schema.StringAttribute{
				Description: "The published version of the plugin definition to pin (definitionRef.pluginVersion). Optional; defaults to \"unknown\" until the marketplace supplies real versions (FUN-11). The pin is immutable, so changing this value forces a replacement.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("unknown"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"definition_hash": schema.StringAttribute{
				Description: "The sha256 content hash of the pinned plugin definition (definitionRef.definitionHash), prefixed with \"sha256:\". Optional; defaults to \"sha256:unknown\" until the marketplace supplies real hashes (FUN-11). The pin is immutable, so changing this value forces a replacement.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("sha256:unknown"),
				Validators: []validator.String{
					stringvalidator.RegexMatches(definitionHashRegex, "definition_hash must be 'sha256:' followed by a 64-character hex digest (or the placeholder 'sha256:unknown')"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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
	pluginVersion := plan.PluginVersion.ValueString()
	definitionHash := plan.DefinitionHash.ValueString()
	image := plan.Image.ValueString()

	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	tflog.Debug(ctx, "Waiting for cluster to be running before installing plugin", map[string]any{"cluster_id": clusterID})

	if err := waitForClusterRunning(ctx, r.client, clusterID); err != nil {
		resp.Diagnostics.AddError(
			"Cluster Not Ready",
			fmt.Sprintf("Cluster %q did not reach RUNNING state: %s", clusterID, err.Error()),
		)
		return
	}

	crd := pluginInstallationCreatePayload{
		APIVersion: pluginInstallationAPIVersion,
		Kind:       "PluginInstallation",
		Metadata:   pluginInstallationMetadata{Name: pluginName},
		Spec: pluginInstallationSpec{
			Image: image,
			DefinitionRef: pluginDefinitionRef{
				PluginName:     pluginName,
				PluginVersion:  pluginVersion,
				DefinitionHash: definitionHash,
			},
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
		conflictBody, _ := io.ReadAll(io.LimitReader(httpResp.Body, 64*1024))
		tflog.Debug(ctx, "Plugin installation POST returned 409, checking existing resource", map[string]any{"body": string(conflictBody)})
		existingCRD, err := r.fetchCRD(ctx, clusterID, pluginName)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Read Existing Plugin Installation on 409", err.Error())
			return
		}
		if existingCRD.Spec.Image != image ||
			existingCRD.Spec.DefinitionRef.PluginVersion != pluginVersion ||
			existingCRD.Spec.DefinitionRef.DefinitionHash != definitionHash {
			resp.Diagnostics.AddError(
				"Plugin Installation Already Exists With Different Configuration",
				fmt.Sprintf("A plugin installation for %q already exists on cluster %q with a different spec "+
					"(existing: image=%q version=%q hash=%q; planned: image=%q version=%q hash=%q). "+
					"Import it with `terraform import fundament_plugin_installation.<name> %s/%s` or delete it manually.",
					pluginName, clusterID,
					existingCRD.Spec.Image, existingCRD.Spec.DefinitionRef.PluginVersion, existingCRD.Spec.DefinitionRef.DefinitionHash,
					image, pluginVersion, definitionHash,
					clusterID, pluginName),
			)
			return
		}
		tflog.Info(ctx, "Plugin installation already exists with matching spec, treating as idempotent success", map[string]any{"plugin_name": pluginName})
	} else if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(httpResp.Body, 64*1024))
		resp.Diagnostics.AddError(
			"Unable to Create Plugin Installation",
			fmt.Sprintf("kube-api-proxy returned HTTP %d: %s", httpResp.StatusCode, string(respBody)),
		)
		return
	}

	// Persist partial state now: the CRD exists on the server, so if the wait
	// below fails we must still track the resource — otherwise Terraform treats
	// Create as failed and orphans the CRD (the next apply hits the 409 path).
	plan.ID = types.StringValue(clusterID + "/" + pluginName)
	plan.Phase = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Polling plugin installation until phase is Running", map[string]any{"plugin_name": pluginName})

	phase, err := r.waitForPluginRunning(ctx, clusterID, pluginName)
	if err != nil {
		resp.Diagnostics.AddError("Plugin Installation Did Not Reach Running", err.Error())
		return
	}
	plan.Phase = types.StringValue(phase)

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

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Check whether the cluster's Kubernetes API is reachable before hitting the proxy.
	getClusterReq := connect.NewRequest(organizationv1.GetClusterRequest_builder{
		ClusterId: clusterID,
	}.Build())

	clusterResp, err := r.client.ClusterService.GetCluster(ctx, getClusterReq)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			tflog.Info(ctx, "Cluster not found, removing plugin installation from state", map[string]any{"cluster_id": clusterID})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to Check Cluster Status Before Reading Plugin Installation", err.Error())
		return
	}

	clusterStatus := clusterResp.Msg.GetCluster().GetStatus()
	switch clusterStatus {
	case organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING,
		organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING:
		// API server is reachable; proceed with read.
	default:
		tflog.Info(ctx, "Cluster Kubernetes API unreachable, skipping plugin installation read", map[string]any{
			"cluster_id": clusterID,
			"status":     clusterStatusToString(clusterStatus),
		})
		return
	}

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
		respBody, _ := io.ReadAll(io.LimitReader(httpResp.Body, 64*1024))
		resp.Diagnostics.AddError("Unable to Read Plugin Installation", fmt.Sprintf("kube-api-proxy returned HTTP %d: %s", httpResp.StatusCode, string(respBody)))
		return
	}

	var crd pluginInstallationCRD
	if err := json.NewDecoder(httpResp.Body).Decode(&crd); err != nil {
		resp.Diagnostics.AddError("Unable to Parse Plugin Installation Response", err.Error())
		return
	}

	if crd.Spec.Image != "" {
		state.Image = types.StringValue(crd.Spec.Image)
	} else {
		state.Image = types.StringNull()
	}
	if crd.Spec.DefinitionRef.PluginVersion != "" {
		state.PluginVersion = types.StringValue(crd.Spec.DefinitionRef.PluginVersion)
	} else {
		state.PluginVersion = types.StringValue("unknown")
	}
	if crd.Spec.DefinitionRef.DefinitionHash != "" {
		state.DefinitionHash = types.StringValue(crd.Spec.DefinitionRef.DefinitionHash)
	} else {
		state.DefinitionHash = types.StringValue("sha256:unknown")
	}
	if crd.Status.Phase != "" {
		state.Phase = types.StringValue(crd.Status.Phase)
	} else {
		state.Phase = types.StringNull()
	}

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
	case organizationv1.ClusterStatus_CLUSTER_STATUS_RUNNING,
		organizationv1.ClusterStatus_CLUSTER_STATUS_UPGRADING:
		// API server is reachable; proceed with deletion.
	default:
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
		respBody, _ := io.ReadAll(io.LimitReader(httpResp.Body, 64*1024))
		resp.Diagnostics.AddError("Unable to Delete Plugin Installation", fmt.Sprintf("kube-api-proxy returned HTTP %d: %s", httpResp.StatusCode, string(respBody)))
	}
}

func (r *PluginInstallationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterID, pluginName, err := parseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("plugin_name"), pluginName)...)
}

// parseImportID splits a composite import ID of the form {cluster_id}/{plugin_name}.
func parseImportID(id string) (clusterID, pluginName string, err error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("import ID must be in the form {cluster_id}/{plugin_name}, got: %q", id)
	}
	return parts[0], parts[1], nil
}

func (r *PluginInstallationResource) listURL(clusterID string) string {
	return fmt.Sprintf("%s/clusters/%s/apis/%s/plugininstallations",
		strings.TrimRight(r.client.KubeProxyURL, "/"), url.PathEscape(clusterID), pluginInstallationAPIVersion)
}

func (r *PluginInstallationResource) resourceURL(clusterID, pluginName string) string {
	return r.listURL(clusterID) + "/" + url.PathEscape(pluginName)
}

// fetchCRD performs a GET and returns the parsed CRD, or an error on failure.
func (r *PluginInstallationResource) fetchCRD(ctx context.Context, clusterID, pluginName string) (*pluginInstallationCRD, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, r.resourceURL(clusterID, pluginName), nil)
	if err != nil {
		return nil, err
	}

	httpResp, err := r.client.KubeProxyHTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 64*1024))
		return nil, fmt.Errorf("kube-api-proxy returned HTTP %d: %s", httpResp.StatusCode, string(body))
	}

	var crd pluginInstallationCRD
	if err := json.NewDecoder(httpResp.Body).Decode(&crd); err != nil {
		return nil, err
	}

	return &crd, nil
}

// classifyPluginPhase maps a plugin installation status phase to a polling
// decision: done signals success (Running), terminal signals an unrecoverable
// phase. Any other phase (Pending, Deploying, or empty) means keep polling.
func classifyPluginPhase(phase string) (done bool, terminal bool) {
	switch phase {
	case "Running":
		return true, false
	case "Failed", "Terminating", "Degraded":
		return false, true
	default:
		return false, false
	}
}

// waitForPluginRunning polls fetchCRD until phase is Running, or a terminal/timeout condition.
// Returns the final phase string on success, or an error.
// Transient fetch errors are retried; only maxConsecutiveErrors consecutive failures are fatal.
func (r *PluginInstallationResource) waitForPluginRunning(ctx context.Context, clusterID, pluginName string) (string, error) {
	const maxConsecutiveErrors = 5

	lastPhase := ""

	err := pollWithBackoff(ctx, 10*time.Second, maxConsecutiveErrors, func(ctx context.Context) (done bool, fatal bool, err error) {
		crdState, err := r.fetchCRD(ctx, clusterID, pluginName)
		if err != nil {
			tflog.Debug(ctx, "Transient error polling plugin installation, retrying", map[string]any{
				"plugin_name": pluginName,
				"error":       err.Error(),
			})
			return false, false, fmt.Errorf("polling plugin installation status: %w", err)
		}

		lastPhase = crdState.Status.Phase
		done, terminal := classifyPluginPhase(lastPhase)
		switch {
		case done:
			return true, false, nil
		case terminal:
			return false, true, fmt.Errorf("plugin installation for %q entered phase %q: %s", pluginName, lastPhase, crdState.Status.Message)
		default:
			tflog.Debug(ctx, "Plugin installation not yet running", map[string]any{"plugin_name": pluginName, "phase": lastPhase})
			return false, false, nil
		}
	})

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "", fmt.Errorf("timed out waiting for plugin %q to reach Running (last phase: %q): %w", pluginName, lastPhase, err)
	}
	if err != nil {
		return "", err
	}
	return lastPhase, nil
}
