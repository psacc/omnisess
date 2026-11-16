-- Sessions exist but the message table does not.
CREATE TABLE session_v2 (
  id text PRIMARY KEY,
  title text,
  directory text NOT NULL,
  model text,
  time_created integer NOT NULL,
  time_updated integer NOT NULL
);
INSERT INTO session_v2 (id, title, directory, model, time_created, time_updated)
  VALUES ('ses_lonely', 'Lonely', '/Users/foo/prj/demo', '{"id":"m"}', 1700000000000, 1700000600000);
