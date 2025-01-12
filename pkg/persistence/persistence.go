package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/iansinnott/browser-gopher/pkg/config"
	"github.com/iansinnott/browser-gopher/pkg/types"
	"github.com/iansinnott/browser-gopher/pkg/util"
	"github.com/samber/lo"
)

// @note Initially visits had a unique index on `extractor_name, url_md5,
// visit_time`, however, this lead to duplicate visits. The visits were
// duplicated because some browsers will immport the history of other browsers,
// or in cases like the history trends chrome extension duplication is
// explicitly part of the goal. Thus, in order to minimize duplication visits
// are considered unique by url and unix timestamp.
const initSql = `
CREATE TABLE IF NOT EXISTS "urls" (
  "url_md5" VARCHAR(32) PRIMARY KEY NOT NULL,
  "url" TEXT UNIQUE NOT NULL,
  "title" TEXT,
  "description" TEXT,
  "last_visit" INTEGER
);

CREATE TABLE IF NOT EXISTS "urls_meta" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "url_md5" VARCHAR(32) UNIQUE NOT NULL REFERENCES urls(url_md5),
  "indexed_at" INTEGER
);

CREATE TABLE IF NOT EXISTS "visits" (
  "id" INTEGER PRIMARY KEY AUTOINCREMENT,
  "url_md5" VARCHAR(32) NOT NULL REFERENCES urls(url_md5),
  "visit_time" INTEGER,
  "extractor_name" TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS visits_unique ON visits(url_md5, visit_time);
CREATE INDEX IF NOT EXISTS visits_url_md5 ON visits(url_md5);
`

// Open a connection to the database. Calling code should close the connection when done.
// @note It is assumed that the database is already initialized. Thus this may be less useful than `InitDB`
func OpenConnection(ctx context.Context, c *config.AppConfig) (*sql.DB, error) {
	dbPath := c.DBPath
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	return conn, err
}

// Initialize the database. Create tables and indexes
func InitDb(ctx context.Context, c *config.AppConfig) (*sql.DB, error) {
	conn, err := OpenConnection(ctx, c)
	if err != nil {
		return nil, err
	}

	_, err = conn.ExecContext(ctx, initSql)

	return conn, err
}

func GetLatestTime(ctx context.Context, db *sql.DB, extractor types.Extractor) (*time.Time, error) {
	qry := `
SELECT
  visit_time
FROM
  visits
WHERE extractor_name = ?
ORDER BY
  visit_time DESC
LIMIT 1;
	`
	row := db.QueryRowContext(ctx, qry, extractor.GetName())
	if err := row.Err(); err != nil {
		return nil, err
	}

	var ts int64
	err := row.Scan(&ts)
	if err != nil {
		return nil, err
	}

	t := time.Unix(ts, 0)

	return &t, nil

}

func InsertUrl(ctx context.Context, db *sql.DB, row *types.UrlRow) error {
	const qry = `
		INSERT OR REPLACE INTO urls(url_md5, url, title, description, last_visit)
			VALUES(?, ?, ?, ?, ?);
	`
	var lastVisit int64
	if row.LastVisit != nil {
		lastVisit = row.LastVisit.Unix()
	}
	md5 := util.HashMd5String(row.Url)

	_, err := db.ExecContext(ctx, qry, md5, row.Url, row.Title, row.Description, lastVisit)
	return err
}

func InsertUrlMeta(ctx context.Context, db *sql.DB, row *types.UrlMetaRow) error {
	const qry = `
		INSERT OR REPLACE INTO urls_meta(url_md5, indexed_at)
			VALUES(?, ?);
	`
	md5 := util.HashMd5String(row.Url)
	var indexed_at int64

	if row.IndexedAt != nil {
		indexed_at = row.IndexedAt.Unix()
	}

	_, err := db.ExecContext(ctx, qry, md5, indexed_at)
	return err
}

func InsertVisit(ctx context.Context, db *sql.DB, row *types.VisitRow) error {
	const qry = `
		INSERT OR IGNORE INTO visits(url_md5, visit_time, extractor_name)
			VALUES(?, ?, ?);
	`
	md5 := util.HashMd5String(row.Url)

	_, err := db.ExecContext(ctx, qry, md5, row.Datetime.Unix(), row.ExtractorName)
	return err
}

// Count the number of urls that match the given where clause. URL meta is available in the where clause as well.
func CountUrlsWhere(ctx context.Context, db *sql.DB, where string, args ...interface{}) (int, error) {
	var qry = `
		SELECT 
			COUNT(*)
		FROM
			urls
			LEFT OUTER JOIN urls_meta ON urls.url_md5 = urls_meta.url_md5
		WHERE %s;
	`
	qry = fmt.Sprintf(qry, where)
	row := db.QueryRowContext(ctx, qry, args...)
	if err := row.Err(); err != nil {
		return 0, err
	}

	var count int
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func UrlsById(ctx context.Context, db *sql.DB, ids ...string) ([]types.UrlDbEntity, error) {
	qry := fmt.Sprintf(
		`SELECT 
				url_md5,
				url,
				title,
				description,
				last_visit
			FROM 
				urls 
			WHERE 
				url_md5 IN (%s);
		`,
		strings.Join(
			lo.Map(ids, func(id string, _ int) string { return "?" }),
			",",
		),
	)

	// C'mon Go, don't expose your implementation details (this conversion is
	// necessary becuase of underlying mem representation):
	// https://go.dev/doc/faq#convert_slice_of_interface
	var args []any
	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := db.QueryContext(ctx, qry, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []types.UrlDbEntity
	for rows.Next() {
		var url types.UrlDbEntity
		var ts int64

		err := rows.Scan(&url.UrlMd5, &url.Url, &url.Title, &url.Description, &ts)
		if err != nil {
			return nil, err
		}

		if ts != 0 {
			t := time.Unix(ts, 0)
			url.LastVisit = &t
		}

		urls = append(urls, url)
	}

	return urls, nil
}
