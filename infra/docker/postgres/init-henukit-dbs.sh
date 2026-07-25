#!/bin/bash
# Creates HENU Kit multi-database schema on a single Postgres instance.
set -euo pipefail

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
  SELECT 'CREATE DATABASE study'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'study')\gexec

  SELECT 'CREATE DATABASE platform'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'platform')\gexec

  SELECT 'CREATE DATABASE quizcraft'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'quizcraft')\gexec

  SELECT 'CREATE DATABASE notice'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'notice')\gexec

  SELECT 'CREATE DATABASE library'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'library')\gexec

  SELECT 'CREATE DATABASE food'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'food')\gexec

  SELECT 'CREATE DATABASE portal'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'portal')\gexec
EOSQL
