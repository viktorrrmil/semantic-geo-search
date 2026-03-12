package services

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

// DuckDBService wraps an in-memory DuckDB instance with httpfs and spatial
// extensions pre-loaded.  It is intended to be used as a singleton.
type DuckDBService struct {
	db *sql.DB
}

var (
	duckDBInstance *DuckDBService
	duckDBOnce     sync.Once
	duckDBInitErr  error
)

// GetDuckDB returns the singleton DuckDB service, initialising it on first call.
func GetDuckDB() (*DuckDBService, error) {
	duckDBOnce.Do(func() {
		connector, err := duckdb.NewConnector("", func(execer driver.ExecerContext) error {
			ctx := context.Background()

			// Install and load extensions
			for _, q := range []string{
				"INSTALL httpfs",
				"LOAD httpfs",
				"INSTALL spatial",
				"LOAD spatial",
			} {
				if _, err := execer.ExecContext(ctx, q, nil); err != nil {
					log.Printf("DuckDB init: %q: %v", q, err)
				}
			}
			return nil
		})
		if err != nil {
			duckDBInitErr = fmt.Errorf("failed to create DuckDB connector: %w", err)
			return
		}
		duckDBInstance = &DuckDBService{db: sql.OpenDB(connector)}
	})
	return duckDBInstance, duckDBInitErr
}

// columnInfo holds the name and DuckDB type of a single column.
type columnInfo struct {
	Name string
	Type string
}

// validRegion guards the region string used in a raw SET statement.
var validRegion = regexp.MustCompile(`^[a-z0-9-]+$`)

// QueryFile reads up to k rows from the Parquet file at path.
// Geometry columns (DuckDB type GEOMETRY) are converted to WKT via ST_AsText.
// Non-geometry BLOB values are returned as hex strings.
func (s *DuckDBService) QueryFile(ctx context.Context, path, region string, k int) ([]map[string]any, error) {
	if !validRegion.MatchString(region) {
		return nil, fmt.Errorf("invalid region %q: must match [a-z0-9-]+", region)
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire DuckDB connection: %w", err)
	}
	defer conn.Close()

	for _, q := range []string{
		"SET s3_access_key_id=''",
		"SET s3_secret_access_key=''",
		fmt.Sprintf("SET s3_region='%s'", region),
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return nil, fmt.Errorf("failed to configure S3 session: %w", err)
		}
	}

	cols, err := describeParquet(ctx, conn, path)
	if err != nil {
		return nil, fmt.Errorf("failed to describe %q: %w", path, err)
	}

	selectExprs := make([]string, len(cols))
	for i, col := range cols {
		qname := quoteIdent(col.Name)
		if isGeometryType(col.Type) {
			selectExprs[i] = fmt.Sprintf("ST_AsText(%s) AS %s", qname, qname)
		} else {
			selectExprs[i] = qname
		}
	}

	query := fmt.Sprintf(
		"SELECT %s FROM read_parquet('%s') LIMIT %d",
		strings.Join(selectExprs, ", "),
		escapeSingleQuotes(path),
		k,
	)

	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query %q: %w", path, err)
	}
	defer rows.Close()

	return scanRows(rows)
}

// describeParquet returns column name/type pairs for a Parquet file.
func describeParquet(ctx context.Context, conn *sql.Conn, path string) ([]columnInfo, error) {
	q := fmt.Sprintf(
		"DESCRIBE SELECT * FROM read_parquet('%s') LIMIT 0",
		escapeSingleQuotes(path),
	)
	rows, err := conn.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []columnInfo
	for rows.Next() {
		var name, colType, null, key, def, extra sql.NullString
		if err := rows.Scan(&name, &colType, &null, &key, &def, &extra); err != nil {
			return nil, err
		}
		cols = append(cols, columnInfo{Name: name.String, Type: colType.String})
	}
	return cols, rows.Err()
}

// scanRows converts sql.Rows into a slice of generic row maps.
// All DuckDB-specific types are recursively normalised to standard Go types
// so the result can be marshalled to JSON without error.
func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = toJSONable(vals[i])
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// toJSONable recursively converts DuckDB-specific Go types into plain types
// that encoding/json can serialise without error.
//
//   - duckdb.Map  (map[any]any)  → map[string]any  (keys stringified)
//   - duckdb.UUID ([16]byte)     → UUID string "xxxxxxxx-xxxx-..."
//   - duckdb.Decimal             → float64
//   - duckdb.Interval            → map[string]any{days,months,micros}
//   - time.Time                  → RFC 3339 string
//   - []byte                     → "0x…" hex string
//   - []any                      → recursed
//   - map[string]any             → recursed
//   - everything else            → passed through (primitives are already fine)
func toJSONable(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case duckdb.Map:
		out := make(map[string]any, len(val))
		for k, mv := range val {
			out[fmt.Sprintf("%v", k)] = toJSONable(mv)
		}
		return out
	case duckdb.UUID:
		return val.String()
	case duckdb.Decimal:
		return val.Float64()
	case duckdb.Interval:
		return map[string]any{
			"months": val.Months,
			"days":   val.Days,
			"micros": val.Micros,
		}
	case time.Time:
		return val.Format(time.RFC3339)
	case []byte:
		return fmt.Sprintf("0x%x", val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = toJSONable(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, mv := range val {
			out[k] = toJSONable(mv)
		}
		return out
	default:
		return val
	}
}

// isGeometryType returns true when DuckDB reports a GEOMETRY-family column type.
func isGeometryType(t string) bool {
	upper := strings.ToUpper(t)
	return upper == "GEOMETRY" ||
		strings.HasPrefix(upper, "GEOMETRY(") ||
		upper == "POINT_2D" ||
		upper == "LINESTRING_2D" ||
		upper == "POLYGON_2D" ||
		upper == "MULTIPOINT_2D" ||
		upper == "MULTILINESTRING_2D" ||
		upper == "MULTIPOLYGON_2D"
}

// quoteIdent wraps a column name in double-quotes, escaping any embedded quotes.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// escapeSingleQuotes prevents SQL injection in literal string values.
func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
