-- Queries used only by the one-time legacy import.

-- name: ImportUser :exec
INSERT INTO users (id, email, username, password_hash, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6);

-- name: ImportGroup :exec
INSERT INTO groups (id, owner_id, name, sort_order, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6);

-- name: ImportDevice :exec
INSERT INTO devices (id, mac, secret_hash, pair_code, name, owner_id, sort_order, group_id, battery_pct, rssi_dbm,
                     fw_version, registered_at, last_seen_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15);

-- name: ImportContent :exec
INSERT INTO contents (id, group_id, sort_order, kind, frame_name, image_hash, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ImportDynamic :exec
INSERT INTO content_dynamic (content_id, type, config, data, rendered_at, next_run_at, refresh_due_at)
VALUES ($1, $2, $3, $4, $5, now(), now());

-- name: SetAudioError :exec
UPDATE content_audio SET last_error = $2 WHERE content_id = $1;
