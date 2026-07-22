-- Region catalog reads. Global catalog data (no RLS, no org filter) - what each
-- region offers feeds the console's region -> versions/machine-types cascade.

-- name: RegionList :many
SELECT id, name
FROM catalog.regions
ORDER BY name;

-- name: RegionKubernetesVersionList :many
SELECT
    catalog.region_kubernetes_versions.region_id,
    catalog.kubernetes_versions.id,
    catalog.kubernetes_versions.version
FROM catalog.region_kubernetes_versions
JOIN catalog.kubernetes_versions ON catalog.kubernetes_versions.id = catalog.region_kubernetes_versions.kubernetes_version_id
ORDER BY catalog.region_kubernetes_versions.region_id, catalog.kubernetes_versions.version;

-- name: RegionMachineTypeList :many
SELECT
    catalog.region_machine_types.region_id,
    catalog.region_machine_types.id,
    catalog.machine_types.name,
    catalog.machine_types.lcpu,
    catalog.machine_types.memory
FROM catalog.region_machine_types
JOIN catalog.machine_types ON catalog.machine_types.id = catalog.region_machine_types.machine_type_id
ORDER BY catalog.region_machine_types.region_id, catalog.machine_types.name;

-- name: RegionKubernetesVersionGet :one
-- Resolve a (region, version) availability pair to its display names; no row
-- means the version is not offered in that region.
SELECT
    catalog.regions.name AS region_name,
    catalog.kubernetes_versions.version
FROM catalog.region_kubernetes_versions
JOIN catalog.regions ON catalog.regions.id = catalog.region_kubernetes_versions.region_id
JOIN catalog.kubernetes_versions ON catalog.kubernetes_versions.id = catalog.region_kubernetes_versions.kubernetes_version_id
WHERE catalog.region_kubernetes_versions.region_id = $1
  AND catalog.region_kubernetes_versions.kubernetes_version_id = $2;

-- name: RegionMachineTypeGet :one
-- Resolve a region_machine_types row to its region + machine-type name.
SELECT
    catalog.region_machine_types.region_id,
    catalog.machine_types.name AS machine_type_name
FROM catalog.region_machine_types
JOIN catalog.machine_types ON catalog.machine_types.id = catalog.region_machine_types.machine_type_id
WHERE catalog.region_machine_types.id = $1;
