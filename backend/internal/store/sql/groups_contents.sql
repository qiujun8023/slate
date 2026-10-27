-- name: ListGroups :many
SELECT g.id, g.name, g.sort_order,
       COUNT(c.id)::int AS content_count,
       (COUNT(c.id) * 15000 + COALESCE(SUM(a.size), 0))::bigint AS total_bytes
FROM groups g
LEFT JOIN contents c ON c.group_id = g.id
LEFT JOIN content_audio a ON a.content_id = c.id
WHERE g.owner_id = $1
GROUP BY g.id
ORDER BY g.sort_order, g.created_at;

-- name: GetGroup :one
SELECT * FROM groups WHERE id = $1;

-- name: LockGroup :one
SELECT id FROM groups WHERE id = $1 FOR UPDATE;

-- name: CreateGroup :one
INSERT INTO groups (id, owner_id, name, sort_order) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: NextGroupSortOrder :one
SELECT COALESCE(MAX(sort_order) + 1, 0)::int FROM groups WHERE owner_id = $1;

-- name: CountGroups :one
SELECT COUNT(*)::int FROM groups WHERE owner_id = $1;

-- name: FirstGroupID :one
SELECT id FROM groups WHERE owner_id = $1 ORDER BY sort_order, created_at LIMIT 1;

-- name: RenameGroup :exec
UPDATE groups SET name = $2, updated_at = now() WHERE id = $1;

-- name: DeleteGroup :exec
DELETE FROM groups WHERE id = $1;

-- name: SetGroupSortOrders :exec
UPDATE groups g SET sort_order = o.ord - 1, updated_at = now()
FROM unnest(@ids::text[]) WITH ORDINALITY AS o(id, ord)
WHERE g.id = o.id AND g.owner_id = @owner_id;

-- name: ListGroupIDs :many
SELECT id FROM groups WHERE owner_id = $1 ORDER BY sort_order, created_at;

-- name: ListContents :many
SELECT c.id, c.group_id, c.sort_order, c.kind, c.frame_name, c.image_hash,
       d.type AS dynamic_type, d.config AS dynamic_config, d.data AS dynamic_data,
       d.rendered_at, d.next_run_at, d.refresh_due_at, d.last_error AS dynamic_error,
       a.source AS audio_source, a.status AS audio_status, a.voice AS audio_voice,
       a.text AS audio_text, a.hash AS audio_hash, a.size AS audio_size,
       a.last_error AS audio_error, a.updated_at AS audio_updated_at
FROM contents c
LEFT JOIN content_dynamic d ON d.content_id = c.id
LEFT JOIN content_audio a ON a.content_id = c.id
WHERE c.group_id = $1
ORDER BY c.sort_order;

-- name: GetContent :one
SELECT c.id, c.group_id, c.sort_order, c.kind, c.frame_name, c.image_hash,
       d.type AS dynamic_type, d.config AS dynamic_config, d.data AS dynamic_data,
       d.rendered_at, d.next_run_at, d.refresh_due_at, d.last_error AS dynamic_error,
       a.source AS audio_source, a.status AS audio_status, a.voice AS audio_voice,
       a.text AS audio_text, a.hash AS audio_hash, a.size AS audio_size,
       a.last_error AS audio_error, a.updated_at AS audio_updated_at,
       g.owner_id
FROM contents c
JOIN groups g ON g.id = c.group_id
LEFT JOIN content_dynamic d ON d.content_id = c.id
LEFT JOIN content_audio a ON a.content_id = c.id
WHERE c.id = $1;

-- name: NextContentSortOrder :one
SELECT COALESCE(MAX(sort_order) + 1, 0)::int FROM contents WHERE group_id = $1;

-- name: CreateContent :exec
INSERT INTO contents (id, group_id, sort_order, kind, frame_name, image_hash)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: SetContentImage :exec
UPDATE contents SET image_hash = $2, updated_at = now() WHERE id = $1;

-- name: SetContentFrameName :exec
UPDATE contents SET frame_name = $2, updated_at = now() WHERE id = $1;

-- name: DeleteContent :exec
DELETE FROM contents WHERE id = $1;

-- name: ListContentIDs :many
SELECT id FROM contents WHERE group_id = $1 ORDER BY sort_order;

-- name: SetContentSortOrders :exec
UPDATE contents c SET sort_order = o.ord - 1, updated_at = now()
FROM unnest(@ids::text[]) WITH ORDINALITY AS o(id, ord)
WHERE c.id = o.id AND c.group_id = @group_id;

-- name: CompactContentSortOrders :exec
UPDATE contents c SET sort_order = r.pos - 1
FROM (SELECT q.id, row_number() OVER (ORDER BY q.sort_order) AS pos FROM contents q WHERE q.group_id = $1) r
WHERE c.id = r.id AND c.sort_order <> r.pos - 1;

-- name: BlobVisibleToUser :one
SELECT EXISTS (
    SELECT 1 FROM contents c JOIN groups g ON g.id = c.group_id
    LEFT JOIN content_audio a ON a.content_id = c.id
    WHERE g.owner_id = @user_id AND (c.image_hash = @hash OR a.hash = @hash)
);

-- name: BlobVisibleToDevice :one
SELECT EXISTS (
    SELECT 1 FROM devices dv JOIN groups g ON g.id = dv.group_id AND g.owner_id = dv.owner_id
    JOIN contents c ON c.group_id = g.id
    LEFT JOIN content_audio a ON a.content_id = c.id
    WHERE dv.id = @device_id AND (c.image_hash = @hash OR a.hash = @hash)
);

-- name: ReferencedBlobHashes :many
SELECT image_hash::text AS hash FROM contents
UNION
SELECT hash::text FROM content_audio WHERE hash IS NOT NULL;
