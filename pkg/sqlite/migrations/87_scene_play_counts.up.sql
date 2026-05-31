CREATE TABLE `scenes_play_counts` (
  `scene_id` integer not null primary key references `scenes`(`id`) on delete CASCADE,
  `play_count` integer not null default 0
);
