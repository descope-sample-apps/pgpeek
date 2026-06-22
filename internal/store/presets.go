package store

// DefaultPresets are seeded on first boot. Edit this list (or add your own via
// the UI) to give the support team a starting library. These are illustrative —
// adjust table/column names to match the actual schema.
var DefaultPresets = []Preset{
	{
		Name:        "Custom domains per company",
		Description: "Companies that have configured one or more custom domains, with the domain list.",
		SQL: `SELECT
    c.id              AS company_id,
    c.name            AS company_name,
    count(d.id)       AS custom_domain_count,
    string_agg(d.domain, ', ' ORDER BY d.domain) AS custom_domains
FROM companies c
JOIN custom_domains d ON d.company_id = c.id
GROUP BY c.id, c.name
ORDER BY custom_domain_count DESC, company_name
LIMIT 500;`,
	},
	{
		Name:        "Recent signups (last 7 days)",
		Description: "Users created in the last week.",
		SQL: `SELECT id, email, created_at
FROM users
WHERE created_at >= now() - interval '7 days'
ORDER BY created_at DESC
LIMIT 500;`,
	},
	{
		Name:        "Table sizes",
		Description: "Largest tables by total on-disk size — handy for spotting growth.",
		SQL: `SELECT
    schemaname,
    relname AS table,
    pg_size_pretty(pg_total_relation_size(relid)) AS total_size,
    n_live_tup AS approx_rows
FROM pg_stat_user_tables
ORDER BY pg_total_relation_size(relid) DESC
LIMIT 50;`,
	},
}
