# Queries

SQL queries used by `sqlc` belong here. Business-critical writes, especially
redemptions, must execute inside explicit Go transactions and be protected by
database constraints.
