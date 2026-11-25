-- Image analysis reads for admin.v1.ReviewService (FUN-24). The worker writes
-- the rows; the backoffice only reads them.

-- name: ImageAnalysisGetByDefinitionID :one
-- Everything but the SBOM, which only GetSbom needs.
SELECT
	plugin_definition_id,
	status,
	status_info,
	analyzed,
	image_digest,
	platform,
	platforms,
	packages,
	files,
	files_truncated
FROM appstore.plugin_image_analyses
WHERE plugin_definition_id = sqlc.arg('plugin_definition_id')::uuid
  AND deleted IS NULL;

-- name: ImageAnalysisSbomGetByDefinitionID :one
SELECT sbom
FROM appstore.plugin_image_analyses
WHERE plugin_definition_id = sqlc.arg('plugin_definition_id')::uuid
  AND status = 'completed'
  AND deleted IS NULL;
