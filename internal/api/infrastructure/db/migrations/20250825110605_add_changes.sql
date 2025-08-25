-- Create "access_logs" table
CREATE TABLE "public"."access_logs" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "user_id" uuid NOT NULL,
  "machine_id" uuid NOT NULL,
  "temp_username" character varying(50) NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "created_at" timestamptz NULL DEFAULT now(),
  "updated_at" timestamptz NULL DEFAULT now(),
  "session_status" character varying(20) NULL DEFAULT 'provisioned',
  PRIMARY KEY ("id")
);
