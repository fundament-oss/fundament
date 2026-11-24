package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/fundament-oss/fundament/common/dbconst"
	db "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/db/gen"
	adminv1 "github.com/fundament-oss/fundament/marketplace-admin-api/pkg/proto/gen/admin/v1"
)

const (
	defaultFilesPageSize = 500
)

// loadedAnalysis is one version's analysis row, decoded. nil row means the
// version has no analysis (yet).
type loadedAnalysis struct {
	row      *db.ImageAnalysisGetByDefinitionIDRow
	packages []analyzedPackage
	files    []analyzedFile
}

func (a *loadedAnalysis) completed() bool {
	return a.row != nil && a.row.Status == dbconst.PluginImageAnalyseStatus_Completed
}

func (s *Server) loadAnalysis(ctx context.Context, versionID uuid.UUID, withLists bool) (*loadedAnalysis, error) {
	row, err := s.queries.ImageAnalysisGetByDefinitionID(ctx, db.ImageAnalysisGetByDefinitionIDParams{
		PluginDefinitionID: versionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return &loadedAnalysis{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading image analysis: %w", err)
	}

	loaded := &loadedAnalysis{row: &row}
	if withLists && loaded.completed() {
		if err := json.Unmarshal(row.Packages, &loaded.packages); err != nil {
			return nil, fmt.Errorf("decoding packages: %w", err)
		}
		if err := json.Unmarshal(row.Files, &loaded.files); err != nil {
			return nil, fmt.Errorf("decoding files: %w", err)
		}
	}
	return loaded, nil
}

// GetImageAnalysis serves a version's analysis, diffed against a base.
func (s *Server) GetImageAnalysis(
	ctx context.Context,
	req *adminv1.GetImageAnalysisRequest,
) (*adminv1.GetImageAnalysisResponse, error) {
	versionID := uuid.MustParse(req.GetPluginVersionId())
	current, err := s.loadAnalysis(ctx, versionID, true)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	response := adminv1.GetImageAnalysisResponse_builder{
		Analysis: analysisMessage(versionID, current),
	}

	base := &loadedAnalysis{}
	if req.GetBasePluginVersionId() != "" {
		baseID := uuid.MustParse(req.GetBasePluginVersionId())
		base, err = s.loadAnalysis(ctx, baseID, true)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		response.Base = analysisMessage(baseID, base)
	}

	compared := current.completed() && base.completed()
	response.Compared = compared
	query := strings.ToLower(strings.TrimSpace(req.GetFilesQuery()))

	packages := diffPackages(current.packages, base.packages, compared)
	packageChanges := make([]adminv1.Change, 0, len(packages))
	for _, diff := range packages {
		packageChanges = append(packageChanges, diff.change)
		response.Packages = append(response.Packages, adminv1.ImagePackage_builder{
			Name:        diff.pkg.Name,
			Version:     diff.pkg.Version,
			Type:        diff.pkg.Type,
			Purl:        diff.pkg.Purl,
			Licenses:    diff.pkg.Licenses,
			Change:      diff.change,
			BaseVersion: diff.baseVersion,
		}.Build())
	}
	response.PackageSummary = summarize(packageChanges)

	files := diffFiles(current.files, base.files, compared)
	fileChanges := make([]adminv1.Change, 0, len(files))
	matching := make([]fileDiff, 0, len(files))
	for _, diff := range files {
		fileChanges = append(fileChanges, diff.change)
		if keep(diff.change, req.GetFilesChangesOnly(), query, diff.file.Path) {
			matching = append(matching, diff)
		}
	}
	response.FileSummary = summarize(fileChanges)
	response.FilesMatching = int32(len(matching)) //nolint:gosec // capped at 2x the worker's file cap

	offset, err := parsePageToken(req.GetFilesPageToken(), len(matching))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	pageSize := int(req.GetFilesPageSize())
	if pageSize == 0 {
		pageSize = defaultFilesPageSize
	}
	end := min(offset+pageSize, len(matching))
	for _, diff := range matching[offset:end] {
		response.Files = append(response.Files, adminv1.ImageFile_builder{
			Path:   diff.file.Path,
			Size:   diff.file.Size,
			Mode:   uint32(diff.file.Mode), //nolint:gosec // permission bits, 0o7777 at most
			Link:   diff.file.Link,
			Change: diff.change,
		}.Build())
	}
	if end < len(matching) {
		response.NextFilesPageToken = strconv.Itoa(end)
	}

	return response.Build(), nil
}

// GetSbom serves the CycloneDX document of a completed analysis.
func (s *Server) GetSbom(ctx context.Context, req *adminv1.GetSbomRequest) (*adminv1.GetSbomResponse, error) {
	sbom, err := s.queries.ImageAnalysisSbomGetByDefinitionID(ctx, db.ImageAnalysisSbomGetByDefinitionIDParams{
		PluginDefinitionID: uuid.MustParse(req.GetPluginVersionId()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("no completed image analysis for this version"))
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("loading sbom: %w", err))
	}
	return adminv1.GetSbomResponse_builder{Sbom: sbom}.Build(), nil
}

func analysisMessage(versionID uuid.UUID, a *loadedAnalysis) *adminv1.ImageAnalysis {
	builder := adminv1.ImageAnalysis_builder{PluginVersionId: versionID.String()}
	if a.row == nil {
		return builder.Build()
	}

	var platforms []string
	_ = json.Unmarshal(a.row.Platforms, &platforms)

	builder.Status = analysisStatusFromDB(a.row.Status)
	builder.StatusInfo = a.row.StatusInfo.String
	builder.Analyzed = timestamptzOrNil(a.row.Analyzed)
	builder.ImageDigest = a.row.ImageDigest
	builder.Platform = a.row.Platform
	builder.Platforms = platforms
	builder.FilesTruncated = a.row.FilesTruncated
	return builder.Build()
}

func analysisStatusFromDB(status dbconst.PluginImageAnalyseStatus) adminv1.ImageAnalysisStatus {
	switch status {
	case dbconst.PluginImageAnalyseStatus_Pending:
		return adminv1.ImageAnalysisStatus_IMAGE_ANALYSIS_STATUS_PENDING
	case dbconst.PluginImageAnalyseStatus_Running:
		return adminv1.ImageAnalysisStatus_IMAGE_ANALYSIS_STATUS_RUNNING
	case dbconst.PluginImageAnalyseStatus_Retrying:
		return adminv1.ImageAnalysisStatus_IMAGE_ANALYSIS_STATUS_RETRYING
	case dbconst.PluginImageAnalyseStatus_Completed:
		return adminv1.ImageAnalysisStatus_IMAGE_ANALYSIS_STATUS_COMPLETED
	case dbconst.PluginImageAnalyseStatus_Failed:
		return adminv1.ImageAnalysisStatus_IMAGE_ANALYSIS_STATUS_FAILED
	default:
		panic("unhandled PluginImageAnalyseStatus: " + string(status))
	}
}
