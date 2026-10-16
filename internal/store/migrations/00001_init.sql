-- +goose Up
CREATE TYPE user_role AS ENUM ('supervisor', 'coordinator', 'repo_manager');
CREATE TYPE stream AS ENUM ('bachelor', 'master');
CREATE TYPE period_stream AS ENUM ('all', 'bachelor', 'master');
CREATE TYPE period_kind AS ENUM ('work_start', 'practice', 'checkpoint');
CREATE TYPE analysis_status AS ENUM ('never', 'queued', 'running', 'done', 'failed');

CREATE TABLE users (
    id bigserial PRIMARY KEY,
    login text NOT NULL UNIQUE CHECK (login ~ '^[a-zA-Z0-9_.-]{3,50}$'),
    password_hash text NOT NULL,
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 100),
    role user_role NOT NULL,
    stream stream,
    junk_patterns text[],
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_stream_only_for_coordinator CHECK ((role = 'coordinator') = (stream IS NOT NULL))
);

CREATE TABLE students (
    id bigserial PRIMARY KEY,
    supervisor_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    full_name text NOT NULL CHECK (char_length(full_name) BETWEEN 1 AND 200),
    stream stream NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX students_supervisor_id_idx ON students (supervisor_id);
CREATE INDEX students_stream_idx ON students (stream);

CREATE TABLE repositories (
    id bigserial PRIMARY KEY,
    student_id bigint NOT NULL UNIQUE REFERENCES students (id) ON DELETE CASCADE,
    clone_url text NOT NULL CHECK (clone_url <> ''),
    default_branch text,
    hosting_metadata jsonb,
    status analysis_status NOT NULL DEFAULT 'never',
    status_reason text,
    analysis_started_at timestamptz,
    analysis_finished_at timestamptz,
    last_analyzed_head text,
    clone_size_bytes bigint,
    checklist jsonb,
    last_commit_at timestamptz
);

CREATE TABLE commits (
    repository_id bigint NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    sha text NOT NULL CHECK (sha ~ '^[0-9a-f]{40}$'),
    author_name text NOT NULL,
    author_email text NOT NULL,
    authored_at timestamptz NOT NULL,
    committed_at timestamptz NOT NULL,
    message text NOT NULL,
    is_merge boolean NOT NULL,
    lines_added integer NOT NULL CHECK (lines_added >= 0),
    lines_deleted integer NOT NULL CHECK (lines_deleted >= 0),
    PRIMARY KEY (repository_id, sha)
);

CREATE INDEX commits_repository_id_authored_at_idx ON commits (repository_id, authored_at);

CREATE TABLE file_changes (
    repository_id bigint NOT NULL,
    sha text NOT NULL,
    path text NOT NULL,
    old_path text,
    lines_added integer NOT NULL CHECK (lines_added >= 0),
    lines_deleted integer NOT NULL CHECK (lines_deleted >= 0),
    is_binary boolean NOT NULL,
    PRIMARY KEY (repository_id, sha, path),
    FOREIGN KEY (repository_id, sha) REFERENCES commits (repository_id, sha) ON DELETE CASCADE
);

CREATE INDEX file_changes_repository_id_path_idx ON file_changes (repository_id, path);

CREATE TABLE snapshot_files (
    repository_id bigint NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    path text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    is_binary boolean NOT NULL,
    category text NOT NULL CHECK (category IN ('code', 'docs', 'tests', 'config', 'binary')),
    code_lines integer NOT NULL CHECK (code_lines >= 0),
    comment_lines integer NOT NULL CHECK (comment_lines >= 0),
    PRIMARY KEY (repository_id, path)
);

CREATE TABLE rapid_series (
    id bigserial PRIMARY KEY,
    repository_id bigint NOT NULL REFERENCES repositories (id) ON DELETE CASCADE,
    commit_shas text[] NOT NULL CHECK (cardinality(commit_shas) >= 3),
    started_at timestamptz NOT NULL,
    ended_at timestamptz NOT NULL CHECK (ended_at >= started_at)
);

CREATE INDEX rapid_series_repository_id_idx ON rapid_series (repository_id);

CREATE TABLE calendar_periods (
    id bigserial PRIMARY KEY,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    kind period_kind NOT NULL,
    stream period_stream NOT NULL,
    start_date date,
    end_date date,
    CONSTRAINT calendar_periods_dates_both_or_none CHECK ((start_date IS NULL) = (end_date IS NULL)),
    CONSTRAINT calendar_periods_end_after_start CHECK (end_date >= start_date),
    CONSTRAINT calendar_periods_checkpoint_single_day CHECK (kind <> 'checkpoint' OR start_date = end_date)
);

INSERT INTO calendar_periods (name, kind, stream, start_date, end_date) VALUES
    ('Start of thesis work', 'work_start', 'all', '2026-09-01', '2026-09-14'),
    ('Bachelor practice', 'practice', 'bachelor', '2027-04-01', '2027-05-31'),
    ('Master practice', 'practice', 'master', NULL, NULL),
    ('Checkpoint', 'checkpoint', 'all', '2027-01-01', '2027-01-01');

CREATE TABLE sessions (
    token text PRIMARY KEY,
    data bytea NOT NULL,
    expiry timestamptz NOT NULL
);

CREATE INDEX sessions_expiry_idx ON sessions (expiry);

-- +goose Down
DROP TABLE sessions;
DROP TABLE calendar_periods;
DROP TABLE rapid_series;
DROP TABLE snapshot_files;
DROP TABLE file_changes;
DROP TABLE commits;
DROP TABLE repositories;
DROP TABLE students;
DROP TABLE users;
DROP TYPE analysis_status;
DROP TYPE period_kind;
DROP TYPE period_stream;
DROP TYPE stream;
DROP TYPE user_role;
