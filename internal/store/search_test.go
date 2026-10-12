package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchUsesTokenIndexInsteadOfContentLike(t *testing.T) {
	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	bodyHit, err := repo.SavePost(Post{Title: "普通标题", Slug: "body-hit", Content: "只有正文里出现独有词紫薇", Status: "published"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SavePost(Post{Title: "百分号", Slug: "percent", Content: "进度 100%", Status: "published"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SavePost(Post{Title: "不是百分号", Slug: "not-percent", Content: "进度 100X", Status: "published"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SavePost(Post{Title: "下划线", Slug: "underscore", Content: "a_b", Status: "published"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SavePost(Post{Title: "没有下划线", Slug: "no-underscore", Content: "axb", Status: "published"}); err != nil {
		t.Fatal(err)
	}

	found, err := repo.SearchPosts(PostQuery{Search: "紫薇", IncludeDrafts: true, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Items) != 1 || found.Items[0].ID != bodyHit.ID {
		t.Fatalf("body search = %#v", found.Items)
	}
	percents, err := repo.SearchPosts(PostQuery{Search: "100%", IncludeDrafts: true, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(percents.Items) != 1 || percents.Items[0].Slug != "percent" {
		t.Fatalf("percent search = %#v", percents.Items)
	}
	underscores, err := repo.SearchPosts(PostQuery{Search: "_", IncludeDrafts: true, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(underscores.Items) != 1 || underscores.Items[0].Slug != "underscore" {
		t.Fatalf("underscore search = %#v", underscores.Items)
	}

	rows, err := repo.db.Query("EXPLAIN QUERY PLAN "+searchTokenLookupSQL, "post", "紫")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(plan.String())
	if text == "" || !strings.Contains(text, "search_tokens") || strings.Contains(text, "like") || strings.Contains(text, "content") {
		t.Fatalf("query plan = %q", plan.String())
	}
}
