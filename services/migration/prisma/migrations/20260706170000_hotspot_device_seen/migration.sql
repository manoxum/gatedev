ALTER TABLE "hotspot_device_info" ADD COLUMN IF NOT EXISTS "first_seen_at" TIMESTAMPTZ;
ALTER TABLE "hotspot_device_info" ADD COLUMN IF NOT EXISTS "last_seen_at" TIMESTAMPTZ;
