-- Messages exist but the part table does not.
CREATE TABLE session (
  id text PRIMARY KEY,
  title text NOT NULL,
  directory text NOT NULL,
  model text,
  time_created integer NOT NULL,
  time_updated integer NOT NULL
);
CREATE TABLE message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  time_created integer NOT NULL,
  data text NOT NULL
);
INSERT INTO session (id, title, directory, model, time_created, time_updated)
  VALUES ('ses_lonely', 'Lonely', '/Users/foo/prj/demo', '', 1700000000000, 1700000600000);
INSERT INTO message (id, session_id, time_created, data)
  VALUES ('m1', 'ses_lonely', 1700000001000, '{"role":"user"}');
