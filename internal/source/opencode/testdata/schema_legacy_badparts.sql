-- part data is NULL: scanning NULL into a string must fail.
CREATE TABLE message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  time_created integer NOT NULL,
  data text NOT NULL
);
CREATE TABLE part (
  id text PRIMARY KEY,
  message_id text NOT NULL,
  time_created integer NOT NULL,
  data text
);
INSERT INTO message (id, session_id, time_created, data) VALUES ('m1', 's1', 0, '{"role":"user"}');
INSERT INTO part (id, message_id, time_created, data) VALUES ('p1', 'm1', 0, NULL);
