-- NULL means the controlled, restartable Go NFC backfill has not reached a row.
ALTER TABLE images ADD COLUMN filename_search TEXT COLLATE BINARY;
ALTER TABLE image_exif ADD COLUMN camera_search TEXT COLLATE BINARY;
ALTER TABLE image_exif ADD COLUMN lens_search TEXT COLLATE BINARY;
ALTER TABLE albums ADD COLUMN name_nfc TEXT COLLATE BINARY;
ALTER TABLE albums ADD COLUMN name_search TEXT COLLATE BINARY;
CREATE INDEX idx_images_owner_state_created_id ON images(user_id, state, created_at, id);
CREATE INDEX idx_images_owner_album_state_created_id ON images(user_id, album_id, state, created_at, id);
CREATE INDEX idx_albums_owner_name_nfc ON albums(user_id, name_nfc);
CREATE INDEX idx_albums_owner_name_search_id ON albums(user_id, name_search, id);
