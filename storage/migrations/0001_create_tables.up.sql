BEGIN;

create schema if not exists bridge;

create table bridge.messages
(
    client_id                 text                 not null,
    end_time                  timestamp            not null,
    bridge_message            bytea                not null
);

COMMIT;
