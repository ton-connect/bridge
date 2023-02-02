BEGIN;

create schema if not exists bridge;
drop table if exists bridge.messages;
create table bridge.messages
(
    client_id                 text                 not null,
    event_id                  bigint               not null,
    end_time                  timestamp            not null,
    bridge_message            bytea                not null
);

COMMIT;
