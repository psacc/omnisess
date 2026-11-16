-- time_created is TEXT holding a non-numeric value: scanning it into an
-- int64 must fail.
CREATE TABLE session_v2 (
  id text PRIMARY KEY,
  title text,
  directory text,
  model text,
  time_created text NOT NULL,
  time_updated integer NOT NULL
);
INSERT INTO session_v2 (id, title, directory, model, time_created, time_updated)
  VALUES ('ses_bad', 'bad', '/tmp', '', 'not-a-number', 1);
