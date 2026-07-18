CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Historical bootstrap note. New installations use 0001_core_schema.up.sql;
-- runtime AutoMigrate is disabled and every admin V1 schema change is versioned.
--
-- Required table set covered by internal/platform/model:
-- users, email_verification_codes, schools, colleges, majors, courses,
-- materials, material_access_grants, orders, payment_records, quiz_questions,
-- quiz_options, quiz_attempts, quiz_answers, wrong_questions,
-- weakness_reports, wiki_entries, wiki_edit_histories,
-- wiki_creator_applications, blog_posts, blog_comments, forum_boards,
-- forum_posts, forum_replies, moments, media_assets, moment_comments, user_relations,
-- points_logs, points_rules, memberships, membership_plans, ai_tasks,
-- ai_drafts, ai_usage_logs, notifications, reports, operation_logs,
-- leaderboard_snapshots, system_configs.
