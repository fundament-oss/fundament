// Package sdktesting provides test helpers for plugin developers.
package sdktesting

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	pluginsdk "github.com/fundament-oss/fundament/plugin-sdk"
	"github.com/fundament-oss/fundament/plugin-sdk/metadata"
	"github.com/fundament-oss/fundament/plugin-sdk/metadata/proto/gen/v1/pluginmetadatav1connect"
	"github.com/fundament-oss/fundament/plugin-sdk/observability"
)

// MockHost implements pluginsdk.Host for unit testing.
type MockHost struct {
	logger    *slog.Logger
	telemetry observability.TelemetryService
	ready     atomic.Bool
	status    pluginsdk.PluginStatus
	statusMu  sync.RWMutex

	// StatusHistory records all status updates for test assertions.
	StatusHistory []pluginsdk.PluginStatus
}

// NewMockHost creates a MockHost with a discard logger.
func NewMockHost() *MockHost {
	logger := slog.New(slog.NewJSONHandler(nil, nil))
	return &MockHost{
		logger:    logger,
		telemetry: observability.NewTelemetryService(logger, nil, nil),
	}
}

// NewMockHostWithLogger creates a MockHost with the given logger.
func NewMockHostWithLogger(logger *slog.Logger) *MockHost {
	return &MockHost{
		logger:    logger,
		telemetry: observability.NewTelemetryService(logger, nil, nil),
	}
}

func (h *MockHost) Logger() *slog.Logger {
	return h.logger
}

func (h *MockHost) Telemetry() observability.TelemetryService {
	return h.telemetry
}

func (h *MockHost) ReportStatus(status pluginsdk.PluginStatus) {
	h.statusMu.Lock()
	defer h.statusMu.Unlock()
	h.status = status
	h.StatusHistory = append(h.StatusHistory, status)
}

func (h *MockHost) ReportReady() {
	h.ready.Store(true)
}

// IsReady reports whether ReportReady has been called.
func (h *MockHost) IsReady() bool {
	return h.ready.Load()
}

// CurrentStatus returns the current plugin status.
func (h *MockHost) CurrentStatus() pluginsdk.PluginStatus {
	h.statusMu.RLock()
	defer h.statusMu.RUnlock()
	return h.status
}

// InProcessPlugin runs a plugin in-process for integration testing.
// It starts the plugin with an HTTP server and returns a metadata client
// and a cancel function to stop the plugin.
type InProcessPlugin struct {
	Plugin         pluginsdk.Plugin
	MetadataClient pluginmetadatav1connect.PluginMetadataServiceClient
	BaseURL        string
	cancel         context.CancelFunc
	g              *errgroup.Group
}

// RunInProcess starts a plugin in-process for integration testing.
// Call Stop() when done to shut down the plugin.
func RunInProcess(plugin pluginsdk.Plugin) (*InProcessPlugin, error) {
	logger := slog.New(slog.NewJSONHandler(nil, nil))
	telemetry := observability.NewTelemetryService(logger, nil, nil)

	h := &MockHost{
		logger:    logger,
		telemetry: telemetry,
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	mux := http.NewServeMux()

	metadataServer := metadata.NewServer(
		func() metadata.Status {
			s := h.CurrentStatus()
			return metadata.Status{Phase: string(s.Phase), Message: s.Message}
		},
		func() metadata.Definition {
			return toMetadataDefinition(plugin.Definition())
		},
	)
	path, handler := pluginmetadatav1connect.NewPluginMetadataServiceHandler(metadataServer)
	mux.Handle(path, handler)

	httpServer := &http.Server{Handler: mux}

	ctx, cancel := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		return httpServer.Shutdown(shutdownCtx)
	})

	g.Go(func() error {
		return plugin.Start(ctx, h)
	})

	baseURL := fmt.Sprintf("http://%s", listener.Addr().String())
	client := pluginmetadatav1connect.NewPluginMetadataServiceClient(http.DefaultClient, baseURL)

	return &InProcessPlugin{
		Plugin:         plugin,
		MetadataClient: client,
		BaseURL:        baseURL,
		cancel:         cancel,
		g:              g,
	}, nil
}

// Stop shuts down the in-process plugin.
func (p *InProcessPlugin) Stop() error {
	p.cancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Plugin.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("plugin shutdown: %w", err)
	}

	return p.g.Wait()
}

func toMetadataDefinition(def pluginsdk.PluginDefinition) metadata.Definition {
	orgMenu := make([]metadata.MenuEntryDef, len(def.Menu.Organization))
	for i, entry := range def.Menu.Organization {
		orgMenu[i] = metadata.MenuEntryDef{
			CRD:    entry.CRD,
			List:   entry.List,
			Detail: entry.Detail,
			Create: entry.Create,
			Icon:   entry.Icon,
		}
	}

	projectMenu := make([]metadata.MenuEntryDef, len(def.Menu.Project))
	for i, entry := range def.Menu.Project {
		projectMenu[i] = metadata.MenuEntryDef{
			CRD:    entry.CRD,
			List:   entry.List,
			Detail: entry.Detail,
			Create: entry.Create,
			Icon:   entry.Icon,
		}
	}

	rbacRules := make([]metadata.PolicyRuleDef, len(def.Permissions.RBAC))
	for i, rule := range def.Permissions.RBAC {
		rbacRules[i] = metadata.PolicyRuleDef{
			APIGroups: rule.APIGroups,
			Resources: rule.Resources,
			Verbs:     rule.Verbs,
		}
	}

	customComponents := make(map[string]metadata.ComponentMappingDef, len(def.CustomComponents))
	for k, v := range def.CustomComponents {
		customComponents[k] = metadata.ComponentMappingDef{
			List:   v.List,
			Detail: v.Detail,
		}
	}

	uiHints := make(map[string]metadata.UIHintDef, len(def.UIHints))
	for k, v := range def.UIHints {
		formGroups := make([]metadata.FormGroupDef, len(v.FormGroups))
		for i, fg := range v.FormGroups {
			formGroups[i] = metadata.FormGroupDef{
				Name:   fg.Name,
				Fields: fg.Fields,
			}
		}

		statusValues := make(map[string]metadata.StatusValueDef, len(v.StatusMapping.Values))
		for sk, sv := range v.StatusMapping.Values {
			statusValues[sk] = metadata.StatusValueDef{
				Badge: sv.Badge,
				Label: sv.Label,
			}
		}

		uiHints[k] = metadata.UIHintDef{
			FormGroups: formGroups,
			StatusMapping: metadata.StatusMappingDef{
				JSONPath: v.StatusMapping.JSONPath,
				Values:   statusValues,
			},
		}
	}

	return metadata.Definition{
		Name:        def.Metadata.Name,
		DisplayName: def.Metadata.DisplayName,
		Version:     def.Metadata.Version,
		Description: def.Metadata.Description,
		Author:      def.Metadata.Author,
		License:     def.Metadata.License,
		Icon:        def.Metadata.Icon,
		URLs: metadata.URLsDef{
			Homepage:      def.Metadata.URLs.Homepage,
			Repository:    def.Metadata.URLs.Repository,
			Documentation: def.Metadata.URLs.Documentation,
		},
		Tags: def.Metadata.Tags,
		Permissions: metadata.PermissionsDef{
			Capabilities: def.Permissions.Capabilities,
			RBAC:         rbacRules,
		},
		Menu: metadata.Menu{
			Organization: orgMenu,
			Project:      projectMenu,
		},
		CustomComponents: customComponents,
		UIHints:          uiHints,
		CRDs:             def.CRDs,
	}
}
