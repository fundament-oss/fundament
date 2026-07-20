-- Backfills note authorship after created_by (free text) became created_by_id
-- (an FK to dcim.users) in migration 032. The task notes seeded in
-- 030_0101-content.sql are mapped onto their roster users so the note cards
-- still render an author; the asset/placement notes from 026_0101-content.sql
-- predate the user directory and keep a null author.
UPDATE dcim.notes SET created_by_id = '019dce30-0000-7000-8000-000000000001'
WHERE id IN ('019dcc00-0000-7000-8000-000000000005', '019dcc00-0000-7000-8000-000000000007');

UPDATE dcim.notes SET created_by_id = '019dce30-0000-7000-8000-000000000003'
WHERE id = '019dcc00-0000-7000-8000-000000000008';
