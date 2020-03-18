-- this script is executed when the postgres image boots
-- c.p. https://stackoverflow.com/a/51759325/1070080
CREATE DATABASE "tripbot_docker";
CREATE USER tripbot_docker WITH PASSWORD 'hunter2';
GRANT ALL PRIVILEGES ON DATABASE "tripbot_docker" to tripbot_docker;
