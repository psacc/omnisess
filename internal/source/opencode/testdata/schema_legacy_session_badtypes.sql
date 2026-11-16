-- `session` exists but time_created holds a non-numeric TEXT value.
CREATE TABLE session (
  id text PRIMARY KEY,
  title text NOT NULL,
  directory text NOT NULL,
  model text,
  time_created text NOT NULL,
  time_updated integer NOT NULL
);
INSERT INTO session (id, title, directory, model, time_created, time_updated)
  VALUES ('ses_bad', 'bad', '/tmp', '', 'not-a-number', 1);
