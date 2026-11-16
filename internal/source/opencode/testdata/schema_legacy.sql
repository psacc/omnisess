-- Minimal pre-v2 schema mirroring the columns omnisess reads.
CREATE TABLE session (
  id text PRIMARY KEY,
  project_id text NOT NULL,
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
  time_updated integer NOT NULL,
  data text NOT NULL
);

CREATE TABLE part (
  id text PRIMARY KEY,
  message_id text NOT NULL,
  session_id text NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  data text NOT NULL
);
