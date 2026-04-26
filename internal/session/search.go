package session

import (
	"strings"
)

// Search performs full-text search across message content.
func (s *SQLiteStore) Search(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	q := sanitizeFTS5(query)
	if q == "" {
		return nil, nil
	}

	rows, err := s.db.Query(
		`SELECT m.session_id, m.role, m.content, m.timestamp
		 FROM messages_fts fts
		 JOIN messages m ON m.id = fts.rowid
		 WHERE messages_fts MATCH ?
		 ORDER BY rank
		 LIMIT ?`, q, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var sr SearchResult
		if err := rows.Scan(&sr.SessionID, &sr.Role, &sr.Content, &sr.Timestamp); err != nil {
			return nil, err
		}
		results = append(results, sr)
	}
	return results, nil
}

// sanitizeFTS5 strips characters that break FTS5 queries.
func sanitizeFTS5(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	// Remove special FTS5 characters
	replacer := strings.NewReplacer(
		"*", "",
		"^", "",
		"\"", "",
		"-", " ",
		"(", "",
		")", "",
		"AND", "",
		"OR", "",
		"NOT", "",
		"NEAR", "",
	)
	query = replacer.Replace(query)
	words := strings.Fields(query)
	if len(words) == 0 {
		return ""
	}
	// Quote each term for exact matching
	for i, w := range words {
		words[i] = `"` + w + `"`
	}
	return strings.Join(words, " ")
}
