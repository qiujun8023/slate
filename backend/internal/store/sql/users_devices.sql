-- name: CreateUser :one
INSERT INTO users (id, email, username, password_hash)
VALUES ($1, $2, $3, $4)
RETURNING id, email, username;

-- name: GetUserByEmail :one
SELECT id, email, username, password_hash FROM users WHERE email = $1;

-- name: GetUserByUsername :one
SELECT id, email, username, password_hash FROM users WHERE username = $1;

-- name: GetUser :one
SELECT id, email, username FROM users WHERE id = $1;

-- name: LockUser :one
SELECT id FROM users WHERE id = $1 FOR UPDATE;

-- name: GetDeviceForRegister :one
SELECT id, owner_id, registered_at, secret_hash FROM devices WHERE mac = $1 FOR UPDATE;

-- name: CreateDevice :exec
INSERT INTO devices (id, mac, secret_hash, pair_code, registered_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ResetDevice :exec
UPDATE devices
SET secret_hash = $2, pair_code = $3, owner_id = NULL, group_id = NULL, sort_order = 0,
    registered_at = $4, updated_at = now()
WHERE id = $1;

-- name: GetDeviceIDBySecret :one
SELECT id FROM devices WHERE secret_hash = $1;

-- name: PairCodesInUse :many
SELECT pair_code FROM devices WHERE pair_code = ANY(@codes::text[]);

-- name: RecordTelemetry :exec
UPDATE devices
SET last_seen_at = now(),
    battery_pct = COALESCE(sqlc.narg(battery_pct), battery_pct),
    rssi_dbm = COALESCE(sqlc.narg(rssi_dbm), rssi_dbm),
    fw_version = COALESCE(sqlc.narg(fw_version), fw_version)
WHERE id = $1;

-- name: GetDevice :one
SELECT * FROM devices WHERE id = $1;

-- name: LockOwnedDevice :one
SELECT * FROM devices WHERE id = $1 AND owner_id = $2 FOR UPDATE;

-- name: ListDevices :many
SELECT * FROM devices WHERE owner_id = $1 ORDER BY sort_order, created_at;

-- name: GetDeviceByPairCode :one
SELECT * FROM devices WHERE pair_code = $1 FOR UPDATE;

-- name: NextDeviceSortOrder :one
SELECT COALESCE(MAX(sort_order) + 1, 0)::int FROM devices WHERE owner_id = $1;

-- name: ClaimDevice :one
UPDATE devices
SET owner_id = $2, sort_order = $3, pair_code = $4, group_id = $5, updated_at = now()
WHERE id = $1 AND owner_id IS NULL
RETURNING *;

-- name: UnbindDevice :execrows
UPDATE devices
SET owner_id = NULL, group_id = NULL, sort_order = 0, pair_code = $3, updated_at = now()
WHERE id = $1 AND owner_id = $2;

-- name: RenameDevice :exec
UPDATE devices SET name = $2, updated_at = now() WHERE id = $1;

-- name: SetDeviceGroup :exec
UPDATE devices SET group_id = $2, updated_at = now() WHERE id = $1;

-- name: AssignGroupToUngroupedDevices :execrows
UPDATE devices SET group_id = $2, updated_at = now() WHERE owner_id = $1 AND group_id IS NULL;

-- name: SetDeviceSortOrders :exec
UPDATE devices d SET sort_order = o.ord - 1, updated_at = now()
FROM unnest(@ids::text[]) WITH ORDINALITY AS o(id, ord)
WHERE d.id = o.id AND d.owner_id = @owner_id;
