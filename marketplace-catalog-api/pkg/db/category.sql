-- name: CategoryList :many
SELECT appstore.categories.id, appstore.categories.name
FROM appstore.categories
WHERE appstore.categories.deleted IS NULL
ORDER BY appstore.categories.name;
