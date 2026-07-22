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

-- name: RegionKubernetesVersionResolve :one
-- Resolve (region name, version) to the catalog ids; no row means the version
-- is not offered in that region (or the region does not exist).
SELECT
    catalog.region_kubernetes_versions.region_id,
    catalog.region_kubernetes_versions.kubernetes_version_id
FROM catalog.region_kubernetes_versions
JOIN catalog.regions ON catalog.regions.id = catalog.region_kubernetes_versions.region_id
JOIN catalog.kubernetes_versions ON catalog.kubernetes_versions.id = catalog.region_kubernetes_versions.kubernetes_version_id
WHERE catalog.regions.name = @region_name
  AND catalog.kubernetes_versions.version = @version;

-- name: RegionMachineTypeResolve :one
-- Resolve (region name, machine type name) to the region_machine_types row; no
-- row means the machine type is not offered in that region.
SELECT
    catalog.region_machine_types.id
FROM catalog.region_machine_types
JOIN catalog.regions ON catalog.regions.id = catalog.region_machine_types.region_id
JOIN catalog.machine_types ON catalog.machine_types.id = catalog.region_machine_types.machine_type_id
WHERE catalog.regions.name = @region_name
  AND catalog.machine_types.name = @machine_type_name;
