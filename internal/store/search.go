package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// searchTokenLookupSQL is the indexed lookup used by post and page search.
// It does not scan the content column with LIKE.
const searchTokenLookupSQL = `SELECT record_id FROM search_tokens WHERE kind = ? AND token = ?`

func searchTokens(parts ...string) []string {
	seen := make(map[string]struct{})
	tokens := make([]string, 0)
	add := func(token string) {
		if token == "" {
			return
		}
		if _, ok := seen[token]; ok {
			return
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	for _, part := range parts {
		runes := []rune(strings.ToLower(part))
		for index := range runes {
			add(string(runes[index : index+1]))
			if index+1 < len(runes) {
				add(string(runes[index : index+2]))
			}
		}
	}
	return tokens
}

func containsLiteral(parts []string, search string) bool {
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return true
	}
	var builder strings.Builder
	for _, part := range parts {
		builder.WriteString(strings.ToLower(part))
		builder.WriteByte('\n')
	}
	return strings.Contains(builder.String(), needle)
}

func replaceSearchTokens(tx *sql.Tx, kind, id string, parts ...string) error {
	if _, err := tx.Exec(`DELETE FROM search_tokens WHERE kind = ? AND record_id = ?`, kind, id); err != nil {
		return fmt.Errorf("clear search tokens: %w", err)
	}
	statement, err := tx.Prepare(`INSERT INTO search_tokens(kind, token, record_id) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare search tokens: %w", err)
	}
	defer statement.Close()
	for _, token := range searchTokens(parts...) {
		if _, err := statement.Exec(kind, token, id); err != nil {
			return fmt.Errorf("write search token: %w", err)
		}
	}
	return nil
}

func deleteSearchTokens(tx *sql.Tx, kind, id string) error {
	if _, err := tx.Exec(`DELETE FROM search_tokens WHERE kind = ? AND record_id = ?`, kind, id); err != nil {
		return fmt.Errorf("delete search tokens: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) recordIDsMatching(kind, search string) ([]string, error) {
	search = strings.TrimSpace(search)
	if search == "" {
		return nil, nil
	}
	tokens := searchTokens(search)
	if len(tokens) == 0 {
		return []string{}, nil
	}
	var matched map[string]struct{}
	for _, token := range tokens {
		rows, err := r.db.Query(searchTokenLookupSQL, kind, token)
		if err != nil {
			return nil, fmt.Errorf("search tokens: %w", err)
		}
		current := make(map[string]struct{})
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan search token: %w", err)
			}
			current[id] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		if matched == nil {
			matched = current
			continue
		}
		for id := range matched {
			if _, ok := current[id]; !ok {
				delete(matched, id)
			}
		}
	}
	if len(matched) == 0 {
		return []string{}, nil
	}
	ids := make([]string, 0, len(matched))
	for id := range matched {
		ids = append(ids, id)
	}
	verified, err := r.filterLiteralMatches(kind, ids, search)
	if err != nil {
		return nil, err
	}
	return verified, nil
}

func (r *SQLiteRepository) filterLiteralMatches(kind string, ids []string, search string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	table := "posts"
	if kind == "page" {
		table = "pages"
	}
	clause, args := idInClause(ids)
	query := `SELECT id, title, excerpt, slug, content FROM ` + table + ` WHERE ` + clause
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("read search candidates: %w", err)
	}
	defer rows.Close()
	matched := make([]string, 0)
	for rows.Next() {
		var id, title, excerpt, slug, content string
		if err := rows.Scan(&id, &title, &excerpt, &slug, &content); err != nil {
			return nil, fmt.Errorf("scan search candidate: %w", err)
		}
		if containsLiteral([]string{title, excerpt, slug, content}, search) {
			matched = append(matched, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return matched, nil
}
