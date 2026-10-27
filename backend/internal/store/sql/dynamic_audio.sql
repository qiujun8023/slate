-- name: CreateDynamic :exec
INSERT INTO content_dynamic (content_id, type, config, data, next_run_at, refresh_due_at)
VALUES ($1, $2, $3, $4, now(), now());

-- name: GetDynamic :one
SELECT * FROM content_dynamic WHERE content_id = $1;

-- name: SaveDynamicRender :exec
UPDATE content_dynamic
SET data = $2, rendered_at = $3, next_run_at = $4, refresh_due_at = $5,
    last_error = $6, attempts = 0, lease_until = NULL
WHERE content_id = $1;

-- name: MarkDynamicError :exec
UPDATE content_dynamic
SET last_error = $2, attempts = attempts + 1, next_run_at = $3, refresh_due_at = $3, lease_until = NULL
WHERE content_id = $1;

-- name: UpdateDynamicConfig :exec
UPDATE content_dynamic SET config = $2, refresh_due_at = now(), lease_until = NULL WHERE content_id = $1;

-- name: ClaimDueDynamic :many
UPDATE content_dynamic SET lease_until = @lease_until
WHERE content_id IN (
    SELECT q.content_id FROM content_dynamic q
    WHERE q.refresh_due_at <= @now AND (q.lease_until IS NULL OR q.lease_until <= @now)
    ORDER BY q.refresh_due_at
    LIMIT @max_jobs
    FOR UPDATE SKIP LOCKED
)
RETURNING content_id, type, lease_until;

-- name: ReleaseDynamicLease :exec
UPDATE content_dynamic
SET lease_until = NULL, refresh_due_at = GREATEST(refresh_due_at, @due_at), last_error = @last_error
WHERE content_id = @content_id AND lease_until = @lease_until;

-- name: NextDynamicDueAt :one
SELECT refresh_due_at FROM content_dynamic WHERE refresh_due_at IS NOT NULL ORDER BY refresh_due_at LIMIT 1;

-- name: GetAudio :one
SELECT * FROM content_audio WHERE content_id = $1;

-- name: SetAudio :exec
INSERT INTO content_audio (content_id, source, status, voice, text, hash, size)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (content_id) DO UPDATE
SET source = EXCLUDED.source, status = EXCLUDED.status, voice = EXCLUDED.voice, text = EXCLUDED.text,
    hash = EXCLUDED.hash, size = EXCLUDED.size, attempts = 0, lease_until = NULL, last_error = NULL,
    updated_at = now();

-- name: DeleteAudio :execrows
DELETE FROM content_audio WHERE content_id = $1;

-- name: FailAbandonedTTS :execrows
-- A worker that died during the last allowed attempt leaves the row generating with no
-- attempts left; ClaimTTSJob would never pick it up again.
UPDATE content_audio
SET status = 'failed', lease_until = NULL, last_error = 'TTS 生成中断', updated_at = now()
WHERE source = 'tts' AND status = 'generating' AND attempts >= @max_attempts AND lease_until <= @now;

-- name: ClaimTTSJob :one
UPDATE content_audio
SET status = 'generating', lease_until = @lease_until, attempts = attempts + 1, last_error = NULL, updated_at = now()
WHERE content_id IN (
    SELECT q.content_id FROM content_audio q
    WHERE q.source = 'tts' AND q.attempts < @max_attempts
      AND ((q.status = 'pending' AND (q.lease_until IS NULL OR q.lease_until <= @now))
        OR (q.status = 'generating' AND q.lease_until <= @now))
    ORDER BY q.updated_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING content_id, voice, text, lease_until, attempts;

-- name: CompleteTTS :execrows
UPDATE content_audio
SET status = 'ready', hash = @hash, size = @size, lease_until = NULL, attempts = 0, last_error = NULL, updated_at = now()
WHERE content_id = @content_id AND status = 'generating' AND lease_until = @lease_until
  AND voice = @voice AND text = @text;

-- name: FailTTS :exec
UPDATE content_audio
SET status = @status, last_error = @last_error, lease_until = sqlc.narg(retry_at), updated_at = now()
WHERE content_id = @content_id AND status = 'generating' AND lease_until = @lease_until;
