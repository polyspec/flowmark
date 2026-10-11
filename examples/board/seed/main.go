// Command seed fills a board corpus with posts through the engine: the same number of posts in each month of a range.
// The corpus is a corpus root with the board catalog and its people (examples/board/block or examples/board/files,
// without their posts). Every post is a row.insert of the operation layer, the way a face writes one, so the corpus
// stays valid and canonical. The authors are the people the corpus holds, in turn.
//
//	go run ./examples/board/seed <corpus root> <year> <first month> <last month> <posts per month>
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/polyspec/flowmark/ops"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 5 {
		return fmt.Errorf("usage: seed <corpus root> <year> <first month> <last month> <posts per month>")
	}
	var num [4]int
	for i, name := range []string{"year", "first month", "last month", "posts per month"} {
		n, err := strconv.Atoi(args[i+1])
		if err != nil || n < 1 {
			return fmt.Errorf("%s %q is not a positive number", name, args[i+1])
		}
		num[i] = n
	}
	year, first, last, perMonth := num[0], num[1], num[2], num[3]
	if first > 12 || last > 12 || first > last {
		return fmt.Errorf("the months %d to %d are not a range of a year", first, last)
	}
	ctx := context.Background()
	c, err := ops.Open(ctx, args[0], 0)
	if err != nil {
		return err
	}
	defer c.Close()
	authors, err := keysOf(ctx, c, "people")
	if err != nil {
		return err
	}
	if len(authors) == 0 {
		return fmt.Errorf("the corpus holds no person to write the posts")
	}
	now := time.Now().UTC()
	n := 0
	for month := first; month <= last; month++ {
		for i := 0; i < perMonth; i++ {
			n++
			members := map[string]any{"table": "posts", "values": map[string]any{
				"title":   fmt.Sprintf("%d월의 글 %d", month, i+1),
				"body":    fmt.Sprintf("%d월 %d번째 글입니다.\n\n전체 %d번째 게시글이고, 이 달에 %d개 중 하나입니다.", month, i+1, n, perMonth),
				"author":  authors[n%len(authors)],
				"created": created(year, month, i, perMonth, now),
			}}
			if err := insert(ctx, c, members); err != nil {
				return fmt.Errorf("post %d of month %d: %w", i+1, month, err)
			}
		}
	}
	fmt.Printf("seeded %d posts, %d in each of the months %d to %d of %d\n", n, perMonth, first, last, year)
	return nil
}

// created returns the time of the i-th of perMonth posts of a month in UTC: the month, from its first instant to its
// last or to now when it has not finished, cut into perMonth equal spans, one post at the start of each, in whole
// seconds. No post is dated after now.
func created(year, month, i, perMonth int, now time.Time) string {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	if end.After(now) {
		end = now
	}
	t := start.Add(end.Sub(start) / time.Duration(perMonth) * time.Duration(i))
	return t.Truncate(time.Second).Format("2006-01-02 15:04:05.000000")
}

// keysOf returns the keys of the rows of table, in the order the corpus reads them in.
func keysOf(ctx context.Context, c *ops.Corpus, table string) ([]string, error) {
	r, err := ops.Parse("rows", map[string]json.RawMessage{"table": json.RawMessage(strconv.Quote(table))})
	if err != nil {
		return nil, err
	}
	a, err := c.Run(ctx, "seed", r)
	if err != nil {
		return nil, err
	}
	b, err := ops.JSON(a)
	if err != nil {
		return nil, err
	}
	tb, ok := c.Catalog().Table(table)
	if !ok {
		return nil, fmt.Errorf("the corpus has no table %s", table)
	}
	var rows struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows.Rows))
	for _, row := range rows.Rows {
		key, ok := row[tb.Key].(string)
		if !ok {
			return nil, fmt.Errorf("a row of %s has no text key %s", table, tb.Key)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// insert runs one row.insert.
func insert(ctx context.Context, c *ops.Corpus, members map[string]any) error {
	raw := map[string]json.RawMessage{}
	for k, v := range members {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		raw[k] = b
	}
	r, err := ops.Parse("row.insert", raw)
	if err != nil {
		return err
	}
	_, err = c.Run(ctx, "seed", r)
	return err
}
