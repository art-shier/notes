CREATE TABLE invitations (
	id VARCHAR(36) NOT NULL, 
	secret_hash VARCHAR(64) NOT NULL, 
	email VARCHAR(254) NOT NULL, 
	role VARCHAR(20) NOT NULL, 
	expires_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	used_at TIMESTAMP WITHOUT TIME ZONE, 
	PRIMARY KEY (id), 
	UNIQUE (secret_hash)
);
CREATE TABLE users (
	id VARCHAR(36) NOT NULL, 
	email VARCHAR(254) NOT NULL, 
	display_name VARCHAR(60) NOT NULL, 
	password_hash TEXT NOT NULL, 
	role VARCHAR(20) NOT NULL, 
	disabled BOOLEAN NOT NULL, 
	quota_bytes BIGINT NOT NULL, 
	used_bytes BIGINT NOT NULL, 
	created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	PRIMARY KEY (id), 
	UNIQUE (email)
);
CREATE TABLE api_tokens (
	id VARCHAR(36) NOT NULL, 
	user_id VARCHAR(36) NOT NULL, 
	name VARCHAR(60) NOT NULL, 
	prefix VARCHAR(16) NOT NULL, 
	secret_hash VARCHAR(64) NOT NULL, 
	scopes JSON NOT NULL, 
	expires_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	revoked_at TIMESTAMP WITHOUT TIME ZONE, 
	created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	PRIMARY KEY (id), 
	FOREIGN KEY(user_id) REFERENCES users (id), 
	UNIQUE (secret_hash)
);
CREATE INDEX ix_api_tokens_user_id ON api_tokens (user_id);
CREATE TABLE attachments (
	id VARCHAR(36) NOT NULL, 
	user_id VARCHAR(36) NOT NULL, 
	object_key VARCHAR(80) NOT NULL, 
	mime_type VARCHAR(40) NOT NULL, 
	size_bytes BIGINT NOT NULL, 
	created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	PRIMARY KEY (id), 
	FOREIGN KEY(user_id) REFERENCES users (id), 
	UNIQUE (object_key)
);
CREATE INDEX ix_attachments_user_id ON attachments (user_id);
CREATE TABLE export_bundles (
	id VARCHAR(36) NOT NULL, 
	user_id VARCHAR(36) NOT NULL, 
	ready BOOLEAN NOT NULL, 
	size_bytes BIGINT NOT NULL, 
	manifest JSON NOT NULL, 
	created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	expires_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	PRIMARY KEY (id), 
	FOREIGN KEY(user_id) REFERENCES users (id)
);
CREATE INDEX ix_export_bundles_expires_at ON export_bundles (expires_at);
CREATE INDEX ix_export_bundles_user_id ON export_bundles (user_id);
CREATE TABLE folders (
	id VARCHAR(36) NOT NULL, 
	user_id VARCHAR(36) NOT NULL, 
	parent_id VARCHAR(36), 
	parent_key VARCHAR(36) NOT NULL, 
	name VARCHAR(80) NOT NULL, 
	is_inbox BOOLEAN NOT NULL, 
	PRIMARY KEY (id), 
	UNIQUE (user_id, parent_key, name), 
	FOREIGN KEY(user_id) REFERENCES users (id), 
	FOREIGN KEY(parent_id) REFERENCES folders (id)
);
CREATE INDEX ix_folders_user_id ON folders (user_id);
CREATE TABLE idempotency_records (
	id VARCHAR(36) NOT NULL, 
	user_id VARCHAR(36) NOT NULL, 
	actor_key VARCHAR(40) NOT NULL, 
	route VARCHAR(80) NOT NULL, 
	key VARCHAR(128) NOT NULL, 
	request_hash VARCHAR(64) NOT NULL, 
	response JSON, 
	expires_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	PRIMARY KEY (id), 
	UNIQUE (user_id, actor_key, route, key), 
	FOREIGN KEY(user_id) REFERENCES users (id)
);
CREATE INDEX ix_idempotency_records_expires_at ON idempotency_records (expires_at);
CREATE TABLE sessions (
	id VARCHAR(36) NOT NULL, 
	user_id VARCHAR(36) NOT NULL, 
	secret_hash VARCHAR(64) NOT NULL, 
	expires_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	PRIMARY KEY (id), 
	FOREIGN KEY(user_id) REFERENCES users (id), 
	UNIQUE (secret_hash)
);
CREATE INDEX ix_sessions_user_id ON sessions (user_id);
CREATE TABLE tags (
	id VARCHAR(36) NOT NULL, 
	user_id VARCHAR(36) NOT NULL, 
	name VARCHAR(40) NOT NULL, 
	PRIMARY KEY (id), 
	UNIQUE (user_id, name), 
	FOREIGN KEY(user_id) REFERENCES users (id)
);
CREATE INDEX ix_tags_user_id ON tags (user_id);
CREATE TABLE notes (
	id VARCHAR(36) NOT NULL, 
	user_id VARCHAR(36) NOT NULL, 
	folder_id VARCHAR(36) NOT NULL, 
	title VARCHAR(300) NOT NULL, 
	content_json JSON NOT NULL, 
	block_ids JSON NOT NULL, 
	plain_text TEXT NOT NULL, 
	favorite BOOLEAN NOT NULL, 
	trashed BOOLEAN NOT NULL, 
	source VARCHAR(10) NOT NULL, 
	version INTEGER NOT NULL, 
	created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	PRIMARY KEY (id), 
	FOREIGN KEY(user_id) REFERENCES users (id), 
	FOREIGN KEY(folder_id) REFERENCES folders (id)
);
CREATE INDEX ix_notes_folder_id ON notes (folder_id);
CREATE INDEX ix_notes_user_id ON notes (user_id);
CREATE INDEX ix_notes_owner_updated ON notes (user_id, updated_at, id);
CREATE TABLE note_revisions (
	id VARCHAR(36) NOT NULL, 
	user_id VARCHAR(36) NOT NULL, 
	note_id VARCHAR(36) NOT NULL, 
	version INTEGER NOT NULL, 
	snapshot JSON NOT NULL, 
	action VARCHAR(30) NOT NULL, 
	actor VARCHAR(10) NOT NULL, 
	restored_from INTEGER, 
	saved_at TIMESTAMP WITHOUT TIME ZONE NOT NULL, 
	PRIMARY KEY (id), 
	UNIQUE (note_id, version), 
	FOREIGN KEY(user_id) REFERENCES users (id), 
	FOREIGN KEY(note_id) REFERENCES notes (id)
);
CREATE INDEX ix_revisions_owner_note_version ON note_revisions (user_id, note_id, version);
CREATE TABLE note_tags (
	user_id VARCHAR(36) NOT NULL, 
	note_id VARCHAR(36) NOT NULL, 
	tag_id VARCHAR(36) NOT NULL, 
	PRIMARY KEY (note_id, tag_id), 
	FOREIGN KEY(user_id) REFERENCES users (id), 
	FOREIGN KEY(note_id) REFERENCES notes (id), 
	FOREIGN KEY(tag_id) REFERENCES tags (id)
);
CREATE INDEX ix_note_tags_user_id ON note_tags (user_id);
CREATE TABLE alembic_version (version_num VARCHAR(32) NOT NULL PRIMARY KEY);
INSERT INTO alembic_version VALUES ('8942cd035480');