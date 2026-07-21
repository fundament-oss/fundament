-- Backfills note authorship after created_by (free text) became created_by_id
-- (an FK to dcim.users) in migration 032. The task notes seeded in
-- 030_0101-content.sql are mapped onto their roster users so the note cards
-- still render an author; note ...0006 ("Admin") has no roster entry, and the
-- asset/placement notes from 026_0101-content.sql predate the user directory,
-- so both keep a null author.
--
-- NOTE (migrations 031/032 are not data-safe on a populated database):
--   * 031 casts dcim.tasks.assignee_id from text to uuid and then immediately
--     VALIDATEs an FK against the brand-new, empty dcim.users. Any pre-existing
--     non-null assignee_id therefore fails the migration outright — on the cast
--     if it is not a uuid, on the validate if it is.
--   * 032 drops notes.created_by unconditionally. Existing free-text authorship
--     is lost; there is no backfill, because there is nothing to map those names
--     onto until dcim.users is provisioned.
-- Both are fine as long as no environment holds real task or note data, which is
-- the case at the time of writing. Before this reaches an environment that does,
-- dcim.users has to be provisioned first and these two migrations reworked to
-- backfill from it rather than assuming an empty starting point.
UPDATE dcim.notes SET created_by_id = '019dce30-0000-7000-8000-000000000001'
WHERE id IN ('019dcc00-0000-7000-8000-000000000005', '019dcc00-0000-7000-8000-000000000007');

UPDATE dcim.notes SET created_by_id = '019dce30-0000-7000-8000-000000000003'
WHERE id = '019dcc00-0000-7000-8000-000000000008';
