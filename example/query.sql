-- name: GetAuthor :one
SELECT * FROM authors
WHERE id = $1 LIMIT 1;

-- name: ListAuthors :many
SELECT * FROM authors
ORDER BY name;

-- name: CreateAuthor :one
INSERT INTO authors (
    name, bio
) VALUES (
    $1, $2
)
RETURNING *;

-- name: UpdateAuthor :exec
UPDATE authors
SET name = $2, bio = $3
WHERE id = $1;

-- name: DeleteAuthor :exec
DELETE FROM authors
WHERE id = $1;

-- name: GetBooksByAuthor :many
SELECT b.*, a.name as author_name
FROM books b
JOIN authors a ON a.id = b.author_id
WHERE b.author_id = $1
ORDER BY b.created_at DESC;

-- name: CreateBook :one
INSERT INTO books (
    title, author_id, price
) VALUES (
    $1, $2, $3
)
RETURNING *;

-- name: GetAuthorWithBooks :many
WITH author_books AS (
    SELECT * FROM books WHERE author_id = $1
)
SELECT * FROM author_books;

-- name: TransferBooks :exec
WITH moved AS (
    UPDATE books SET author_id = $2 WHERE author_id = $1 RETURNING *
)
SELECT count(*) FROM moved;

-- name: LockAuthor :one
-- rw:write
SELECT * FROM authors WHERE id = $1 FOR UPDATE;

-- name: GetAuthorFromPrimary :one
-- rw:write
SELECT * FROM authors WHERE id = $1;
