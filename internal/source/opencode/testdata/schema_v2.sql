-- Minimal v2 schema mirroring the columns omnisess reads.
CREATE TABLE project (
  id text PRIMARY KEY,
  worktree text NOT NULL,
  name text
);

CREATE TABLE session_v2 (
  id text PRIMARY KEY,
  project_id text NOT NULL,
  title text,
  directory text NOT NULL,
  model text,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  parent_id text
);

CREATE TABLE session_message (
  id text PRIMARY KEY,
  session_id text NOT NULL,
  type text NOT NULL,
  seq integer NOT NULL,
  time_created integer NOT NULL,
  time_updated integer NOT NULL,
  data text NOT NULL
);
