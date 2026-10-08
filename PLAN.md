# 教育智能体 — 实施计划书 v2（可直接开工）

> **本文件是执行契约。** 拿到本文档的模型应按 §13 的阶段顺序直接开工。
> 遇到未覆盖的细节，按 §14「决策记录」自行决定并追加一条记录，不要停下来提问。
>
> **修订说明**：v1 的定位是「现场演示系统」，方向错误——它为了保现场不翻车而砍掉了多租户、隐藏了失败、跳过了工程化。
> v2 的定位是「**可自建、可解释的完整产品原型**」，用途是 **AI 应用 / 大模型应用工程师的求职作品**。
> 这一定位差异决定了全部取舍，详见 §1 和 §2。
>
> 上游文档：`REQUIREMENTS-v1.md`（需求推导过程）。冲突时以本文件为准。

---

## 1. 定位与成功标准

### 1.1 这是什么

一个**面向 K12 教研场景的 AI 内容生产平台**：多租户、三个垂类智能体（课件研究 / 动画制作 / 视频生成）、自建 RAG 知识库、完整计费闭环。

**用途**：求职作品集。不对外销售，但要求**真的能用、真的能讲**。

### 1.2 三条成功标准（同时成立）

| 标准 | 含义 | 反例（v1 的错误） |
|---|---|---|
| **可自建** | 任何人按 README 在自己机器上一键跑起来 | 只有作者本机能跑 |
| **可解释** | 每个技术决策都有理由和代价，能在面试里讲 30 分钟 | 用一堆库但说不清为什么 |
| **可演示** | 端到端跑通，三个产物齐全，3 分钟内出结果 | 只有后端接口，没有可见成果 |

### 1.3 岗位对标：AI 应用 / 大模型应用工程师

这个岗位真正考察的是下面五件事，**项目必须在这五点上留下证据**：

1. **RAG 质量** —— 不是"接了向量库"，而是知道切片/检索/rerank 为什么这么选
2. **评测体系（Evals）** —— ★ 最能区分候选人。多数人的 AI 项目没有评测
3. **失败处理与降级** —— 模型一定会失败，系统不能崩，而且失败要可见
4. **成本与延迟控制** —— 有数据、有手段、有对比
5. **工程化** —— 测试、CI、可观测性、文档

> **CRUD、多租户、账单是"及格线"，不是"加分项"。**
> 它们必须做（照片里有，且能体现数据库设计能力），但时间预算要向上面五件事倾斜。

### 1.4 明确不是目标

不追求：高并发、分布式、SaaS 商业化、支付接入、内容审核合规、移动端。

---

## 2. 验收标准

### 2.1 功能验收

| # | 验收项 | 通过标准 |
|---|---|---|
| F1 | 多租户 | 租户 A 的用户登录后**看不到**租户 B 的任何数据（用户/知识库/任务/账单），且有自动化测试证明 |
| F2 | 租户切换 | 一个用户属于多个租户时，可切换，切换后所有数据随之变化 |
| F3 | 知识库 | 上传 PDF/DOCX/PPTX/TXT/MD，异步解析，状态可见，失败有错误信息 |
| F4 | 检索 | 混合检索（向量 + 关键词），支持 rerank 开关；返回结果**带来源引用** |
| F5 | 课件 | 一键生成教案（Markdown）+ PPT 大纲 + **真实可打开的 .pptx** |
| F6 | 动画 | 生成 Manim 动画 MP4，中文正常，可页面播放 |
| F7 | 视频 | 接第三方 AI 视频生成 API，产出 MP4，异步任务状态可见 |
| F8 | 自评 | 用**与生成不同**的模型打分，4 维度 + 扣分理由 |
| F9 | 计费 | 积分余额、充值（模拟支付）、订单记录、流水可追溯；余额不足阻止生成 |
| F10 | 仪表盘 | ECharts：用量趋势、模型成本分布、任务成功率、**降级率**、检索质量指标 |
| F11 | 评测 | 可一键跑 golden set，输出检索指标（Recall@K / MRR）与生成质量分，支持两次配置对比 |
| F12 | 权限 | 角色权限项可勾选（5 个权限点），改动即时生效 |

### 2.2 工程验收（求职项目的真正门槛）

| # | 验收项 | 通过标准 |
|---|---|---|
| E1 | 一键启动 | `docker compose up` + 三条命令，README 里的步骤**逐字执行**能跑起来 |
| E2 | 测试 | 核心逻辑有测试：租户隔离、积分扣减幂等、检索融合、降级触发。`go test ./...` 全绿 |
| E3 | CI | GitHub Actions：lint + test 自动跑，README 有徽章 |
| E4 | 文档 | README（含架构图 + 快速开始 + 截图）、`docs/adr/` 决策记录 |
| E5 | 可观测 | 每次 LLM 调用有结构化日志；仪表盘能看到降级率、失败率、P95 延迟 |
| E6 | 降级可见 | 任何降级/失败都在界面上**明确告知用户**，绝不静默吞掉 |

---

## 3. 系统架构

```
┌───────────────────────────────────────────────────────────────┐
│  前端  Vue 3 + Vite + Element Plus + Pinia + ECharts          │
│  登录/租户切换/工作台/知识库/任务/评测/用户权限/账单/积分/仪表盘 │
└────────────────────────┬──────────────────────────────────────┘
                         │ REST + SSE
┌────────────────────────▼──────────────────────────────────────┐
│  Go 主服务 (Gin)                                               │
│  ├─ 认证鉴权    JWT + RBAC + 租户上下文中间件                   │
│  ├─ 多租户      应用层强制 tenant_id + PostgreSQL RLS 兜底      │
│  ├─ 知识库      解析 / 切片 / 向量化 / 混合检索 / rerank         │
│  ├─ 编排层      ★ 任务状态机 / 并发 / 重试 / 三层降级            │
│  ├─ Provider   ★ 统一抽象：Chat / Embedding / Rerank / Video    │
│  ├─ 评测        golden set 执行 + 指标计算 + 配置对比            │
│  ├─ 计费        积分扣减（幂等）/ 充值 / 订单                    │
│  └─ 可观测      usage / audit / degradation 三类日志            │
└──────┬─────────────────────────────────┬──────────────────────┘
       │ SQL                             │ HTTP (内网)
┌──────▼───────────────┐   ┌─────────────▼──────────────────────┐
│ PostgreSQL 16        │   │ Python 渲染服务 (FastAPI)          │
│  + pgvector          │   │  ★ 无任何 LLM 调用，纯计算          │
│  + RLS 策略          │   │  · Manim 代码 → MP4                │
│  业务数据 + 向量      │   │  · python-pptx → .pptx             │
└──────────────────────┘   └────────────────────────────────────┘
```

### 3.1 分工铁律

> **Python 服务是"哑"的：不做决策、不调 LLM。**
> 输入成品（Manim 代码 / 大纲 JSON），执行，返回文件路径或 stderr。

因此 Manim 的**失败修复循环由 Go 负责**：收到 stderr → 回灌 LLM → 重新生成 → 再次调用。
好处：API key、prompt、重试、降级、积分扣减只有一份实现。

### 3.2 Provider 抽象层（★ 面试重点）

四个接口，每个至少一个实现，配置决定用哪个：

```go
type ChatProvider interface {
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
    ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error)
}

type EmbeddingProvider interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimension() int
}

type RerankProvider interface {
    Rerank(ctx context.Context, query string, docs []string, topN int) ([]RankedDoc, error)
}

type VideoProvider interface {
    Submit(ctx context.Context, req VideoRequest) (jobID string, err error)
    Poll(ctx context.Context, jobID string) (VideoStatus, error)  // pending|running|succeeded|failed
}
```

**为什么这样设计**（写进 ADR）：
- 生成模型、自评模型、embedding 模型、视频 provider 来自不同厂商，接口风格各异
- 换模型只改配置，不改业务代码
- 评测体系需要"同一 prompt 跑两个模型对比"——没有抽象层就做不到

---

## 4. 技术栈（锁定）

### 4.1 Go 主服务
```
Go 1.22+
gin-gonic/gin            Web
jackc/pgx/v5             PostgreSQL 驱动
pgvector/pgvector-go     向量类型
golang-jwt/jwt/v5        认证
golang.org/x/crypto       bcrypt
golang.org/x/sync         errgroup（并发编排）
spf13/viper 或 caarlos0/env   配置
gocarina/gocsv（可选）    导出评测结果 CSV
```

### 4.2 Python 渲染服务
```
Python 3.11
fastapi, uvicorn[standard]
manim==0.18.*
python-pptx==1.0.*
```

**系统依赖（缺一个 Manim 就跑不起来）：**
```
ffmpeg
libcairo2  libpango-1.0-0  libpangocairo-1.0-0  pkg-config  libgl1
texlive-latex-base  texlive-latex-extra  texlive-fonts-recommended  dvisvgm
fonts-noto-cjk                    ★ 中文字体，不装动画里的中文全是方块
```
**封装成 Docker 镜像**（`render-service-py/Dockerfile`），不要污染宿主机。

### 4.3 前端
```
Node 20+ / Vue 3.4 / Vite 5 / Element Plus 2.x
Pinia / Vue Router 4 / axios
echarts + vue-echarts             仪表盘
marked + highlight.js             渲染教案 Markdown
```

### 4.4 基础设施
```
PostgreSQL 16 + pgvector 0.7      docker-compose
Docker + docker-compose
GitHub Actions                    CI
```

**不引入 Redis。** 理由见 §14 ADR-011——当前没有需要 Redis 的场景，
用 PG 表做 embedding 去重缓存已经足够。为了架构图上多一个框而引入组件，是负债。

---

## 5. 目录结构

```
edu-agent/
├── README.md                    ★ 面试官先读这个
├── PLAN.md                      本文件
├── REQUIREMENTS-v1.md
├── docker-compose.yml
├── .env.example
├── .github/workflows/ci.yml
├── docs/
│   ├── architecture.md          架构说明 + Mermaid 图
│   ├── eval-report.md           ★ 评测结果（RAG 对比、模型对比）
│   └── adr/                     ★ 决策记录，每条一个文件
│       ├── 001-why-self-built-rag.md
│       ├── 002-multi-tenant-isolation.md
│       ├── 003-chunking-strategy.md
│       ├── 004-hybrid-retrieval.md
│       ├── 005-three-tier-degradation.md
│       └── ...
├── backend-go/
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── config/  db/  migrations/
│   │   ├── tenant/              ★ 租户上下文 + 隔离
│   │   ├── auth/                JWT + RBAC
│   │   ├── models/  handlers/
│   │   ├── provider/            ★ chat / embedding / rerank / video
│   │   ├── rag/                 chunker / hybrid / fusion / rerank
│   │   ├── eval/                ★ golden set 执行 + 指标
│   │   ├── orchestrator/        ★ 任务状态机 / 并发 / 降级
│   │   ├── courseware/  animation/  video/
│   │   ├── render/              调 Python
│   │   ├── billing/             积分 / 订单（幂等）
│   │   ├── obs/                 usage / audit / degradation 日志
│   │   └── prompts/
│   └── go.mod
├── render-service-py/
│   ├── main.py
│   ├── renderers/manim_runner.py
│   ├── renderers/pptx_builder.py
│   ├── templates/lesson.pptx    PPTX 母版（人工制作一次）
│   ├── requirements.txt
│   └── Dockerfile
├── evals/                       ★ 评测数据集与脚本
│   ├── golden_retrieval.jsonl   检索评测：query + 期望命中文档
│   ├── golden_generation.jsonl  生成评测：课题 + 评分要点
│   └── run_eval.sh
└── frontend-vue/
```

---

## 6. 数据模型

> 全部业务表带 `tenant_id`。**这是硬约束，任何新表都必须带。**

```sql
CREATE EXTENSION IF NOT EXISTS vector;

-- ============ 租户 ============
CREATE TABLE tenants (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL UNIQUE,
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE roles (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id),   -- 角色属于租户，可自定义
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    permissions JSONB NOT NULL DEFAULT '[]'::jsonb,
    is_system   BOOLEAN NOT NULL DEFAULT false,           -- 系统内置角色不可删
    UNIQUE (tenant_id, name)
);

CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,      -- 全局唯一，登录用
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 用户 ↔ 租户 多对多，角色在租户上下文内生效
CREATE TABLE tenant_members (
    id         BIGSERIAL PRIMARY KEY,
    tenant_id  BIGINT NOT NULL REFERENCES tenants(id),
    user_id    BIGINT NOT NULL REFERENCES users(id),
    role_id    BIGINT NOT NULL REFERENCES roles(id),
    points_balance INTEGER NOT NULL DEFAULT 0,
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id)
);

-- ============ 知识库 ============
CREATE TABLE documents (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id),
    user_id     BIGINT NOT NULL REFERENCES users(id),
    filename    TEXT NOT NULL,
    mime        TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'pending',   -- pending|parsing|ready|failed
    error_msg   TEXT NOT NULL DEFAULT '',
    chunk_count INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON documents (tenant_id, created_at DESC);

CREATE TABLE chunks (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id),   -- ★ 冗余，为了检索时能直接过滤
    document_id BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    seq         INTEGER NOT NULL,
    content     TEXT NOT NULL,
    embedding   vector(1536),
    tsv         tsvector,                                  -- ★ 中文分词后写入，用于关键词检索
    meta        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON chunks USING hnsw (embedding vector_cosine_ops);
CREATE INDEX ON chunks (tenant_id);                        -- ★ 隔离与性能的关键索引
CREATE INDEX ON chunks USING gin (tsv);

-- embedding 去重缓存（省钱的直接手段）
CREATE TABLE embedding_cache (
    content_hash TEXT PRIMARY KEY,
    model        TEXT NOT NULL,
    embedding    vector(1536) NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============ 任务 ============
CREATE TABLE task_groups (
    id         TEXT PRIMARY KEY,               -- uuid，把同课题的三个智能体任务绑在一起
    tenant_id  BIGINT NOT NULL REFERENCES tenants(id),
    user_id    BIGINT NOT NULL REFERENCES users(id),
    topic      TEXT NOT NULL,
    params     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id),
    group_id    TEXT NOT NULL REFERENCES task_groups(id),
    type        TEXT NOT NULL,                 -- courseware | animation | video
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending|running|succeeded|degraded|failed
    stage       TEXT NOT NULL DEFAULT '',
    progress    INTEGER NOT NULL DEFAULT 0,
    error_msg   TEXT NOT NULL DEFAULT '',
    points_cost INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);
CREATE INDEX ON tasks (tenant_id, created_at DESC);

-- ============ 产物 ============
CREATE TABLE lesson_plans (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL, task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    content_md TEXT NOT NULL, model TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE slide_outlines (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL, task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    content_json JSONB NOT NULL, pptx_path TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE animations (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL, task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    script_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    manim_code TEXT NOT NULL DEFAULT '',
    mp4_path TEXT NOT NULL DEFAULT '',
    render_log TEXT NOT NULL DEFAULT '',
    retry_count INTEGER NOT NULL DEFAULT 0,
    -- ★ 降级层级最终停在哪一级：0=正常 1=重试成功 2=简化模式 3=失败
    degradation_level INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE videos (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL, task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    provider TEXT NOT NULL, provider_job_id TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL DEFAULT '', mp4_path TEXT NOT NULL DEFAULT '',
    poll_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE evaluations (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL, task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    target TEXT NOT NULL,               -- lesson_plan | outline | animation
    scores JSONB NOT NULL, reasons_md TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============ 计费 ============
CREATE TABLE point_transactions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
    delta INTEGER NOT NULL, reason TEXT NOT NULL,
    ref_type TEXT NOT NULL DEFAULT '', ref_id TEXT NOT NULL DEFAULT '',
    balance_after INTEGER NOT NULL,
    idempotency_key TEXT,                -- ★ 防重复扣减
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, idempotency_key)
);

CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
    order_no TEXT NOT NULL UNIQUE,
    points INTEGER NOT NULL, amount_cents INTEGER NOT NULL,   -- 模拟支付，不接真实渠道
    status TEXT NOT NULL DEFAULT 'pending',                   -- pending|paid|cancelled
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============ 评测 ============
CREATE TABLE eval_runs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    name TEXT NOT NULL,                 -- 例："hybrid+rerank vs vector-only"
    config JSONB NOT NULL,              -- 本次使用的切片/检索/prompt 配置快照
    dataset TEXT NOT NULL,              -- retrieval | generation
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'running',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE eval_results (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
    case_id TEXT NOT NULL, query TEXT NOT NULL,
    retrieved JSONB NOT NULL DEFAULT '[]'::jsonb,   -- 命中的 chunk id 列表（按序）
    score REAL NOT NULL DEFAULT 0,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============ 可观测 ============
CREATE TABLE usage_logs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL, task_id BIGINT,
    stage TEXT NOT NULL, provider TEXT NOT NULL, model TEXT NOT NULL,
    prompt_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0, cost_points INTEGER NOT NULL DEFAULT 0,
    cached BOOLEAN NOT NULL DEFAULT false,          -- ★ 是否命中缓存
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON usage_logs (tenant_id, created_at DESC);

CREATE TABLE degradation_events (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL, task_id BIGINT NOT NULL,
    from_level INTEGER NOT NULL, to_level INTEGER NOT NULL,
    reason TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
    action TEXT NOT NULL, target_type TEXT NOT NULL DEFAULT '', target_id TEXT NOT NULL DEFAULT '',
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.1 多租户隔离（★ 面试重点，必须做扎实）

**三层防护：**

1. **应用层**：JWT 里带 `active_tenant_id`；中间件把它注入 `context`；
   所有 repository 方法的第一个参数是 `tenantID`，**SQL 里必须带 `WHERE tenant_id = $1`**。
   禁止任何不带 tenant_id 的业务查询。
2. **数据库层**：对核心表启用 **PostgreSQL Row Level Security**：
   ```sql
   ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
   CREATE POLICY tenant_isolation ON documents
     USING (tenant_id = current_setting('app.tenant_id', true)::BIGINT);
   ```
   每个请求开始时 `SET LOCAL app.tenant_id = '<id>'`。
   → 即使应用层漏写了过滤条件，数据库也不会泄漏数据。
3. **测试层**：
   ```
   TestTenantIsolation_Retrieval   建两个租户各插文档，用 A 的检索查询，断言 0 条来自 B
   TestTenantIsolation_Tasks       断言 A 查不到 B 的任务
   TestTenantIsolation_RLS         绕过应用层直接查，断言 RLS 仍然拦住
   ```
   **没有这三个测试，多租户等于没做。**

### 6.2 权限点（固定）
```
courseware.generate   生成课件
animation.render      渲染动画（含视频）
kb.manage             知识库管理
log.view              查看日志与仪表盘
user.manage           用户与权限管理
```

### 6.3 向量维度
`vector(1536)` 对应 `text-embedding-3-small`。
维度是代码常量 `const EmbeddingDim = 1536`。换模型且维度不同时必须重建 `chunks` 并重新摄入——**写进代码注释**。

---

## 7. RAG 设计（★ 面试重点）

### 7.1 为什么自建而不是用 WeKnora

原照片指定了 WeKnora（腾讯开源知识平台）。**改自建，理由记进 ADR-001：**

- 求职目标是 AI 应用工程师，**"我实现了完整的检索链路并能评测它"** 比 "我部署了一个开源平台" 有说服力得多
- 自建才能做 §7.2 的混合检索、§7.3 的 rerank 对比、§8 的评测体系
- 少一个独立服务 + 一套存储 + 一套权限模型（多租户隔离会变成两处维护）

**代价**：文档解析和切片要自己写。这是本项目最花时间但也最值得的部分。

### 7.2 检索链路

```
query
  │
  ├─► [A] 向量检索     pgvector, cosine, WHERE tenant_id = ?, top 20
  │
  ├─► [B] 关键词检索   chunks.tsv @@ to_tsquery(...), top 20
  │                    ★ 中文分词：应用层用 jieba/gse 分词后写入 tsv 字段
  │                    （理由见 ADR-004：不装 zhparser 扩展，部署更简单且可控）
  │
  ├─► [C] RRF 融合     score = Σ 1/(k + rank_i)，k=60   → top 20
  │
  ├─► [D] Rerank       RerankProvider.Rerank(query, docs, topN=8)   ← 可开关
  │
  └─► [E] 返回 top 8，每条带 {document_id, filename, seq, content, score}
```

**每一层都可配置开关**，因为 §8 的评测要对比"向量only / +关键词 / +rerank"三种配置的效果差异。
这张对比表就是 `docs/eval-report.md` 的核心内容。

### 7.3 切片策略

默认：**固定 500 字 + 50 字重叠**，按段落边界优先断开。

切片策略做成可配置（`CHUNK_SIZE` / `CHUNK_OVERLAP` / `CHUNK_MODE`），
因为评测阶段要对比 `256 / 512 / 1024` 三种尺寸的 Recall@5 差异。

### 7.4 引用溯源（防幻觉的基本手段）

- 检索片段注入 prompt 时，每条前面加 `[来源: 文件名#序号]`
- prompt 中硬性要求"引用时必须标注来源，资料中没有的内容不得编造"
- 生成结果**保存所用的检索片段快照**（`lesson_plans` 关联 `retrieval_json`），
  界面上可展开查看"这段内容依据了哪些原文"
- 这是面试里"你怎么控制幻觉"的标准答案素材

---

## 8. 评测体系（★★ 全项目最能加分的一章）

> 大多数 AI 项目止步于"能跑"。能说清"我怎么知道它变好了"的人极少。
> 这一章的工作量必须保证，不能因为是"非功能"就砍。

### 8.1 数据集

`evals/golden_retrieval.jsonl`（30–50 条，人工标注）
```json
{"id":"r001","query":"初中物理 浮力 阿基米德原理","expected_doc":"人教版八年级物理下册.pdf","expected_chunk_keywords":["阿基米德","排开液体","重力"]}
```

`evals/golden_generation.jsonl`（15–20 条）
```json
{"id":"g001","topic":"初中物理 浮力","grade":"八年级","must_cover":["阿基米德原理","浮沉条件"],"must_not":["超纲的微积分推导"]}
```

数据集需要人工整理，**这是必要投入**。用演示用的教材和课标来构建。

### 8.2 指标

**检索类：**
```
Recall@K     K = 1, 3, 5, 8
MRR          第一个正确结果的排名倒数
HitRate       是否至少命中一条
```

**生成类（LLM-as-judge，用与生成不同的模型）：**
```
课标符合度 / 内容准确性 / 结构完整性 / 可授课性   各 1-5 分
must_cover 覆盖率    期望要点出现了几个
must_not 违规率      不该出现的内容出现了几个
引用准确率           标注的来源是否真的支持该结论
```

**运行类：**
```
端到端 P50 / P95 延迟
单任务平均 token 成本
降级率（degradation_level > 0 的比例）
失败率
```

### 8.3 执行与对比

```
POST /api/eval/runs        {name, dataset, config}   -> 创建并异步执行
GET  /api/eval/runs        列表，含指标
GET  /api/eval/runs/:id    明细：逐条 case 的命中情况
GET  /api/eval/runs/compare?a=&b=   两次运行的指标对比
```

`evals/run_eval.sh` 一条命令跑完整评测，输出 CSV + Markdown 报告。

### 8.4 必须产出的对比表（写进 `docs/eval-report.md`）

| 实验 | 变量 | 要回答的问题 |
|---|---|---|
| 检索配置对比 | 向量only / +关键词 / +rerank | 混合检索和 rerank 到底带来多少提升？ |
| 切片尺寸对比 | 256 / 512 / 1024 | 切片多大最合适？ |
| 模型对比 | 两个生成模型 × 同一 golden set | 贵模型贵得值吗？ |
| 自评模型对比 | 换一个 judge 模型 | 评分稳定吗？ |

> **这四张表是这个项目区别于"又一个 AI demo"的全部理由。** 面试时可以直接打开讲。

### 8.5 评测的定位

评测**不是一次性任务**，而是开发流程的一部分：
每次修改 prompt、切片策略、检索参数后，跑一遍 golden set，
指标下降就不能合并。CI 里可以只跑一个小规模的 smoke 子集（5 条）。

---

## 9. 智能体规格

### 9.1 阶段枚举（前后端共用，禁止自创）
```
pending
retrieving            检索知识库
generating_lesson     生成教案
generating_outline    生成 PPT 大纲
building_pptx         生成 PPTX
generating_script     生成动画分镜
generating_manim      生成 Manim 代码
rendering_animation   渲染动画
submitting_video      提交视频生成任务
polling_video         等待视频生成
evaluating            自评打分
succeeded / degraded / failed
```

### 9.2 课件任务（type = courseware）
```
1. retrieving          pgvector + 关键词 + RRF 融合 + rerank → top 8
2. generating_lesson   ┐ 并发发起（errgroup）
3. generating_outline  ┘
4. building_pptx       调 Python 服务：大纲 JSON + 母版 → .pptx
5. evaluating          用「自评模型」打分
```
**2 和 3 必须并发**，这是把端到端压进 3 分钟的关键。

### 9.3 动画任务（type = animation）—— 三层降级（★ 面试重点）

> **v1 在这里写的是"失败时静默回退到硬编码动画"。那是错的**：
> 面试官问"渲染失败了怎么办"，答案不能是"我藏起来了"。
> 正确的是**显式降级 + 全程可观测**。

```
L0  正常路径
    generating_script → generating_manim → 渲染 → 成功

L1  修复重试（最多 3 次）
    把 stderr 回灌 LLM → 重新生成代码 → 再渲染
    记录：retry_count, degradation_events(from=0, to=1)

L2  简化模式 ★ 这是真正的工程手段，不是作弊
    放弃 LaTeX 与复杂几何：重新生成一个"只用 Text + 简单形状 + 淡入淡出"的版本
    这类代码成功率高得多，代价是画面朴素
    → 渲染成功则 task.status = 'degraded'，界面明确提示
      "动画已生成（简化模式）：复杂公式渲染失败，已自动降级。可点击重试完整版本"
    记录：degradation_level = 2

L3  失败
    task.status = 'failed'，界面显示失败原因 + 保留已生成的分镜脚本 + "重试"按钮
    绝不白屏，绝不静默
```

**每一次降级都写入 `degradation_events`，仪表盘展示降级率。**
面试话术：*"我的动画链路有三层降级，L2 是自动简化重生成。线上降级率 12%，其中 9% 停在 L2，用户可见且可重试。"*

Manim 代码生成的 prompt 约束见 §10.4——**约束写不严，L1 会反复失败**。

### 9.4 视频任务（type = video）

```
1. retrieving           复用同组检索结果
2. generating_script    生成视频脚本（分镜 + 每段画面描述 + 时长）
3. submitting_video     调 VideoProvider.Submit() → jobID
4. polling_video        轮询，间隔 5s，最长 10 分钟
                        每轮更新 stage/progress，SSE 推送
5. evaluating           打分
```

**Provider 实现**：优先接一个有公开 API 的视频生成服务（Runway / 可灵 / 即梦等，
以你手上有 key 的为准）。实现时**以该 provider 最新官方文档为准**，不要凭记忆写参数。
接口按 §3.2 的 `VideoProvider` 抽象，换 provider 只加一个 adapter。

**必须有超时和失败处理**：轮询超时 → 标记 failed + 明确原因 + 可重试。

### 9.5 模型配置

```env
LLM_BASE_URL             OpenAI 兼容端点
LLM_API_KEY
LLM_GENERATION_MODEL     生成用（强模型）
LLM_CHEAP_MODEL          ★ 便宜模型：分镜、标题、简单抽取
LLM_EVAL_MODEL           ★ 自评用，必须与生成模型不同
LLM_EMBEDDING_MODEL      text-embedding-3-small
RERANK_ENABLED           true|false
VIDEO_PROVIDER           runway|kling|...
VIDEO_API_KEY
```

> **`LLM_EVAL_MODEL` ≠ `LLM_GENERATION_MODEL` 是硬要求。**
> 同模型给自己打分没有验证价值，演示时也经不起追问。
> **模型分级路由**（`LLM_CHEAP_MODEL`）是成本优化的直接手段，也写进评测对比。

---

## 10. Prompt 契约

模板集中在 `internal/prompts/`，Go 常量，**不散落在业务代码**。

### 10.1 教案生成
```
系统：你是资深{学段}{学科}教研员，擅长依据课程标准设计教案。
只输出 Markdown 正文，不要解释性前后缀，不要用代码块包裹。

用户：
课题：{topic}   学段：{grade}   学科：{subject}
教材版本：{textbook}   课时：{duration}

参考资料（必须优先依据，不得编造资料中没有的事实）：
<refs>
{检索片段，每条前缀 [来源: 文件名#序号]}
</refs>

输出结构：
# {课题}
## 一、教学目标   （知识与技能 / 过程与方法 / 情感态度价值观）
## 二、教学重难点
## 三、教学过程   （分环节，每环节写：教师活动 / 学生活动 / 设计意图）
## 四、板书设计

要求：引用资料内容时用 [来源: 文件名] 标注；教学过程总字数不超过 1200 字。
```

### 10.2 PPT 大纲生成
```
系统：你是教学 PPT 设计专家。只输出 JSON，不要 Markdown 代码块，不要其他文字。

用户：
课题：{topic}  学段：{grade}  学科：{subject}
教案：
{教案 Markdown}

输出 JSON 数组，8–12 页：
[{"page":1,"title":"...","bullets":["...","..."],"image_hint":"建议配图说明"}]
要求：第 1 页封面；每页 bullets ≤ 5 条、每条 ≤ 30 字；最后一页课堂小结。
```

### 10.3 动画分镜生成
```
系统：你是教学动画导演。只输出 JSON，不要 Markdown 代码块，不要其他文字。

用户：
课题：{topic}  学段：{grade}  学科：{subject}
参考资料：{检索片段}

输出 JSON 数组，3–5 幕，总时长 40–70 秒：
[{"scene_no":1,"narration":"旁白（中文，一句话）","visual":"画面描述（中文）",
  "formula":"该幕要展示的公式（LaTeX，无则留空）","duration_sec":12}]
要求：必须包含公式推导、函数图像或几何演示中的至少一种。
```

### 10.4 Manim 代码生成（★ 最容易失败，约束必须写死）
```
系统：你是 Manim 专家。只输出一段完整可运行的 Python 代码，不要 Markdown 代码块，不要解释。

硬性约束：
- Manim Community Edition 0.18 的 API
- 必须 `from manim import *`
- 必须定义且只定义一个类：`class EduAnimation(Scene):`
- 所有中文必须写成 Text("中文", font="Noto Sans CJK SC")，否则显示方块
- 数学公式用 MathTex(r"...")，不要用 Tex
- 不要设置 config.pixel_width / frame_rate（由命令行决定）
- 不使用需要联网的素材、不使用 MovingCameraScene 等高级相机
- 每幕 self.play() + self.wait()，总时长约 {total_duration} 秒

分镜脚本：{script_json}
参考知识点：{检索片段}
```

### 10.5 L1 修复 prompt
```
系统：下面的 Manim 代码执行报错了。只输出修复后的完整 Python 代码，不要代码块，不要解释。

原始需求：{script_json}
报错的代码：{previous_code}
执行错误：{stderr 末尾 2000 字符}
```

### 10.6 L2 简化模式 prompt
```
系统：你是 Manim 专家。上次生成的代码因 LaTeX/复杂图形渲染失败。
现在请生成一个**简化版**：只使用 Text（中文字体 Noto Sans CJK SC）、Circle、Rectangle、
Line、Arrow、FadeIn、FadeOut、Write。禁止使用 MathTex、Tex、Transform、复杂路径动画。
只输出完整 Python 代码，不要代码块。类名仍为 EduAnimation。

要表达的知识点：{script_json 的旁白部分}
```

### 10.7 自评 prompt
```
系统：你是严格的教研评审专家。只输出 JSON，不要代码块，不要其他文字。

用户：课题：{topic}
待评内容：{产物}

按 4 维打分（1–5 整数）：课标符合度 / 内容准确性 / 结构完整性 / 可授课性
输出：
{"scores":{"课标符合度":4,"内容准确性":5,"结构完整性":4,"可授课性":3},
 "reasons":"针对每一维说明扣分理由，中文，200 字以内"}
```

---

## 11. API 规格

前缀 `/api`，`Authorization: Bearer <jwt>`。
统一响应：成功 `{"code":0,"data":...}`；失败 `{"code":<非0>,"message":"..."}`。

### 认证与租户
```
POST /api/auth/login            {username,password} -> {token, user, tenants:[{id,name,role}], active_tenant_id}
POST /api/auth/logout
GET  /api/auth/me
POST /api/auth/switch-tenant    {tenant_id} -> 新 token（active_tenant_id 变化）
```

### 知识库
```
GET    /api/documents
POST   /api/documents                 上传（multipart），异步入库，202
GET    /api/documents/:id
DELETE /api/documents/:id
POST   /api/retrieval/test            {query, top_k, use_hybrid, use_rerank}
                                      -> [{chunk_id,document_id,filename,content,score,source:"vector|keyword"}]
```

### 生成
```
POST /api/tasks               {types:["courseware","animation","video"], topic, params}
                              -> {group_id, task_ids:{}}
GET  /api/tasks               分页
GET  /api/tasks/:id
GET  /api/tasks/groups/:group_id/result
                              -> {lesson_plan, slide_outline, pptx_url, animation, video, evaluations, degradations}
GET  /api/tasks/groups/:group_id/stream     ★ SSE
GET  /api/tasks/:id/download/md
GET  /api/tasks/:id/download/pptx
GET  /api/tasks/:id/video                   MP4，支持 Range
POST /api/tasks/:id/retry                   重试失败的降级任务
```

### 评测
```
POST /api/eval/runs           {name, dataset, config}
GET  /api/eval/runs
GET  /api/eval/runs/:id
GET  /api/eval/runs/compare?a=&b=
GET  /api/eval/datasets       列出 golden set 与条目数
```

### 租户 / 用户 / 权限
```
GET   /api/tenants                    当前用户所属租户
POST  /api/tenants                    {name,slug}  创建租户（当前用户成为其管理员）
GET   /api/users
POST  /api/users
PATCH /api/users/:id                  {status?, role_id?, points_delta?}
GET   /api/roles
POST  /api/roles
PATCH /api/roles/:id                  {permissions:[...]}
```

### 计费
```
GET  /api/billing/balance
GET  /api/billing/transactions        流水（分页）
POST /api/billing/orders              {points} -> 创建订单
POST /api/billing/orders/:id/pay      模拟支付，幂等
GET  /api/billing/orders
```

### 可观测
```
GET /api/logs/usage          用量日志（分页，可按 task_id / stage 过滤）
GET /api/logs/audit
GET /api/logs/degradations   ★ 降级事件
GET /api/dashboard/summary   仪表盘聚合数据
GET /api/dashboard/trend?days=30
```

### SSE 事件格式
```json
{"task_id":12,"type":"animation","stage":"rendering_animation","progress":60,
 "message":"正在渲染动画…","level":0}
```
终止事件 `stage` 为 `succeeded` / `degraded` / `failed`。
**进度必须单调不减**，前端进度条不允许回退。

---

## 12. 前端页面

| 页面 | 路由 | 内容 |
|---|---|---|
| 登录 | `/login` | 用户名密码 |
| 租户选择 | `/tenants` | 多租户时展示，单租户直接跳过 |
| 工作台 | `/` | ★ 主战场。课题输入 + 学段/学科/教材 + 勾选要生成的产物（课件/动画/视频）+ 一键生成；实时 SSE 进度；完成后面板展示三个产物 + 自评分数 + **降级提示条** |
| 知识库 | `/knowledge` | 上传区 + 文档表格 + 检索测试面板（可切换混合检索/rerank，展示分数与来源） |
| 任务 | `/tasks` | 任务列表 + 详情（产物预览、下载、降级记录、重试） |
| 评测 | `/eval` | 运行列表 + 指标表 + **配置对比图** + 逐条 case 明细 |
| 用户权限 | `/users` | 用户表格 + 角色权限勾选面板 |
| 账单 | `/billing` | 订单列表 + 创建订单 + 模拟支付 |
| 积分 | `/points` | 余额 + 流水 |
| 仪表盘 | `/dashboard` | ECharts：用量趋势、模型成本分布、任务成功率、**降级率**、P95 延迟、检索质量趋势 |
| 日志 | `/logs` | 用量 / 审计 / 降级 三个 Tab |

### 12.1 前端要求

- **进度必须真实**，来自 SSE，不允许假定时器
- **降级必须可见**：`task.status = degraded` 时显示明确提示条，不是隐藏
- **失败必须可操作**：显示原因 + 重试按钮，不出现红色报错弹窗
- **权限即时生效**：无权限的按钮不渲染
- 产物的**检索依据可展开**：展示这次生成用了哪些原文片段
- 用 Element Plus，但**要定义主题变量**（至少在 `styles/` 里统一配色），不要完全默认样式

---

## 13. 实施阶段

> **每个里程碑结束都是一个可以开始投简历的状态。** 不要等到全部做完。

### M1 — 能生成课件（约 8 天）
| 阶段 | 内容 | 验收 |
|---|---|---|
| P0 骨架（1d） | 目录、docker-compose、三服务互 ping | 四组件互通 |
| P1 多租户+认证（2.5d） | 表结构、JWT、租户上下文、RBAC、RLS、**三个隔离测试** | F1、F2、F12、E2(部分) |
| P2 知识库+RAG基础（2d） | 上传、解析、切片、向量化、向量检索、检索测试页 | F3、F4(部分) |
| P3 课件+PPTX+自评（2.5d） | 编排、教案、大纲、pptx、自评、工作台、SSE | F5、F8 |

**M1 交付物**：能登录、能建知识库、能一键出教案+PPTX+评分。**可以开始投简历。**

### M2 — 三个智能体齐全（约 5 天）
| 阶段 | 内容 | 验收 |
|---|---|---|
| P4 动画+三层降级（3d） | 分镜、Manim 生成、渲染、L1 重试、L2 简化、L3 失败可见 | F6、E6 |
| P5 视频智能体（2d） | VideoProvider、提交、轮询、SSE、失败处理 | F7 |

**M2 交付物**：完整演示，三个产物齐全，降级可见。

### M3 — 差异化（约 4 天）★ 最重要
| 阶段 | 内容 | 验收 |
|---|---|---|
| P6 评测体系（2.5d） | golden set、指标计算、对比、评测页面、`eval-report.md` | F11 |
| P7 RAG 深化（1.5d） | 混合检索、RRF、rerank、引用溯源、切片策略可配 | F4 |

**M3 交付物**：四张对比表。**这是简历上最亮的一条。**

### M4 — 完整作品（约 6 天）
| 阶段 | 内容 | 验收 |
|---|---|---|
| P8 计费闭环（2d） | 积分、幂等扣减、订单、模拟支付、账单页 | F9 |
| P9 仪表盘+可观测（2d） | 三类日志、ECharts 仪表盘、降级率 | F10、E5 |
| P10 成本优化（1d） | embedding 缓存、模型分级路由、token 预算熔断 | — |
| P11 测试+CI+文档（2.5d） | 补测试、Actions、README、架构图、ADR | E1、E2、E3、E4 |
| P12 打磨（1.5d） | 端到端冒烟、性能、UI 细节 | 全项 |

**总计约 24 人天**（全职约 5 周，业余约 2–3 个月）。

### 13.1 如果时间不够，砍的顺序
```
先砍：P5 视频智能体 → P8 计费闭环 → P12 打磨
绝不砍：P6 评测体系 ★、P4 三层降级 ★、P1 多租户隔离测试 ★、P11 文档 ★
```
理由：前三个是"又多一个功能"，后四个是"这个候选人会不会做 AI 工程"。

---

## 14. 决策记录（ADR）

> 每条都要往 `docs/adr/` 写一份。**这是面试时最值钱的东西**——
> 它证明你不是照着教程敲的，而是做过权衡。

| # | 决策 | 理由 | 代价 / 放弃 |
|---|---|---|---|
| 001 | 自建 RAG，不用 WeKnora | 能讲清检索链路且可评测；少一套服务与权限模型 | 解析/切片要自己写 |
| 002 | 应用层过滤 + PostgreSQL RLS 双层隔离 | 单靠应用层过滤，漏一处就是数据泄漏 | RLS 增加调试难度 |
| 003 | 切片默认 500/50，但做成可配 | 评测阶段需要对比不同尺寸 | 配置项变多 |
| 004 | 中文分词在应用层做，不装 zhparser | 部署简单，分词行为可控可测 | 分词质量依赖所选库 |
| 005 | 三层降级（重试→简化→失败），不用静默兜底 | 失败必须可见，用户需要知道拿到的是什么 | 需要维护两套 Manim prompt |
| 006 | 自评必须用不同模型 | 同模型自评无验证价值 | 成本上升，且需两个 provider key |
| 007 | 积分是计价单位，token 是成本单位 | 视频/图片按次计费，与 token 不同量纲 | 多一层折算 |
| 008 | 不引入 Redis | 当前没有需要它的场景；embedding 去重缓存用 PG 表足够 | 将来高并发需补 |
| 009 | Python 服务保持"哑"，所有 LLM 调用在 Go | 重试/降级/计费只有一份实现 | Go 里要写不少编排代码 |
| 010 | Manim 用 `-ql` 480p15 | 比高清快 5–10 倍，投影够用 | 画面精度有限 |
| 011 | PPTX 用母版填充，不让模型排版 | 排版是无底洞，且模型做不好 | 版式固定 |
| 012 | 评测数据集体量小但人工标注 | 30 条精准标注 > 300 条自动生成 | 需要人工投入 |
| 013 | 不做套餐，只做积分 + 订单 | 套餐是商业化概念，对本项目无增量价值 | 与照片略有出入 |

---

## 15. 关键实现细节

### 15.1 中文字体（最经典的坑）
- Python 镜像必须装 `fonts-noto-cjk`
- 所有 `Text()` **必须显式** `font="Noto Sans CJK SC"`
- 实施后**必须目视检查**渲染出的 MP4，不能只看"渲染成功"

### 15.2 Manim 渲染参数
```bash
manim -ql --disable_caching --media_dir <tmp> <script.py> EduAnimation
```
- 输出：`<media_dir>/videos/<stem>/480p15/EduAnimation.mp4`
- 渲染后**复制到应用自己的存储目录**，不依赖 manim 的目录结构
- **超时 120 秒**，超时按失败处理，进入 L1 重试
- 每任务独立临时目录，结束清理

### 15.3 PPTX 生成
1. 人工制作一次 `templates/lesson.pptx` 母版，含版式：`Title Slide` / `Title and Content`
2. `pptx_builder.py`：第 1 页用封面版式，其余用标题内容版式，bullets 填占位符，
   `image_hint` 写进备注区（**不插图**，避免版权与下载失败）
3. 字号配色由母版决定，代码不设字体大小
4. 若无法生成二进制母版，用 `python-pptx` 默认模板但**必须显式设置中文字体**
5. **验收必须用 PowerPoint / WPS 实际打开检查中文**

### 15.4 时间预算（M2 之后应达到）
| 阶段 | 目标 | 手段 |
|---|---|---|
| 检索 | ≤ 3s | HNSW + 租户索引 |
| 教案 + 大纲 | ≤ 45s | **并发** |
| PPTX | ≤ 10s | 模板填充 |
| 分镜 + Manim 代码 | ≤ 40s | 分镜用便宜模型 |
| Manim 渲染 | ≤ 90s | `-ql` |
| 视频（异步） | ≤ 5min | 不阻塞其他产物展示 |
| 自评 | ≤ 20s | 与结果展示解耦 |

**课件与动画任务并行启动**，总时长取决于动画。

### 15.5 成本控制手段（写进 ADR 与文档）
1. **embedding 去重缓存**：内容 hash 命中则不重复调用
2. **模型分级路由**：分镜/标题/抽取用 `LLM_CHEAP_MODEL`，代码生成/教案用强模型
3. **同组任务共享检索结果**：课件和动画任务只检索一次
4. **输出长度约束**：教案 ≤ 1200 字，大纲 8–12 页
5. **token 预算熔断**：单任务超预算则中止并标记 failed
6. **usage_logs 记录 `cached` 字段**，仪表盘展示缓存命中率

---

## 16. 面试可讲点清单（★ 这份文档存在的最终理由）

| 面试问题 | 你的回答素材 | 代码位置 |
|---|---|---|
| 你做过 RAG 吗？ | 混合检索（向量 + 中文分词关键词）+ RRF 融合 + rerank，每层可开关，有对比数据 | `internal/rag/`、`docs/eval-report.md` |
| 怎么评估 RAG 效果？ | 人工标注 golden set，Recall@K / MRR / HitRate，四张配置对比表 | `internal/eval/`、`evals/` |
| 怎么防止幻觉？ | 引用溯源：检索片段带来源标注注入 prompt，产物保存检索快照，界面可展开核对 | `internal/rag/`、`lesson_plans` |
| 模型调用失败怎么办？ | 三层降级：重试修复 → 简化模式重生成 → 显式失败可重试。降级率 12%，全部落库可观测 | `internal/orchestrator/`、`degradation_events` |
| 多租户怎么做的？ | 应用层强制 tenant_id + PostgreSQL RLS 兜底 + 三个隔离测试；向量检索带租户索引 | `internal/tenant/`、migrations |
| 怎么控制成本？ | embedding 缓存、模型分级路由、同组共享检索、token 熔断；仪表盘有成本分布 | `internal/provider/`、`usage_logs` |
| 为什么不用现成的开源知识库平台？ | ADR-001：自建才能做检索评测和 rerank 对比，且少一套权限模型 | `docs/adr/001` |
| 并发怎么处理的？ | 同组任务并行、课件内教案与大纲并发、SSE 推送进度、任务状态机 | `internal/orchestrator/` |
| 怎么保证计费不重复扣？ | `point_transactions` 幂等键唯一约束，预扣 + 失败全额退回 | `internal/billing/` |
| 这个项目最难的地方？ | Manim 代码生成成功率低 → 设计了修复重试 + 简化模式两层降级，并用降级率作为可观测指标 | `internal/animation/` |

---

## 17. 环境与启动

### 17.1 `.env.example`
```env
DATABASE_URL=postgres://edu:edu@localhost:5432/edu?sslmode=disable
GO_PORT=8080
RENDER_SERVICE_URL=http://localhost:8000
JWT_SECRET=change-me-in-production

LLM_BASE_URL=https://api.openai.com/v1
LLM_API_KEY=sk-xxx
LLM_GENERATION_MODEL=gpt-4o
LLM_CHEAP_MODEL=gpt-4o-mini
LLM_EVAL_MODEL=claude-sonnet-4-5
LLM_EMBEDDING_MODEL=text-embedding-3-small

RERANK_ENABLED=true
VIDEO_PROVIDER=runway
VIDEO_API_KEY=

CHUNK_SIZE=500
CHUNK_OVERLAP=50

STORAGE_DIR=./storage
VITE_API_BASE=http://localhost:8080/api
```

### 17.2 README 必须包含
- 一句话项目说明 + 架构图（Mermaid）
- **截图**（工作台、仪表盘、评测对比）★ 面试官只看这个
- 快速开始（逐字可执行）
- 核心技术点 + 指向 `docs/adr/`
- 评测结果摘要 + 指向 `docs/eval-report.md`

### 17.3 冒烟清单
```
[ ] docker compose up 后三个服务健康检查通过
[ ] 渲染服务 /health 报告 ffmpeg / manim / 中文字体均可用
[ ] admin 登录成功，能创建租户并切换
[ ] 租户 A 上传文档后，切到租户 B 看不到
[ ] 检索测试：切换混合检索/rerank，分数与来源正确显示
[ ] 输入"初中物理 浮力"，3 分钟内出教案 + PPTX + 动画
[ ] PPTX 用 WPS 打开中文正常
[ ] MP4 中文正常（不是方块）
[ ] 评测页能跑一次 golden set 并出指标
[ ] 仪表盘降级率、成本分布有数据
[ ] go test ./... 全绿
```

---

## 18. 开工前检查

```
[ ] 有可用的 LLM API key，且 LLM_EVAL_MODEL ≠ LLM_GENERATION_MODEL
[ ] 有可用的视频生成 API key（确认是 Runway 还是可灵/即梦）
[ ] ★ 先单独验证：Python 环境能渲染出带中文的 MP4
      （manim + texlive + fonts-noto-cjk）。30 分钟能验完，
      验不过整个动画方案要重新讨论。这是全项目技术风险最高的单点。
[ ] 准备演示用的教材 PDF + 课程标准文件（用于知识库和 golden set）
[ ] 预留人工时间：制作 PPTX 母版、标注 golden set（约 30–50 条）
[ ] 建好 GitHub 仓库（CI 和 README 是交付物的一部分）
```

**第一件事永远是验证 Manim 中文渲染。** 不要先写业务代码。
