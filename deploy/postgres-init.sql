-- First boot only (empty volume). pg_restore recreates objects on migrate.
CREATE SCHEMA IF NOT EXISTS keycloak;
CREATE SCHEMA IF NOT EXISTS app;
GRANT ALL ON SCHEMA keycloak TO CURRENT_USER;
GRANT ALL ON SCHEMA app TO CURRENT_USER;
GRANT ALL ON SCHEMA public TO CURRENT_USER;
