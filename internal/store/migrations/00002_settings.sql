-- +goose Up
CREATE TABLE settings (
    name text PRIMARY KEY CHECK (name IN ('junk_patterns', 'commit_sizes', 'rapid_series')),
    value jsonb NOT NULL
);

ALTER TABLE users DROP COLUMN junk_patterns;
ALTER TABLE repositories ADD COLUMN commit_order text[];
ALTER TABLE rapid_series DROP CONSTRAINT rapid_series_commit_shas_check;
ALTER TABLE rapid_series ADD CONSTRAINT rapid_series_commit_shas_check CHECK (cardinality(commit_shas) >= 2);

-- +goose Down
ALTER TABLE rapid_series DROP CONSTRAINT rapid_series_commit_shas_check;
ALTER TABLE rapid_series ADD CONSTRAINT rapid_series_commit_shas_check CHECK (cardinality(commit_shas) >= 3);
ALTER TABLE repositories DROP COLUMN commit_order;
ALTER TABLE users ADD COLUMN junk_patterns text[];
DROP TABLE settings;
