package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"hub/server/internal/db"
)

// schemaTables lists the tables of the current schema with their stored columns, in an
// order where each comes after those its foreign keys point to. schema_migrations stays
// out: a restore writes it by migrating.
func schemaTables(ctx context.Context, q pgx.Tx) ([]Table, error) {
	rows, err := q.Query(ctx, `
		SELECT c.relname, array_agg(a.attname::text ORDER BY a.attnum)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped AND a.attgenerated = ''
		WHERE n.nspname = current_schema() AND c.relkind = 'r' AND c.relname <> 'schema_migrations'
		GROUP BY c.relname`)
	if err != nil {
		return nil, err
	}
	columns := map[string][]string{}
	for rows.Next() {
		var name string
		var cols []string
		if err := rows.Scan(&name, &cols); err != nil {
			return nil, err
		}
		columns[name] = cols
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = q.Query(ctx, `
		SELECT t.relname, r.relname
		FROM pg_constraint k
		JOIN pg_class t ON t.oid = k.conrelid
		JOIN pg_class r ON r.oid = k.confrelid
		JOIN pg_namespace n ON n.oid = k.connamespace
		WHERE k.contype = 'f' AND n.nspname = current_schema() AND t.oid <> r.oid`)
	if err != nil {
		return nil, err
	}
	needs := map[string][]string{}
	for rows.Next() {
		var table, ref string
		if err := rows.Scan(&table, &ref); err != nil {
			return nil, err
		}
		needs[table] = append(needs[table], ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Depth-first, by name for a stable order.
	var out []Table
	done := map[string]bool{}
	visiting := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if done[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("clés étrangères en boucle autour de %s", name)
		}
		visiting[name] = true
		refs := needs[name]
		slices.Sort(refs)
		for _, ref := range refs {
			if err := visit(ref); err != nil {
				return err
			}
		}
		visiting[name], done[name] = false, true
		out = append(out, Table{Name: name, Columns: columns[name]})
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(columns)) {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func copySQL(t Table, dir string) string {
	cols := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		cols[i] = pgx.Identifier{c}.Sanitize()
	}
	return fmt.Sprintf("COPY %s (%s) %s", pgx.Identifier{t.Name}.Sanitize(), strings.Join(cols, ", "), dir)
}

// Dump writes the manifest and every table, all read from one snapshot. The keys go in
// between, so a restore has them before it touches the database.
func Dump(ctx context.Context, pool *pgxpool.Pool, w *Writer, m Manifest, keys map[string][]byte) (Manifest, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return m, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if m.Migration, err = db.Applied(ctx, tx); err != nil {
		return m, err
	}
	tables, err := schemaTables(ctx, tx)
	if err != nil {
		return m, err
	}
	// Each table in memory before its tar entry, which needs its size: Hub's data is small.
	bodies := make([][]byte, len(tables))
	for i := range tables {
		var buf bytes.Buffer
		tag, err := tx.Conn().PgConn().CopyTo(ctx, &buf, copySQL(tables[i], "TO STDOUT"))
		if err != nil {
			return m, fmt.Errorf("table %s : %w", tables[i].Name, err)
		}
		tables[i].Rows = tag.RowsAffected()
		bodies[i] = buf.Bytes()
	}
	m.Tables = tables
	m.Keys = slices.Sorted(maps.Keys(keys))

	if err := w.Manifest(m); err != nil {
		return m, err
	}
	for _, role := range m.Keys {
		if err := w.Key(role, keys[role]); err != nil {
			return m, err
		}
	}
	for i, t := range tables {
		if err := w.Table(t.Name, bodies[i]); err != nil {
			return m, err
		}
	}
	return m, nil
}

// ErrNotEmpty is returned when a restore finds a database already in use.
var ErrNotEmpty = errors.New("la base n'est pas vide : restaurer sur des volumes neufs")

// Restore rebuilds the schema the backup was made on, loads its rows, then applies the
// migrations made since. It refuses a database that has any migration applied. keys
// receives each key file, before the database is touched.
func Restore(ctx context.Context, pool *pgxpool.Pool, r *Reader, keys func(role string, body []byte) error) error {
	latest, err := db.Latest()
	if err != nil {
		return err
	}
	if r.Manifest.Migration > latest {
		return fmt.Errorf("sauvegarde faite par une version plus récente de Hub (migration %d, celle-ci connaît %d)", r.Manifest.Migration, latest)
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrNotEmpty
	}

	// Keys first, in the order written; then the tables.
	name, body, err := r.Next()
	for err == nil && strings.HasPrefix(name, "keys/") {
		if err := keys(strings.TrimPrefix(name, "keys/"), body); err != nil {
			return err
		}
		name, body, err = r.Next()
	}
	if err != nil {
		return fmt.Errorf("lecture de la sauvegarde : %w", err)
	}

	if err := db.MigrateTo(ctx, pool, r.Manifest.Migration); err != nil {
		return err
	}
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		tables, err := schemaTables(ctx, tx)
		if err != nil {
			return err
		}
		if err := sameTables(tables, r.Manifest.Tables); err != nil {
			return err
		}
		// A migration may have written rows of its own: the backup's replace them.
		names := make([]string, len(tables))
		for i, t := range tables {
			names[i] = pgx.Identifier{t.Name}.Sanitize()
		}
		if _, err := tx.Exec(ctx, "TRUNCATE "+strings.Join(names, ", ")); err != nil {
			return err
		}
		for i, t := range r.Manifest.Tables {
			if i > 0 {
				if name, body, err = r.Next(); err != nil {
					return fmt.Errorf("lecture de la sauvegarde : %w", err)
				}
			}
			if name != "db/"+t.Name {
				return fmt.Errorf("attendu db/%s, trouvé %s", t.Name, name)
			}
			tag, err := tx.Conn().PgConn().CopyFrom(ctx, bytes.NewReader(body), copySQL(t, "FROM STDIN"))
			if err != nil {
				return fmt.Errorf("table %s : %w", t.Name, err)
			}
			if tag.RowsAffected() != t.Rows {
				return fmt.Errorf("table %s : %d lignes au lieu de %d", t.Name, tag.RowsAffected(), t.Rows)
			}
		}
		return resetSequences(ctx, tx)
	})
	if err != nil {
		return err
	}
	return db.Migrate(ctx, pool)
}

func sameTables(have, want []Table) error {
	cols := map[string]string{}
	for _, t := range have {
		cols[t.Name] = strings.Join(t.Columns, ",")
	}
	if len(have) != len(want) {
		return fmt.Errorf("%d tables dans la sauvegarde, %d dans le schéma", len(want), len(have))
	}
	for _, t := range want {
		c, ok := cols[t.Name]
		if !ok {
			return fmt.Errorf("table %s absente du schéma", t.Name)
		}
		if c != strings.Join(t.Columns, ",") {
			return fmt.Errorf("table %s : colonnes différentes", t.Name)
		}
	}
	return nil
}

// resetSequences moves each identity or serial sequence past the rows loaded.
func resetSequences(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
		SELECT c.relname, a.attname::text, s
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		CROSS JOIN LATERAL pg_get_serial_sequence(quote_ident(c.relname), a.attname) AS s
		WHERE n.nspname = current_schema() AND c.relkind = 'r' AND s IS NOT NULL`)
	if err != nil {
		return err
	}
	type seq struct{ table, column, name string }
	var list []seq
	for rows.Next() {
		var s seq
		if err := rows.Scan(&s.table, &s.column, &s.name); err != nil {
			return err
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, s := range list {
		q := fmt.Sprintf("SELECT setval($1, coalesce((SELECT max(%s) FROM %s), 0) + 1, false)",
			pgx.Identifier{s.column}.Sanitize(), pgx.Identifier{s.table}.Sanitize())
		if _, err := tx.Exec(ctx, q, s.name); err != nil {
			return fmt.Errorf("séquence %s : %w", s.name, err)
		}
	}
	return nil
}
