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

CREATE TABLE IF NOT EXISTS public.audit_events (
  id          bigint GENERATED ALWAYS AS IDENTITY,
  user_id     integer     NOT NULL REFERENCES public.users(id),
  event_type  text        NOT NULL,
  occurred_at timestamptz NOT NULL
) PARTITION BY RANGE (occurred_at);

CREATE TABLE IF NOT EXISTS public.visual_edge_cases (
  id            serial PRIMARY KEY,
  case_name     text  NOT NULL,
  long_text     text  NOT NULL,
  callback_url  text  NOT NULL,
  payload       jsonb NOT NULL,
  optional_text text,
  multiline     text  NOT NULL
);

DO $$
DECLARE
  partition_month date;
BEGIN
  FOR month_number IN 0..23 LOOP
    partition_month := date '2025-01-01' + make_interval(months => month_number);
    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS %I PARTITION OF public.audit_events FOR VALUES FROM (%L::timestamptz) TO (%L::timestamptz)',
      'audit_events_' || to_char(partition_month, 'YYYY_MM'),
      to_char(partition_month, 'YYYY-MM-DD') || ' 00:00:00+00',
      to_char(partition_month + interval '1 month', 'YYYY-MM-DD') || ' 00:00:00+00'
    );
  END LOOP;
END $$;

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

INSERT INTO public.visual_edge_cases (case_name, long_text, callback_url, payload, optional_text, multiline) VALUES
  (
    'long-unbroken-token',
    'tok_' || repeat('0123456789abcdef', 16),
    'https://synthetic.example.test/callback',
    '{"kind":"token","safe":true}',
    'short',
    'single line'
  ),
  (
    'long-url',
    'Readable text around a deliberately long URL.',
    'https://a-very-long-synthetic-subdomain.example.test/oauth/callback/with/many/path/segments?state=made-up-state-value&mode=visual-check&source=local-seed',
    '{"kind":"url","environment":"local"}',
    NULL,
    'single line'
  ),
  (
    'nested-json',
    'Nested objects and arrays should wrap without escaping their cell.',
    'https://synthetic.example.test/nested',
    '{"providers":[{"kind":"saml","id":"provider-alpha"},{"kind":"oidc","id":"provider-beta"}],"settings":{"enforce":true,"labels":["alpha","beta","gamma"],"metadata":{"source":"synthetic-local-seed","revision":3}}}',
    'nested',
    'single line'
  ),
  (
    'multiline-text',
    'Text containing explicit line breaks.',
    'https://synthetic.example.test/multiline',
    '{"kind":"multiline"}',
    'line breaks',
    E'first line\nsecond line with more detail\nthird line with a final marker'
  ),
  (
    'unicode-and-direction',
    '日本語の長い表示確認テキスト 한국어 줄바꿈 확인 العربية لاختبار اتجاه النص',
    'https://synthetic.example.test/unicode',
    '{"languages":["日本語","한국어","العربية"],"synthetic":true}',
    'naïve café — Ελληνικά',
    'Mixed scripts remain readable inside one table cell.'
  ),
  (
    'empty-and-null',
    '',
    '',
    '{}',
    NULL,
    ''
  ),
  (
    'large-array',
    'A JSON array with many compact entries.',
    'https://synthetic.example.test/array',
    jsonb_build_object('items', to_jsonb(ARRAY(SELECT 'synthetic-item-' || i FROM generate_series(1, 24) AS i))),
    'array',
    'single line'
  ),
  (
    'long-json-key',
    'A long JSON key and value must both wrap.',
    'https://synthetic.example.test/json-key',
    jsonb_build_object(
      'a_deliberately_long_synthetic_configuration_key_that_contains_no_spaces_and_must_wrap',
      repeat('synthetic-value-', 20)
    ),
    repeat('optional-', 24),
    'single line'
  ),
  (
    'whitespace-only',
    E'   \t   ',
    'https://synthetic.example.test/whitespace',
    '{"kind":"whitespace","visibleLabel":"intentionally blank-looking cells"}',
    E'\t',
    E'   \n\t\n   '
  ),
  (
    'emoji-graphemes',
    repeat('界', 110) || '👩‍💻' || repeat('界', 110),
    'https://synthetic.example.test/emoji',
    '{"emoji":["👩‍💻","👨‍👩‍👧‍👦","🏳️‍🌈"],"synthetic":true}',
    'accented: Ångström, São Paulo, crème brûlée',
    'Grapheme clusters should remain intact when wrapping.'
  ),
  (
    'markup-like-text',
    '<script>alert("synthetic-only")</script> & <strong>not markup</strong>',
    'https://synthetic.example.test/?query=%3Ctag%3E&quote=%22made-up%22',
    '{"html":"<img src=x onerror=synthetic>","escaped":true}',
    'quotes: ''single'' "double" `backtick`',
    '<div>This must render as text, never as an element.</div>'
  );

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

INSERT INTO public.audit_events (user_id, event_type, occurred_at)
SELECT
  1 + (g % 45),
  (ARRAY['login','logout','profile.updated','report.exported'])[1 + (g % 4)],
  timestamptz '2025-01-01 00:00:00+00' + (g * interval '48 hours')
FROM generate_series(0, 359) AS g;
