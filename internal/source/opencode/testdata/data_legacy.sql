INSERT INTO session (id, project_id, title, directory, model, time_created, time_updated) VALUES
  ('ses_old001', 'p1', 'Legacy session', '/Users/foo/prj/legacy', '{"id":"legacy-model"}', 1700000000000, 1700000600000);

INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
  ('msg_o0', 'ses_old001', 1700000001000, 1700000001000, '{"role":"user","time":{"created":1700000001000}}'),
  ('msg_o1', 'ses_old001', 1700000002000, 1700000002000, '{"role":"assistant","time":{"created":1700000002000}}'),
  ('msg_o2', 'ses_old001', 1700000003000, 1700000003000, '{"role":"system","time":{"created":1700000003000}}');

INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
  ('prt_o0_0', 'msg_o0', 'ses_old001', 1700000001000, 1700000001000, '{"type":"text","text":"legacy question"}'),
  ('prt_o1_0', 'msg_o1', 'ses_old001', 1700000002000, 1700000002000, '{"type":"text","text":"legacy answer"}'),
  ('prt_o1_1', 'msg_o1', 'ses_old001', 1700000002100, 1700000002100, '{"type":"tool","tool":"read","callID":"call_old","state":{"status":"completed","input":{"filePath":"/Users/foo/prj/legacy/main.go"},"output":"package legacy"}}'),
  ('prt_o1_2', 'msg_o1', 'ses_old001', 1700000002200, 1700000002200, '{"type":"tool","tool":"grep","callID":"call_old_err","state":{"status":"error","input":{"pattern":"x"},"content":[{"type":"text","text":"not found"}]}}'),
  ('prt_o1_3', 'msg_o1', 'ses_old001', 1700000002300, 1700000002300, '{"type":"reasoning","text":"hidden"}'),
  ('prt_o2_0', 'msg_o2', 'ses_old001', 1700000003000, 1700000003000, '{"type":"text","text":"system note"}'),
  ('prt_o1_bad', 'msg_o1', 'ses_old001', 1700000002400, 1700000002400, 'not-json');
