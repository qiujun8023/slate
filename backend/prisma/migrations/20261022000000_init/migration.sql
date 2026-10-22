-- CreateSchema
CREATE SCHEMA IF NOT EXISTS "public";

-- CreateExtension
-- 邮箱、用户名用 citext，比较和唯一约束不区分大小写
CREATE EXTENSION IF NOT EXISTS citext;

-- CreateEnum
CREATE TYPE "content_kind" AS ENUM ('image', 'dynamic');

-- CreateEnum
CREATE TYPE "content_audio_status" AS ENUM ('none', 'pending', 'generating', 'ready', 'failed');

-- CreateEnum
CREATE TYPE "content_audio_source" AS ENUM ('upload', 'tts');

-- CreateTable
CREATE TABLE "users" (
    "id" TEXT NOT NULL,
    "email" CITEXT NOT NULL,
    "username" CITEXT,
    "password" VARCHAR(255) NOT NULL,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "users_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "devices" (
    "id" TEXT NOT NULL,
    "mac" VARCHAR(17) NOT NULL,
    "secret_hash" CHAR(64) NOT NULL,
    "pair_code" CHAR(6) NOT NULL,
    "name" VARCHAR(64),
    "owner_user_id" TEXT,
    "sort_order" INTEGER NOT NULL DEFAULT 0,
    "selected_group_id" TEXT,
    "last_registered_at" TIMESTAMP(3),
    "last_seen_at" TIMESTAMP(3),
    "battery_pct" INTEGER,
    "rssi_dbm" INTEGER,
    "fw_version" VARCHAR(32),
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "devices_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "groups" (
    "id" TEXT NOT NULL,
    "name" VARCHAR(64) NOT NULL,
    "structure_etag" VARCHAR(64) NOT NULL DEFAULT 'empty',
    "manifest_etag" VARCHAR(64) NOT NULL DEFAULT 'empty',
    "owner_user_id" TEXT,
    "sort_order" INTEGER NOT NULL DEFAULT 0,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "groups_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "contents" (
    "id" TEXT NOT NULL,
    "group_id" TEXT NOT NULL,
    "sort_order" INTEGER NOT NULL,
    "frame_name" VARCHAR(64),
    "content_etag" VARCHAR(64) NOT NULL DEFAULT 'empty',
    "image_etag" VARCHAR(64) NOT NULL,
    "audio_etag" VARCHAR(64),
    "image_size" INTEGER NOT NULL,
    "audio_size" INTEGER,
    "audio_status" "content_audio_status" NOT NULL DEFAULT 'none',
    "audio_source" "content_audio_source",
    "audio_voice" VARCHAR(32),
    "audio_text" VARCHAR(512),
    "audio_last_error" VARCHAR(512),
    "audio_updated_at" TIMESTAMP(3),
    "audio_lease_until" TIMESTAMP(3),
    "audio_attempts" INTEGER NOT NULL DEFAULT 0,
    "kind" "content_kind" NOT NULL DEFAULT 'image',
    "dynamic_type" VARCHAR(32),
    "dynamic_config" JSONB,
    "dynamic_data" JSONB,
    "dynamic_last_run_at" TIMESTAMP(3),
    "dynamic_next_run_at" TIMESTAMP(3),
    "dynamic_refresh_due_at" TIMESTAMP(3),
    "dynamic_refresh_lease_until" TIMESTAMP(3),
    "dynamic_refresh_attempts" INTEGER NOT NULL DEFAULT 0,
    "dynamic_last_error" VARCHAR(512),
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "contents_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "users_email_key" ON "users"("email");

-- CreateIndex
CREATE UNIQUE INDEX "users_username_key" ON "users"("username");

-- CreateIndex
CREATE UNIQUE INDEX "devices_mac_key" ON "devices"("mac");

-- CreateIndex
CREATE UNIQUE INDEX "devices_secret_hash_key" ON "devices"("secret_hash");

-- CreateIndex
CREATE UNIQUE INDEX "devices_pair_code_key" ON "devices"("pair_code");

-- CreateIndex
CREATE INDEX "devices_selected_group_id_idx" ON "devices"("selected_group_id");

-- CreateIndex
CREATE INDEX "devices_last_seen_at_idx" ON "devices"("last_seen_at");

-- CreateIndex
CREATE UNIQUE INDEX "devices_owner_user_id_sort_order_key" ON "devices"("owner_user_id", "sort_order");

-- CreateIndex
CREATE UNIQUE INDEX "groups_owner_user_id_sort_order_key" ON "groups"("owner_user_id", "sort_order");

-- CreateIndex
CREATE INDEX "contents_group_id_kind_idx" ON "contents"("group_id", "kind");

-- CreateIndex
CREATE INDEX "contents_audio_worker_idx" ON "contents"("audio_source", "audio_status", "audio_lease_until", "audio_updated_at");

-- CreateIndex
CREATE INDEX "contents_dynamic_refresh_worker_idx" ON "contents"("kind", "dynamic_refresh_due_at", "dynamic_refresh_lease_until");

-- CreateIndex
CREATE INDEX "contents_kind_dynamic_type_idx" ON "contents"("kind", "dynamic_type");

-- CreateIndex
CREATE UNIQUE INDEX "contents_group_id_sort_order_key" ON "contents"("group_id", "sort_order");

-- AddForeignKey
ALTER TABLE "devices" ADD CONSTRAINT "devices_owner_user_id_fkey" FOREIGN KEY ("owner_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "devices" ADD CONSTRAINT "devices_selected_group_id_fkey" FOREIGN KEY ("selected_group_id") REFERENCES "groups"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "groups" ADD CONSTRAINT "groups_owner_user_id_fkey" FOREIGN KEY ("owner_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "contents" ADD CONSTRAINT "contents_group_id_fkey" FOREIGN KEY ("group_id") REFERENCES "groups"("id") ON DELETE CASCADE ON UPDATE CASCADE;

