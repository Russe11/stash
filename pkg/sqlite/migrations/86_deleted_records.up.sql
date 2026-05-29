-- Records deletions of top-level syncable entities so that clients can reconcile removals
-- incrementally (via the deletedSince query) instead of diffing the entire id set. Rows are
-- written by the shared destroy path, filtered to an allowlist of synced entity types.
CREATE TABLE `deleted_records` (
  `id` integer not null primary key autoincrement,
  `record_type` varchar(40) not null,
  `record_id` integer not null,
  `deleted_at` datetime not null
);

CREATE INDEX `index_deleted_records_on_deleted_at` ON `deleted_records` (`deleted_at`);
