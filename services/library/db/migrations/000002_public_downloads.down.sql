DROP TRIGGER IF EXISTS library_download_start_events_append_only ON library_download_start_events;
DROP FUNCTION IF EXISTS library_download_start_append_only();
DROP TABLE IF EXISTS library_download_start_events;
DROP TABLE IF EXISTS library_public_material_snapshots;
DROP TABLE IF EXISTS library_public_releases;
