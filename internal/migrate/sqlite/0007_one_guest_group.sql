-- At most one group may carry is_guest, mirroring groups_one_default.
-- Concurrent creation could leave several; keep the one the guest_group_id
-- setting names, otherwise the lowest id (the group the fallback lookup
-- already resolves), and clear the flag on the rest.
UPDATE groups SET is_guest = false
WHERE is_guest = true
  AND id <> COALESCE(
    (SELECT g.id FROM groups g, settings s
      WHERE s.key = 'guest_group_id' AND g.id = CAST(s.value AS INTEGER) AND g.is_guest = true),
    (SELECT MIN(id) FROM groups WHERE is_guest = true));
CREATE UNIQUE INDEX groups_one_guest ON groups (is_guest) WHERE is_guest = true;
