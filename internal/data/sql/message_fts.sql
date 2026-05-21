CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
  message_id UNINDEXED,
  session_id UNINDEXED,
  content_markdown,
  tokenize = 'unicode61'
);

INSERT INTO messages_fts(message_id, session_id, content_markdown)
SELECT id, session_id, content_markdown FROM messages
WHERE id NOT IN (SELECT message_id FROM messages_fts);

CREATE TRIGGER IF NOT EXISTS messages_fts_ai AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts(message_id, session_id, content_markdown)
  VALUES (new.id, new.session_id, new.content_markdown);
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_ad AFTER DELETE ON messages BEGIN
  DELETE FROM messages_fts WHERE message_id = old.id;
END;

CREATE TRIGGER IF NOT EXISTS messages_fts_au AFTER UPDATE ON messages BEGIN
  DELETE FROM messages_fts WHERE message_id = old.id;
  INSERT INTO messages_fts(message_id, session_id, content_markdown)
  VALUES (new.id, new.session_id, new.content_markdown);
END;
