-- message type is NULL: scanning NULL into a string must fail.
CREATE TABLE session_message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  type text,
  seq integer NOT NULL,
  data text NOT NULL
);
INSERT INTO session_message (id, session_id, type, seq, data) VALUES ('m1', 's1', NULL, 0, '{}');
