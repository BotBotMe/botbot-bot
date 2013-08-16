BEGIN;
CREATE TABLE "bots_chatbot" (
    "id" serial NOT NULL PRIMARY KEY,
    "is_active" boolean NOT NULL,
    "connection" hstore NOT NULL,
    "server" varchar(100) NOT NULL,
    "server_password" varchar(100),
    "nick" varchar(64) NOT NULL,
    "password" varchar(100),
    "real_name" varchar(250) NOT NULL
)
;
CREATE TABLE "bots_channel" (
    "id" serial NOT NULL PRIMARY KEY,
    "chatbot_id" integer NOT NULL REFERENCES "bots_chatbot" ("id") DEFERRABLE INITIALLY DEFERRED,
    "name" varchar(250) NOT NULL,
    "slug" varchar(50) UNIQUE,
    "password" varchar(250),
    "is_public" boolean NOT NULL,
    "is_active" boolean NOT NULL,
    "is_featured" boolean NOT NULL
)
;
CREATE TABLE "bots_usercount" (
    "id" serial NOT NULL PRIMARY KEY,
    "channel_id" integer NOT NULL REFERENCES "bots_channel" ("id") DEFERRABLE INITIALLY DEFERRED,
    "dt" date NOT NULL,
    "counts" int[]
)
;

COMMIT;
