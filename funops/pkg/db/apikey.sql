-- name: APIKeyRevokeAllForOrganization :execrows
-- Revokes the organization's API keys that still work, for when the
-- organization is deleted. The keys stay, as revoked ones do in the console.
UPDATE authn.api_keys
SET revoked = now()
WHERE organization_id = @organization_id
  AND revoked IS NULL
  AND deleted IS NULL;
