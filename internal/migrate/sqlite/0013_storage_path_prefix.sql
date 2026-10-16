-- Folder inside the bucket or directory that holds every object of a storage
-- ('' = its root). The public base URL already points at that folder, so links
-- are base_url + '/' + key and the prefix only changes object keys.
ALTER TABLE storages ADD COLUMN path_prefix TEXT NOT NULL DEFAULT '';
