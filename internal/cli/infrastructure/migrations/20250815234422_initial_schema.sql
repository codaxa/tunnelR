-- Create "users" table
CREATE TABLE "public"."users" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "username" character varying(50) NOT NULL,
  "password" character varying(64) NOT NULL,
  "role" character varying(20) NULL DEFAULT 'user',
  "created_at" timestamptz NULL DEFAULT now(),
  "updated_at" timestamptz NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "chk_users_role" CHECK ((role)::text = ANY ((ARRAY['admin'::character varying, 'operator'::character varying, 'readonly'::character varying])::text[]))
);
-- Create index "idx_users_username" to table: "users"
CREATE UNIQUE INDEX "idx_users_username" ON "public"."users" ("username");
