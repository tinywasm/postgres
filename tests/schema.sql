-- dialect: postgres
--
-- Fixture for TestExportDDL_FullSchema in postgres/tests/export_test.go
--
-- Covers ALL DB types of the framework and the 3 PK strategies:
--
--   Auto-generated PK (DB):  id int64 `db:"pk,autoinc"` -> BIGSERIAL PRIMARY KEY
--   App-provided PK:         id string `db:"pk"`         -> TEXT PRIMARY KEY  (e.g. UUID)
--   Composite PK:            (user_id, role_id) int64    -> PRIMARY KEY (user_id, role_id)
--
-- DB types supported by the framework (FieldStruct/FieldIntSlice/FieldStructSlice are not DB columns):
--
--   FieldText  string without max         -> TEXT
--   FieldText  string with input:max=N    -> VARCHAR(N)
--   FieldRaw   RawJSON (string alias)     -> TEXT  (pre-serialized JSON, no additional encoding)
--   FieldInt   int64                      -> BIGINT
--   FieldFloat float64                    -> DOUBLE PRECISION
--   FieldBool  bool                       -> BOOLEAN
--   FieldBlob  []byte                     -> BYTEA
--
-- Source structures (Go) — identical to SQLite fixture, output differs by dialect:
--
--   users: id int64 `db:"pk,autoinc"`,
--          username string `db:"not_null" input:"required,max=50"` UNIQUE,
--          email string `db:"not_null"` UNIQUE,
--          score float64, active bool, avatar []byte
--
--   roles: id int64 `db:"pk,autoinc"`,
--          name string `db:"not_null" input:"required,max=100"` UNIQUE
--
--   sessions: id string `db:"pk"`,                          <- App-provided PK (UUID)
--             user_id int64 `db:"ref=users"`,               <- ON DELETE CASCADE by default
--             metadata RawJSON                              <- FieldRaw -> TEXT
--
--   user_roles: user_id int64 `db:"pk,ref=users"`,          <- Composite PK
--               role_id int64 `db:"pk,ref=roles"`
--
-- Order guaranteed by TopologicalSort: users -> roles -> sessions -> user_roles

CREATE TABLE IF NOT EXISTS users (id BIGSERIAL PRIMARY KEY, username VARCHAR(50) NOT NULL UNIQUE, email TEXT NOT NULL UNIQUE, score DOUBLE PRECISION, active BOOLEAN, avatar BYTEA);

CREATE TABLE IF NOT EXISTS roles (id BIGSERIAL PRIMARY KEY, name VARCHAR(100) NOT NULL UNIQUE);

CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, user_id BIGINT, metadata TEXT, CONSTRAINT fk_sessions_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);

CREATE TABLE IF NOT EXISTS user_roles (user_id BIGINT NOT NULL, role_id BIGINT NOT NULL, PRIMARY KEY (user_id, role_id), CONSTRAINT fk_user_roles_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE, CONSTRAINT fk_user_roles_role_id FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE);

CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles(user_id);

CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON user_roles(role_id);
