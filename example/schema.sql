CREATE TABLE authors (
    id   BIGSERIAL PRIMARY KEY,
    name text NOT NULL,
    bio  text
);

CREATE TABLE books (
    id        BIGSERIAL PRIMARY KEY,
    title     text NOT NULL,
    author_id BIGINT NOT NULL REFERENCES authors(id),
    price     NUMERIC(10, 2),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
