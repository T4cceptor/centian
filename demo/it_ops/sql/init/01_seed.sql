CREATE TABLE IF NOT EXISTS tickets (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  priority TEXT NOT NULL,
  status TEXT NOT NULL
);

INSERT INTO tickets (id, title, priority, status)
VALUES ('IT-1042', 'Checkout service returns intermittent 503 responses', 'P2', 'investigating')
ON CONFLICT (id) DO NOTHING;
