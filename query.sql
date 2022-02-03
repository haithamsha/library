-- name: User :one
SELECT *
  FROM users
 WHERE id = $1
 LIMIT 1;

-- name: UserBySlug :one
SELECT *
  FROM users
 WHERE slug = $1
 LIMIT 1;

-- name: Signup :one
INSERT
 INTO public.users(name, image, slug, email)
VALUES($1,$2,$3,$4)
       ON CONFLICT (email)
       DO UPDATE SET name = $1, image = $2
       RETURNING id;

-- name: UserUnshelvedBooks :many
SELECT books.id id, title, books.image image, google_books_id, slug, isbn
  FROM books, users
 WHERE users.id = books.user_id
   AND user_id = $1
   AND shelf_id IS NULL;

-- name: Shelves :many
SELECT id, name
  FROM shelves
 WHERE user_id = $1
 ORDER BY position;

-- name: ShelfBooks :many
SELECT books.id id, title, books.image image, google_books_id, slug, isbn
  FROM books, users
 WHERE users.id = books.user_id
   AND shelf_id = $1
 ORDER BY books.created_at DESC;

-- name: BookByIsbnAndUser :one
SELECT books.*, slug, shelves.name shelf_name
  FROM users, books
       LEFT JOIN shelves
           ON shelves.id = books.shelf_id
 WHERE users.id = books.user_id
   AND books.user_id = $1
   AND isbn = $2
 LIMIT 1;

-- name: Highlights :many
SELECT *
  FROM highlights
 WHERE book_id = $1;

-- name: NewBook :one
INSERT INTO public.books (title, isbn, author, subtitle, description, publisher, page_count, google_books_id, user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
       RETURNING *;

-- name: UpdateBook :exec
UPDATE public.books
   SET title = $1,
       author = $2,
       subtitle = $3,
       description = $4,
       publisher = $5,
       page_count = $6
 WHERE id = $7;

-- name: ShelfByIdAndUser :one
SELECT *
  FROM shelves
 WHERE shelves.user_id = $1
   AND shelves.id = $2
 LIMIT 1;

-- name: HighlightByIDAndBook :one
SELECT *
  FROM highlights
 WHERE id = $1
   AND book_id = $2
 LIMIT 1;

-- name: UpdateUser :exec
UPDATE users
   SET description = $1,
       amazon_associates_id = $2,
       facebook = $3,
       twitter = $4,
       linkedin = $5,
       instagram = $6,
       phone = $7,
       whatsapp = $8,
       telegram = $9
 WHERE id = $10;
