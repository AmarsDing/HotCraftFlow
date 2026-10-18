-- FrameWork init: ensure app + log databases exist
SELECT 'CREATE DATABASE framework_log'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'framework_log')\gexec
