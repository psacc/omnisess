-- message data is NULL: scanning NULL into a string must fail.
CREATE TABLE message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  time_created integer NOT NULL,
  data text
);
INSERT INTO message (id, session_id, time_created, data) VALUES ('m1', 's1', 0, NULL);
