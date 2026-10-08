CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE IF NOT EXISTS users (
 id bigserial PRIMARY KEY, username text NOT NULL UNIQUE, password_hash text NOT NULL,
 display_name text NOT NULL, active boolean NOT NULL DEFAULT true
);
CREATE TABLE IF NOT EXISTS tenants (id bigserial PRIMARY KEY, name text NOT NULL);
CREATE TABLE IF NOT EXISTS members (
 id bigserial PRIMARY KEY,tenant_id bigint NOT NULL REFERENCES tenants,user_id bigint NOT NULL REFERENCES users,
 role text NOT NULL DEFAULT 'teacher', permissions jsonb NOT NULL DEFAULT '[]',
 balance bigint NOT NULL DEFAULT 0 CHECK(balance>=0),reserved bigint NOT NULL DEFAULT 0 CHECK(reserved>=0 AND reserved<=balance),
 active boolean NOT NULL DEFAULT true, UNIQUE(tenant_id,user_id),UNIQUE(tenant_id,id)
);
ALTER TABLE members ADD COLUMN IF NOT EXISTS plan text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS sessions (id text PRIMARY KEY,user_id bigint NOT NULL REFERENCES users,expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS documents (
 id text PRIMARY KEY,tenant_id bigint NOT NULL REFERENCES tenants,member_id bigint NOT NULL,
 name text NOT NULL, status text NOT NULL DEFAULT 'pending',error text NOT NULL DEFAULT '',pages jsonb NOT NULL DEFAULT '[]',
 source_url text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,id),FOREIGN KEY(tenant_id,member_id) REFERENCES members(tenant_id,id)
);
ALTER TABLE documents ADD COLUMN IF NOT EXISTS upload_path text NOT NULL DEFAULT '';
ALTER TABLE documents ADD COLUMN IF NOT EXISTS lease_until timestamptz;
CREATE TABLE IF NOT EXISTS chunks (
 id text PRIMARY KEY,tenant_id bigint NOT NULL,document_id text NOT NULL,seq integer NOT NULL,
 content text NOT NULL,location text NOT NULL,embedding vector(1536),
 FOREIGN KEY(tenant_id,document_id) REFERENCES documents(tenant_id,id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS chunks_tenant ON chunks(tenant_id,document_id);
CREATE TABLE IF NOT EXISTS tasks (
 id text PRIMARY KEY,tenant_id bigint NOT NULL,member_id bigint NOT NULL,type text NOT NULL,
 status text NOT NULL DEFAULT 'pending',stage text NOT NULL DEFAULT 'queued',input jsonb NOT NULL,
 result jsonb NOT NULL DEFAULT '{}',sources jsonb NOT NULL DEFAULT '[]',error text NOT NULL DEFAULT '',
 mode text NOT NULL,cost bigint NOT NULL CHECK(cost>0),charge_state text NOT NULL DEFAULT 'reserved',
 idempotency_key text NOT NULL,pptx_path text NOT NULL DEFAULT '',mp4_path text NOT NULL DEFAULT '',
 revision integer NOT NULL DEFAULT 0,attempt integer NOT NULL DEFAULT 0,lease_until timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,id),UNIQUE(tenant_id,member_id,idempotency_key),
 FOREIGN KEY(tenant_id,member_id) REFERENCES members(tenant_id,id),
 CHECK(charge_state IN ('reserved','settled','released'))
);
CREATE INDEX IF NOT EXISTS tasks_queue ON tasks(status,created_at);
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS display_title text NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS provider_job_id text NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS provider_model text NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
CREATE TABLE IF NOT EXISTS task_events (
 id bigserial PRIMARY KEY,tenant_id bigint NOT NULL,task_id text NOT NULL,stage text NOT NULL,
 message text NOT NULL,detail jsonb NOT NULL DEFAULT '{}',created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,task_id) REFERENCES tasks(tenant_id,id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS orders (
 id text PRIMARY KEY,tenant_id bigint NOT NULL,member_id bigint NOT NULL,sku text NOT NULL,
 points bigint NOT NULL CHECK(points>0),amount_cents bigint NOT NULL CHECK(amount_cents>=0),status text NOT NULL DEFAULT 'pending',
 created_at timestamptz NOT NULL DEFAULT now(),paid_at timestamptz,
 FOREIGN KEY(tenant_id,member_id) REFERENCES members(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS ledger (
 id bigserial PRIMARY KEY,tenant_id bigint NOT NULL,member_id bigint NOT NULL,kind text NOT NULL,
 delta bigint NOT NULL,balance_after bigint NOT NULL,reference text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,member_id,kind,reference),FOREIGN KEY(tenant_id,member_id) REFERENCES members(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS usage_logs (
 id bigserial PRIMARY KEY,tenant_id bigint NOT NULL,member_id bigint NOT NULL,task_id text NOT NULL DEFAULT '',
 operation text NOT NULL,model text NOT NULL, input_tokens integer NOT NULL DEFAULT 0,output_tokens integer NOT NULL DEFAULT 0,
 latency_ms integer NOT NULL,success boolean NOT NULL,detail text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,member_id) REFERENCES members(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS audit_logs (
 id bigserial PRIMARY KEY,tenant_id bigint NOT NULL,member_id bigint NOT NULL,action text NOT NULL,
 target text NOT NULL,detail jsonb NOT NULL DEFAULT '{}',created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,member_id) REFERENCES members(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS eval_runs (
 id text PRIMARY KEY,tenant_id bigint NOT NULL,member_id bigint NOT NULL,config jsonb NOT NULL,
 metrics jsonb NOT NULL DEFAULT '{}',results jsonb NOT NULL DEFAULT '[]',status text NOT NULL DEFAULT 'pending',
 error text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,member_id) REFERENCES members(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS artifact_revisions (
 tenant_id bigint NOT NULL,task_id text NOT NULL,revision integer NOT NULL,
 result jsonb NOT NULL,pptx_path text NOT NULL,mp4_path text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,task_id,revision),FOREIGN KEY(tenant_id,task_id) REFERENCES tasks(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS courseware_feedback (
 tenant_id bigint NOT NULL,task_id text NOT NULL,slide_id text NOT NULL,content_hash text NOT NULL,
 vote integer NOT NULL DEFAULT 0 CHECK(vote IN (-1,0,1)),reason text NOT NULL DEFAULT '',
 approved boolean NOT NULL DEFAULT false,updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,task_id,slide_id,content_hash),
 FOREIGN KEY(tenant_id,task_id) REFERENCES tasks(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS courseware_messages (
 id text PRIMARY KEY,tenant_id bigint NOT NULL,task_id text NOT NULL,request_key text NOT NULL,
 revision integer NOT NULL,slide_id text NOT NULL DEFAULT '',mode text NOT NULL,
 content text NOT NULL,reply text NOT NULL DEFAULT '',status text NOT NULL DEFAULT 'pending',
 error text NOT NULL DEFAULT '',child_id text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,task_id,request_key),FOREIGN KEY(tenant_id,task_id) REFERENCES tasks(tenant_id,id)
);
ALTER TABLE courseware_messages ADD COLUMN IF NOT EXISTS vote integer NOT NULL DEFAULT 0 CHECK(vote IN (-1,0,1));
ALTER TABLE courseware_messages ADD COLUMN IF NOT EXISTS reason text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS embedding_cache (
 tenant_id bigint NOT NULL REFERENCES tenants,provider text NOT NULL,model text NOT NULL,content_hash text NOT NULL,
 embedding vector(1536) NOT NULL,PRIMARY KEY(tenant_id,provider,model,content_hash)
);
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['members','documents','chunks','tasks','task_events','orders','ledger','usage_logs','audit_logs','eval_runs','artifact_revisions','embedding_cache','courseware_feedback','courseware_messages'] LOOP
   EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',t);
   EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',t);
   IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE tablename=t AND policyname='tenant_isolation') THEN
     EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = nullif(current_setting(''app.tenant_id'',true),'''')::bigint) WITH CHECK (tenant_id = nullif(current_setting(''app.tenant_id'',true),'''')::bigint)',t);
   END IF;
 END LOOP;
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='edu_runtime') THEN
   CREATE ROLE edu_runtime NOLOGIN NOSUPERUSER NOBYPASSRLS;
 END IF;
END $$;
GRANT USAGE ON SCHEMA public TO edu_runtime;
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO edu_runtime;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO edu_runtime;

-- Only authentication needs to discover memberships before a tenant is selected.
CREATE OR REPLACE FUNCTION auth_memberships(uid bigint)
RETURNS TABLE(id bigint,name text,role text)
LANGUAGE sql SECURITY DEFINER SET search_path=public,pg_temp AS $$
 SELECT t.id,t.name,m.role FROM public.tenants t JOIN public.members m ON m.tenant_id=t.id
 WHERE m.user_id=uid AND m.active ORDER BY t.id
$$;
REVOKE ALL ON FUNCTION auth_memberships(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_memberships(bigint) TO edu_runtime;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='edu_app') THEN
  CREATE ROLE edu_app LOGIN PASSWORD 'edu-runtime-local-only' NOSUPERUSER NOBYPASSRLS;
 END IF;
END $$;
GRANT edu_runtime TO edu_app;
