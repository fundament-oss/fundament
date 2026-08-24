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
	getStatus func() PluginStatus
	uninstall func(context.Context) error
}

// NewMetadataHandler creates a metadata handler that serves plugin status
// via the PluginMetadataService Connect RPC service.
func NewMetadataHandler(statusFn func() PluginStatus, uninstallFn func(context.Context) error) *metadataHandler {
	return &metadataHandler{
		getStatus: statusFn,
		uninstall: uninstallFn,
	}
}

func (h *metadataHandler) GetStatus(_ context.Context, _ *pb.GetStatusRequest) (*pb.GetStatusResponse, error) {
	status := h.getStatus()
	return &pb.GetStatusResponse{
		Phase:   ptr(string(status.Phase)),
		Message: ptr(status.Message),
	}, nil
}

func (h *metadataHandler) RequestUninstall(ctx context.Context, _ *pb.RequestUninstallRequest) (*pb.RequestUninstallResponse, error) {
	if err := h.uninstall(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return &pb.RequestUninstallResponse{}, nil
}
