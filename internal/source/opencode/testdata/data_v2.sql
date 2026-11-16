INSERT INTO project (id, worktree) VALUES
  ('p1', '/Users/foo/prj/demo'),
  ('p2', '/Users/foo/prj/other');

INSERT INTO session_v2 (id, project_id, title, directory, model, time_created, time_updated, parent_id) VALUES
  ('ses_alpha001', 'p1', 'Fix the build', '/Users/foo/prj/demo', '{"id":"model-a","providerID":"prov"}', 1700000000000, 1700000600000, NULL),
  ('ses_beta002', 'p2', '', '/Users/foo/prj/other', 'not-json', 1690000000000, 1690000001500, NULL),
  ('ses_gamma003', 'p1', 'No messages', '/Users/foo/prj/demo', '{"modelID":"m2"}', 1680000000000, 1680000100000, NULL),
  ('ses_delta004', 'p2', NULL, '/Users/foo/prj/other', NULL, 1670000000000, 1670000100000, NULL);

INSERT INTO session_message (id, session_id, type, seq, time_created, time_updated, data) VALUES
  ('msg_a0', 'ses_alpha001', 'user', 0, 1700000001000, 1700000001000,
   '{"time":{"created":1700000001000},"text":"Please fix the build","files":[],"agents":[]}'),
  ('msg_a1', 'ses_alpha001', 'system', 1, 1700000001500, 1700000001500,
   '{"text":"ignored system notice","time":{"created":1700000001500}}'),
  ('msg_a2', 'ses_alpha001', 'assistant', 2, 1700000002000, 1700000002500,
   '{"time":{"created":1700000002000,"completed":1700000002500},"agent":"build","model":{"id":"model-a","providerID":"prov"},"content":[{"type":"reasoning","text":"internal thinking"},{"type":"text","text":"Looking at the build."},{"type":"tool","id":"call_1","name":"read","state":{"status":"completed","input":{"filePath":"/Users/foo/prj/demo/main.go"},"output":"package main\n"}},{"type":"tool","id":"call_2","name":"grep","state":{"status":"completed","input":{"pattern":"TODO"},"content":[{"type":"text","text":"main.go:3: TODO"}]}},{"type":"tool","id":"call_3","name":"shell","state":{"status":"error","input":{"command":"false"},"output":"exit 1"}},{"type":"step-start"}]}'),
  ('msg_a3', 'ses_alpha001', 'assistant', 3, 1700000003000, 1700000003000,
   '{"time":{"created":1700000003000},"content":[{"type":"reasoning","text":"only reasoning"}]}'),
  ('msg_a4', 'ses_alpha001', 'shell', 4, 1700000004000, 1700000004000,
   '{"command":"go test","status":"exited","output":{"output":"ok"}}'),
  ('msg_a5', 'ses_alpha001', 'user', 5, 1700000005000, 1700000005000,
   '{"time":{"created":1700000005000},"text":"","files":[]}'),
  ('msg_a6', 'ses_alpha001', 'user', 6, 1700000500000, 1700000500000,
   '{"time":{"created":1700000500000},"text":"Another question about TODO items"}'),
  ('msg_b0', 'ses_beta002', 'user', 0, 1690000001000, 1690000001000,
   '{"time":{"created":1690000001000},"text":"beta question"}'),
  ('msg_b1', 'ses_beta002', 'assistant', 1, 1690000002000, 1690000002000,
   '{"time":{"created":1690000002000},"content":[{"type":"text","text":"beta answer"}]}');
