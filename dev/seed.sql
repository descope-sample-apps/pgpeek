-- Demo data for the local `docker compose` dev stack.
--
-- The official Postgres image runs every file in /docker-entrypoint-initdb.d/
-- exactly once, when the data directory is first initialised (i.e. on a fresh
-- volume). compose.yaml mounts this file there, so `docker compose up` gives
-- pgpeek something realistic to browse out of the box: two schemas, foreign
-- keys (for click-through), and enough rows to page through.
--
-- Already ran compose before this file existed? The seed only fires on an empty
-- data dir, so reset the volume first:  docker compose down -v && docker compose up

CREATE TABLE IF NOT EXISTS public.companies (
  id         serial PRIMARY KEY,
  name       text        NOT NULL,
  plan       text        NOT NULL DEFAULT 'free',
  seats      int         NOT NULL DEFAULT 5,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.users (
  id         serial PRIMARY KEY,
  email      text        NOT NULL,
  full_name  text,
  company_id integer     NOT NULL REFERENCES public.companies(id),
  is_active  boolean     NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE SCHEMA IF NOT EXISTS auth;
CREATE TABLE IF NOT EXISTS auth.sessions (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    integer     NOT NULL REFERENCES public.users(id),
  ip         inet,
  user_agent text,
  expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO public.companies (name, plan, seats) VALUES
  ('Acme Inc',            'enterprise', 250),
  ('Globex',              'pro',         40),
  ('Initech',             'free',         5),
  ('Umbrella Corp',       'enterprise', 500),
  ('Hooli',               'pro',         75),
  ('Stark Industries',    'enterprise', 1200),
  ('Wayne Enterprises',   'enterprise', 900),
  ('Cyberdyne Systems',   'pro',         60),
  ('Soylent Corp',        'free',         8),
  ('Wonka Industries',    'pro',         30),
  ('Vandelay Industries', 'free',         3),
  ('Pied Piper',          'pro',         12),
  ('Massive Dynamic',     'enterprise', 340),
  ('Tyrell Corp',         'pro',         88);

INSERT INTO public.users (email, full_name, company_id, is_active)
SELECT
  'user' || g || '@' || lower(replace(c.name, ' ', '')) || '.test',
  (ARRAY['Ada Lovelace','Alan Turing','Grace Hopper','Linus Torvalds','Margaret Hamilton',
         'Dennis Ritchie','Ken Thompson','Barbara Liskov','Edsger Dijkstra','Donald Knuth'])[1 + (g % 10)],
  c.id,
  (g % 7 <> 0)
FROM generate_series(1, 45) AS g
JOIN public.companies c ON c.id = 1 + (g % 14);

INSERT INTO auth.sessions (user_id, ip, user_agent, expires_at)
SELECT
  1 + (g % 45),
  ('192.0.2.' || (1 + (g % 254)))::inet,
  (ARRAY['Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)',
         'Mozilla/5.0 (Windows NT 10.0; Win64; x64)',
         'Mozilla/5.0 (X11; Linux x86_64)',
         'curl/8.4.0'])[1 + (g % 4)],
  now() + (g || ' hours')::interval
FROM generate_series(1, 25) AS g;

DO $$
DECLARE
  extra_cols text;
BEGIN
  SELECT string_agg(format('field_%s text NOT NULL', lpad(i::text, 2, '0')), ', ')
  INTO extra_cols
  FROM generate_series(1, 46) AS s(i);

  EXECUTE 'CREATE TABLE IF NOT EXISTS public.wide_support_events (
    id serial PRIMARY KEY,
    subject text NOT NULL,
    notes text NOT NULL,
    payload jsonb NOT NULL, ' || extra_cols || '
  )';
END $$;

DO $$
DECLARE
  extra_names text;
  extra_values text;
BEGIN
  SELECT string_agg(format('field_%s', lpad(i::text, 2, '0')), ', ')
  INTO extra_names
  FROM generate_series(1, 46) AS s(i);

  SELECT string_agg(format(
    '(''field_%1$s row '' || g || '' — long diagnostic text with searchable token renewal-blocked-%1$s and JSON pointer $.events[%2$s].detail. '' || repeat(''trace context '', 8))',
    lpad(i::text, 2, '0'),
    i
  ), ', ')
  INTO extra_values
  FROM generate_series(1, 46) AS s(i);

  EXECUTE 'INSERT INTO public.wide_support_events (subject, notes, payload, ' || extra_names || ')
  SELECT
    ''Support investigation '' || g,
    ''Long support note for a wide table row. The important filtered phrase renewal-blocked-needle sits near the end so truncated cells used to hide it. '' || repeat(''Customer timeline, entitlement metadata, admin comments, and audit evidence. '', 5),
    jsonb_build_object(
      ''ticketId'', ''SUP-'' || to_char(g, ''FM0000''),
      ''filterExample'', ''renewal-blocked-needle'',
      ''columns'', 50,
      ''events'', jsonb_build_array(
        jsonb_build_object(''kind'', ''email'', ''body'', repeat(''long email body '', 8)),
        jsonb_build_object(''kind'', ''webhook'', ''payload'', jsonb_build_object(''plan'', ''enterprise'', ''blocked'', true, ''reason'', ''renewal-blocked-needle''))
      )
    ),
    ' || extra_values || '
  FROM generate_series(1, 18) AS g';
END $$;
