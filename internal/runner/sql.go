package runner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/kevnnard/devtrainer/internal/check"
)

// runSQL runs p.Setup then p.Code against a fresh in-memory database and
// returns the rows of Code's last statement that returned rows.
func (r *Runner) runSQL(ctx context.Context, p Program) (check.CodeRun, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return check.CodeRun{}, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1) // every connection to :memory: is its own database

	var run check.CodeRun
	fail := func(err error) (check.CodeRun, error) {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			run.TimedOut = true
			return run, nil
		}
		run.Error = err.Error()
		return run, nil
	}
	if strings.TrimSpace(p.Setup) != "" {
		if _, err := db.ExecContext(ctx, p.Setup); err != nil {
			return run, fmt.Errorf("runner: the exercise setup failed: %w", err)
		}
	}
	if strings.TrimSpace(p.Code) == "" {
		run.Error = "no SQL to run"
		return run, nil
	}
	rows, err := db.QueryContext(ctx, p.Code)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	if run.Columns, err = rows.Columns(); err != nil {
		return fail(err)
	}
	budget := r.maxOutput()
	vals := make([]any, len(run.Columns))
	ptrs := make([]any, len(vals))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if len(run.Rows) >= MaxRows {
			run.Truncated = true
			return run, nil
		}
		if err := rows.Scan(ptrs...); err != nil {
			return fail(err)
		}
		row := make([]string, len(vals))
		for i, v := range vals {
			row[i] = Text(v)
			budget -= len(row[i]) + 3
		}
		run.Rows = append(run.Rows, row)
		if budget < 0 {
			run.Truncated = true
			return run, nil
		}
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	if run.Rows == nil {
		run.Rows = [][]string{}
	}
	return run, nil
}

// Text renders one SQLite value the way the sqlite3 shell does: NULL,
// integers in decimal, reals with at least one decimal (3.0, 2.5), text
// and blobs as is.
func Text(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		s := strconv.FormatFloat(x, 'g', 15, 64)
		if !strings.ContainsAny(s, ".eEnN") {
			s += ".0"
		}
		return s
	case []byte:
		return string(x)
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return "0"
	case time.Time:
		if x.Hour() == 0 && x.Minute() == 0 && x.Second() == 0 && x.Nanosecond() == 0 {
			return x.Format("2006-01-02")
		}
		return x.Format("2006-01-02 15:04:05")
	}
	return fmt.Sprint(v)
}
