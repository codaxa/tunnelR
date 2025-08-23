-- Create "machines" table
CREATE TABLE "public"."machines" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "hostname" character varying(50) NOT NULL,
  "ip_address" inet NOT NULL,
  "auth_method" character varying(20) NULL DEFAULT 'readonly',
  "password" character varying(100) NULL,
  "key" character varying(100) NULL,
  "created_at" timestamptz NULL DEFAULT now(),
  "updated_at" timestamptz NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "chk_machines_auth_method" CHECK ((auth_method)::text = ANY ((ARRAY['password'::character varying, 'key'::character varying, 'both'::character varying])::text[]))
);
-- Create index "idx_machines_ip_address" to table: "machines"
CREATE UNIQUE INDEX "idx_machines_ip_address" ON "public"."machines" ("ip_address");
-- Create "machine_teams" table
CREATE TABLE "public"."machine_teams" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "machine_id" uuid NOT NULL,
  "team_id" uuid NOT NULL,
  "created_at" timestamptz NULL DEFAULT now(),
  PRIMARY KEY ("id", "machine_id", "team_id"),
  CONSTRAINT "fk_machine_teams_machine" FOREIGN KEY ("machine_id") REFERENCES "public"."machines" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "fk_machine_teams_team" FOREIGN KEY ("team_id") REFERENCES "public"."teams" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION
);
