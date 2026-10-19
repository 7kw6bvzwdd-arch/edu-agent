CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS users (
 id bigserial PRIMARY KEY, username text NOT NULL UNIQUE, password_hash text NOT NULL,
 display_name text NOT NULL, active boolean NOT NULL DEFAULT true
);
ALTER TABLE users ADD COLUMN IF NOT EXISTS recovery_hash text NOT NULL DEFAULT '';
-- Shared, bounded authentication throttling; no raw identifiers or credentials.
CREATE TABLE IF NOT EXISTS auth_attempts (
 key text PRIMARY KEY, attempts integer NOT NULL, expires_at timestamptz NOT NULL
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
ALTER TABLE documents ADD COLUMN IF NOT EXISTS curriculum jsonb NOT NULL DEFAULT '{}';
CREATE TABLE IF NOT EXISTS media_assets (
 id text PRIMARY KEY, tenant_id bigint NOT NULL, member_id bigint NOT NULL,
 metadata jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,member_id) REFERENCES members(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS courseware_references (
 id text PRIMARY KEY, tenant_id bigint NOT NULL, member_id bigint NOT NULL,
 name text NOT NULL, profile jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(tenant_id,member_id) REFERENCES members(tenant_id,id)
);
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
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS restored_at timestamptz;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS cleanup_pending boolean NOT NULL DEFAULT false;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS charge_cycle integer NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS capacity_hold boolean NOT NULL DEFAULT false;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS next_attempt_at timestamptz;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS automatic_failures integer NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS courseware_provider_calls integer NOT NULL DEFAULT 0;
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
CREATE INDEX IF NOT EXISTS usage_member_time ON usage_logs(tenant_id,member_id,created_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS usage_totals (
 tenant_id bigint NOT NULL,member_id bigint NOT NULL,period text NOT NULL,
 calls bigint NOT NULL DEFAULT 0,input_tokens bigint NOT NULL DEFAULT 0,output_tokens bigint NOT NULL DEFAULT 0,
 failures bigint NOT NULL DEFAULT 0,latency_ms bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(tenant_id,member_id,period),FOREIGN KEY(tenant_id,member_id) REFERENCES members(tenant_id,id)
);
-- Backfill once before installing the insert trigger. Detail deletion never changes totals.
INSERT INTO usage_totals(tenant_id,member_id,period,calls,input_tokens,output_tokens,failures,latency_ms)
 SELECT tenant_id,member_id,'all',count(*),sum(input_tokens),sum(output_tokens),count(*) FILTER(WHERE NOT success),sum(latency_ms)
 FROM usage_logs GROUP BY tenant_id,member_id ON CONFLICT DO NOTHING;
INSERT INTO usage_totals(tenant_id,member_id,period,calls,input_tokens,output_tokens,failures,latency_ms)
 SELECT tenant_id,member_id,'d:'||to_char(created_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD'),count(*),sum(input_tokens),sum(output_tokens),count(*) FILTER(WHERE NOT success),sum(latency_ms)
 FROM usage_logs WHERE created_at>=now()-interval '89 days' GROUP BY tenant_id,member_id,3 ON CONFLICT DO NOTHING;
INSERT INTO usage_totals(tenant_id,member_id,period,calls,input_tokens,output_tokens,failures,latency_ms)
 SELECT tenant_id,member_id,'m:'||to_char(created_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM'),count(*),sum(input_tokens),sum(output_tokens),count(*) FILTER(WHERE NOT success),sum(latency_ms)
 FROM usage_logs WHERE created_at>=date_trunc('month',now())-interval '23 months' GROUP BY tenant_id,member_id,3 ON CONFLICT DO NOTHING;
CREATE OR REPLACE FUNCTION record_usage_totals() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p text;
BEGIN
 FOREACH p IN ARRAY ARRAY['all','d:'||to_char(NEW.created_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD'),'m:'||to_char(NEW.created_at AT TIME ZONE 'Asia/Shanghai','YYYY-MM')] LOOP
  INSERT INTO usage_totals(tenant_id,member_id,period,calls,input_tokens,output_tokens,failures,latency_ms)
  VALUES(NEW.tenant_id,NEW.member_id,p,1,NEW.input_tokens,NEW.output_tokens,CASE WHEN NEW.success THEN 0 ELSE 1 END,NEW.latency_ms)
  ON CONFLICT(tenant_id,member_id,period) DO UPDATE SET calls=usage_totals.calls+1,input_tokens=usage_totals.input_tokens+NEW.input_tokens,
  output_tokens=usage_totals.output_tokens+NEW.output_tokens,failures=usage_totals.failures+CASE WHEN NEW.success THEN 0 ELSE 1 END,latency_ms=usage_totals.latency_ms+NEW.latency_ms;
 END LOOP;
 DELETE FROM usage_totals WHERE tenant_id=NEW.tenant_id AND member_id=NEW.member_id AND
 ((period LIKE 'd:%' AND period<'d:'||to_char((now() AT TIME ZONE 'Asia/Shanghai')::date-89,'YYYY-MM-DD')) OR
 (period LIKE 'm:%' AND period<'m:'||to_char(date_trunc('month',now() AT TIME ZONE 'Asia/Shanghai')-interval '23 months','YYYY-MM')));
 DELETE FROM usage_logs WHERE tenant_id=NEW.tenant_id AND member_id=NEW.member_id AND
 (created_at<now()-interval '90 days' OR id IN (SELECT id FROM usage_logs WHERE tenant_id=NEW.tenant_id AND member_id=NEW.member_id ORDER BY created_at DESC,id DESC OFFSET 5000));
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS usage_totals_insert ON usage_logs;
CREATE TRIGGER usage_totals_insert AFTER INSERT ON usage_logs FOR EACH ROW EXECUTE FUNCTION record_usage_totals();
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
 FOREACH t IN ARRAY ARRAY['media_assets','courseware_references','members','documents','chunks','tasks','task_events','orders','ledger','usage_logs','usage_totals','audit_logs','eval_runs','artifact_revisions','embedding_cache','courseware_feedback','courseware_messages'] LOOP
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
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
 SELECT t.id,t.name,m.role FROM public.tenants t JOIN public.members m ON m.tenant_id=t.id
 WHERE m.user_id=uid AND m.active ORDER BY t.id
$$;
REVOKE ALL ON FUNCTION auth_memberships(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_memberships(bigint) TO edu_runtime;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='edu_app') THEN
  CREATE ROLE edu_app LOGIN NOSUPERUSER NOBYPASSRLS;
 END IF;
END $$;
GRANT edu_runtime TO edu_app;
REVOKE CREATE ON SCHEMA public FROM PUBLIC,edu_runtime,edu_app;

ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_charge_state_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_charge_state_check CHECK(charge_state IN ('unreserved','reserved','settled','released'));
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS animation jsonb NOT NULL DEFAULT '{}';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS html_path text NOT NULL DEFAULT '';
ALTER TABLE artifact_revisions ADD COLUMN IF NOT EXISTS html_path text NOT NULL DEFAULT '';

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS animation_export jsonb NOT NULL DEFAULT '{}';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS animation_export_lease_until timestamptz;

-- Credential operations run under a fixed owner/search_path. Public profile
-- columns remain joinable by tenant-scoped business queries.
REVOKE ALL ON users,sessions FROM PUBLIC,edu_runtime,edu_app;
GRANT SELECT(id,username,display_name,active) ON users TO edu_runtime;
GRANT UPDATE(display_name) ON users TO edu_runtime;
DROP FUNCTION IF EXISTS auth_credentials(text,bigint,boolean);
DROP FUNCTION IF EXISTS auth_set_password(bigint,text,text);
DROP FUNCTION IF EXISTS auth_set_recovery(bigint,text);
DROP FUNCTION IF EXISTS auth_session_create(text,bigint,timestamptz);
-- Proof-bearing operations never return hashes to the business runtime.
CREATE OR REPLACE FUNCTION auth_login(account text,password text,sid text,expires timestamptz)
RETURNS bigint LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE u public.users%ROWTYPE; candidate text; verified boolean; BEGIN
 SELECT * INTO u FROM public.users WHERE username=account FOR SHARE;
 candidate:=COALESCE(u.password_hash,'$2a$10$7EqJtq98hPqEX7fNZaFWoO5uHNwyP1g3qJWt/2y3N7sG6uEp9XKgG');
 verified:=public.crypt(password,candidate)=candidate;
 IF u.id IS NULL OR NOT u.active OR verified IS DISTINCT FROM true THEN RETURN NULL; END IF;
 INSERT INTO public.sessions VALUES(sid,u.id,LEAST(expires,now()+interval '12 hours'));
 RETURN u.id; END $$;
CREATE OR REPLACE FUNCTION auth_change_password(uid bigint,current_password text,new_hash text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE u public.users%ROWTYPE; BEGIN
 SELECT * INTO u FROM public.users WHERE id=uid AND active FOR UPDATE;
 IF u.id IS NULL OR public.crypt(current_password,u.password_hash) IS DISTINCT FROM u.password_hash THEN RETURN false; END IF;
 UPDATE public.users SET password_hash=new_hash,recovery_hash='' WHERE id=uid;
 DELETE FROM public.sessions WHERE user_id=uid;
 RETURN true; END $$;
CREATE OR REPLACE FUNCTION auth_reset_password(account text,proof text,new_hash text,recovery text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE u public.users%ROWTYPE; BEGIN
 SELECT * INTO u FROM public.users WHERE username=account AND active FOR UPDATE;
 IF u.id IS NULL OR u.recovery_hash='' OR u.recovery_hash IS DISTINCT FROM proof THEN RETURN false; END IF;
 UPDATE public.users SET password_hash=new_hash,recovery_hash=recovery WHERE id=u.id;
 DELETE FROM public.sessions WHERE user_id=u.id;
 RETURN true; END $$;
CREATE OR REPLACE FUNCTION auth_issue_recovery(uid bigint,password text,recovery text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE u public.users%ROWTYPE; BEGIN
 SELECT * INTO u FROM public.users WHERE id=uid AND active FOR UPDATE;
 IF u.id IS NULL OR public.crypt(password,u.password_hash) IS DISTINCT FROM u.password_hash THEN RETURN false; END IF;
 UPDATE public.users SET recovery_hash=recovery WHERE id=uid;
 RETURN true; END $$;
CREATE OR REPLACE FUNCTION auth_create_user(account text,hash text,label text,recovery text)
RETURNS bigint LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
 INSERT INTO public.users(username,password_hash,display_name,recovery_hash) VALUES(account,hash,label,recovery) ON CONFLICT DO NOTHING RETURNING id
$$;
CREATE OR REPLACE FUNCTION auth_session_active(sid text,uid bigint)
RETURNS boolean LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,public AS $$ SELECT EXISTS(SELECT 1 FROM public.sessions s JOIN public.users u ON u.id=s.user_id WHERE s.id=sid AND s.user_id=uid AND s.expires_at>now() AND u.active) $$;
CREATE OR REPLACE FUNCTION auth_session_revoke(sid text)
RETURNS void LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,public AS $$ DELETE FROM public.sessions WHERE id=sid $$;
CREATE OR REPLACE FUNCTION auth_session_refresh(sid text,uid bigint,expires timestamptz)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$ BEGIN
 PERFORM 1 FROM public.users WHERE id=uid AND active FOR SHARE;
 IF NOT FOUND THEN RETURN false; END IF;
 UPDATE public.sessions SET expires_at=GREATEST(expires_at,LEAST(expires,now()+interval '12 hours')) WHERE id=sid AND user_id=uid AND expires_at>now();
 RETURN FOUND;
END $$;
DO $$ DECLARE f text; BEGIN
 FOREACH f IN ARRAY ARRAY['auth_login(text,text,text,timestamp with time zone)','auth_create_user(text,text,text,text)','auth_change_password(bigint,text,text)','auth_reset_password(text,text,text,text)','auth_issue_recovery(bigint,text,text)','auth_session_active(text,bigint)','auth_session_revoke(text)','auth_session_refresh(text,bigint,timestamp with time zone)'] LOOP
 EXECUTE 'REVOKE ALL ON FUNCTION '||f||' FROM PUBLIC';
 EXECUTE 'GRANT EXECUTE ON FUNCTION '||f||' TO edu_runtime';
 END LOOP;
END $$;
