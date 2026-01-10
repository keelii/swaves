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