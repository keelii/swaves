CREATE TABLE IF NOT EXISTS posts
(
    id
    INTEGER
    PRIMARY
    KEY
    AUTOINCREMENT,
    title
    TEXT
    NOT
    NULL,
    slug
    TEXT
    NOT
    NULL
    UNIQUE,
    content
    TEXT
    NOT
    NULL,
    status
    TEXT
    NOT
    NULL,
    created_at
    INTEGER
    NOT
    NULL,
    updated_at
    INTEGER
    NOT
    NULL,
    deleted_at
    INTEGER
);

CREATE TABLE IF NOT EXISTS encrypted_posts
(
    id
    INTEGER
    PRIMARY
    KEY
    AUTOINCREMENT,
    title
    TEXT
    NOT
    NULL,
    slug
    TEXT
    NOT
    NULL
    UNIQUE,
    content
    TEXT
    NOT
    NULL,
    password
    TEXT
    NOT
    NULL,
    expires_at
    INTEGER,
    created_at
    INTEGER
    NOT
    NULL,
    updated_at
    INTEGER
    NOT
    NULL,
    deleted_at
    INTEGER
);

CREATE TABLE IF NOT EXISTS tags
(
    id
    INTEGER
    PRIMARY
    KEY
    AUTOINCREMENT,
    name
    TEXT
    NOT
    NULL,
    slug
    TEXT
    NOT
    NULL
    UNIQUE,
    created_at
    INTEGER
    NOT
    NULL,
    updated_at
    INTEGER
    NOT
    NULL,
    deleted_at
    INTEGER
);

CREATE TABLE IF NOT EXISTS post_tags
(
    post_id
    INTEGER
    NOT
    NULL,
    tag_id
    INTEGER
    NOT
    NULL,
    created_at
    INTEGER
    NOT
    NULL,
    updated_at
    INTEGER
    NOT
    NULL,
    deleted_at
    INTEGER,
    UNIQUE
(
    post_id,
    tag_id
)
    );

CREATE TABLE IF NOT EXISTS redirects
(
    id
    INTEGER
    PRIMARY
    KEY
    AUTOINCREMENT,
    from_path
    TEXT
    NOT
    NULL
    UNIQUE,
    to_path
    TEXT
    NOT
    NULL,
    created_at
    INTEGER
    NOT
    NULL,
    updated_at
    INTEGER
    NOT
    NULL,
    deleted_at
    INTEGER
);

CREATE TABLE IF NOT EXISTS settings
(
    id
    INTEGER
    PRIMARY
    KEY
    AUTOINCREMENT,

    category
    TEXT
    NOT
    NULL
    DEFAULT
    'default',
    name
    TEXT
    NOT
    NULL,
    code
    TEXT
    NOT
    NULL
    UNIQUE,
    type
    TEXT
    NOT
    NULL,
    options
    TEXT,
    attrs
    TEXT,
    value
    TEXT,
    description
    TEXT,
    sort
    INTEGER
    NOT
    NULL
    DEFAULT
    0,

    created_at
    INTEGER
    NOT
    NULL,
    updated_at
    INTEGER
    NOT
    NULL,
    deleted_at
    INTEGER
);

CREATE INDEX IF NOT EXISTS idx_settings_category ON settings(category);
CREATE INDEX IF NOT EXISTS idx_settings_code ON settings(code);
,

CREATE TABLE IF NOT EXISTS admin_sessions
(
    id
    TEXT
    PRIMARY
    KEY,
    sid
    TEXT
    NOT
    NULL
    UNIQUE,
    expires_at
    INTEGER
    NOT
    NULL,
    created_at
    INTEGER
    NOT
    NULL,
    updated_at
    INTEGER
    NOT
    NULL,
    deleted_at
    INTEGER
);

CREATE TABLE IF NOT EXISTS http_error_logs
(
    id
    INTEGER
    PRIMARY
    KEY
    AUTOINCREMENT,

    req_id
    TEXT
    NOT
    NULL,
    client_ip
    TEXT
    NOT
    NULL,
    method
    TEXT
    NOT
    NULL,
    path
    TEXT
    NOT
    NULL,
    status
    INTEGER
    NOT
    NULL,
    user_agent
    TEXT
    NOT
    NULL,

    query_params
    TEXT,
    body_params
    TEXT,

    created_at
    INTEGER
    NOT
    NULL,
    expired_at
    INTEGER
    NOT
    NULL
);
CREATE INDEX IF NOT EXISTS idx_http_error_logs_created_at
    ON http_error_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_http_error_logs_expired_at
    ON http_error_logs(expired_at);
CREATE INDEX IF NOT EXISTS idx_http_error_logs_path
    ON http_error_logs(path);
CREATE INDEX IF NOT EXISTS idx_http_error_logs_status
    ON http_error_logs(status);
CREATE TABLE IF NOT EXISTS cron_jobs
(
    id
    INTEGER
    PRIMARY
    KEY
    AUTOINCREMENT,

    name
    TEXT
    NOT
    NULL,    -- 任务名称（后台展示）
    description
    TEXT
    NOT
    NULL
    DEFAULT
    '',

    schedule
    TEXT
    NOT
    NULL,    -- cron 表达式，如 "0 */5 * * *"
    enabled
    INTEGER
    NOT
    NULL
    DEFAULT
    1,       -- 1=启用 0=停用

    last_run_at
    INTEGER, -- 最近一次开始执行时间（可选）
    last_success_at
    INTEGER, -- 最近一次成功时间（可选）
    last_error_at
    INTEGER, -- 最近一次失败时间（可选）

    created_at
    INTEGER
    NOT
    NULL,
    updated_at
    INTEGER
    NOT
    NULL,
    deleted_at
    INTEGER
);

CREATE TABLE IF NOT EXISTS cron_job_logs
(
    id
    INTEGER
    PRIMARY
    KEY
    AUTOINCREMENT,

    job_id
    INTEGER
    NOT
    NULL,    -- cron_jobs.id（无外键）
    run_id
    TEXT
    NOT
    NULL,    -- 单次执行唯一标识（UUID）

    status
    TEXT
    NOT
    NULL,    -- "success" | "error"
    message
    TEXT
    NOT
    NULL
    DEFAULT
    '',      -- 简要结果 / 错误信息

    started_at
    INTEGER
    NOT
    NULL,    -- 执行开始时间
    finished_at
    INTEGER
    NOT
    NULL,    -- 执行结束时间
    duration
    INTEGER
    NOT
    NULL,    -- 执行耗时（毫秒）

    expire_at
    INTEGER, -- 过期时间（可为空）

    created_at
    INTEGER
    NOT
    NULL
);
CREATE INDEX IF NOT EXISTS idx_cron_job_logs_job_id
    ON cron_job_logs(job_id);
CREATE INDEX IF NOT EXISTS idx_cron_job_logs_job_id_status
    ON cron_job_logs(job_id, status);
CREATE INDEX IF NOT EXISTS idx_cron_job_logs_created_at
    ON cron_job_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_cron_job_logs_expire_at
    ON cron_job_logs(expire_at);