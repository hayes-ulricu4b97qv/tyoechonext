package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	rows, err := repo.db.Query("EXPLAIN QUERY PLAN "+searchTokenCandidatesSQL(3), "post", "紫", "紫薇", "薇", 3)
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

func TestSearchPostsSkipsContentUnlessRequested(t *testing.T) {
	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	body := strings.Repeat("长正文", 5000)
	created, err := repo.SavePost(Post{Title: "列表摘要", Slug: "summary", Excerpt: "可公开的摘要", Content: body, Status: "published"})
	if err != nil {
		t.Fatal(err)
	}

	listed, err := repo.SearchPosts(PostQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != created.ID || listed.Items[0].Content != "" {
		t.Fatalf("summary query returned body data: %#v", listed.Items)
	}

	feed, err := repo.SearchPosts(PostQuery{IncludeContent: true, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Items) != 1 || feed.Items[0].Content != body {
		t.Fatalf("explicit content query returned %d runes", len(feed.Items[0].Content))
	}
}

func TestPostListBatchPopulatesTermsAndVisibleCounts(t *testing.T) {
	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	now := time.Now().UTC()
	posts := []Post{
		{Title: "Public", Slug: "batch-public", Content: "body", Status: "published", PublishedAt: now.Add(-time.Hour)},
		{Title: "Scheduled", Slug: "batch-scheduled", Content: "body", Status: "published", PublishedAt: now.Add(time.Hour)},
		{Title: "Draft", Slug: "batch-draft", Content: "body", Status: "draft"},
	}
	for index := range posts {
		posts[index].Categories = []Term{{Name: "Batch category", Kind: "category"}}
		posts[index].Tags = []Term{{Name: "Batch tag", Kind: "tag"}}
		if _, err := repo.SavePost(posts[index]); err != nil {
			t.Fatal(err)
		}
	}

	listed, err := repo.SearchPosts(PostQuery{IncludeDrafts: true, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != len(posts) {
		t.Fatalf("listed posts = %d, want %d", len(listed.Items), len(posts))
	}
	for _, post := range listed.Items {
		if len(post.Categories) != 1 || post.Categories[0].Name != "Batch category" || len(post.Tags) != 1 || post.Tags[0].Name != "Batch tag" {
			t.Fatalf("terms for %q = categories %#v, tags %#v", post.Slug, post.Categories, post.Tags)
		}
		if post.Categories[0].PostCount != 1 || post.Tags[0].PostCount != 1 {
			t.Fatalf("visible counts for %q = category %d, tag %d, want 1 each", post.Slug, post.Categories[0].PostCount, post.Tags[0].PostCount)
		}
	}

	all, err := repo.Posts()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(posts) {
		t.Fatalf("all posts = %d, want %d", len(all), len(posts))
	}
	for _, post := range all {
		if len(post.Categories) != 1 || len(post.Tags) != 1 || post.Categories[0].PostCount != 1 || post.Tags[0].PostCount != 1 {
			t.Fatalf("batched post terms for %q = %#v / %#v", post.Slug, post.Categories, post.Tags)
		}
	}
}

func TestRevisionSnapshotsDetectChangedEmptyCustomFieldKeys(t *testing.T) {
	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	post, err := repo.SavePost(Post{Title: "Fields", Slug: "empty-field-key", Content: "Body", CustomFields: map[string]string{"old": ""}})
	if err != nil {
		t.Fatal(err)
	}
	post.CustomFields = map[string]string{"new": ""}
	if _, err := repo.SavePost(post); err != nil {
		t.Fatal(err)
	}
	revisions, err := repo.ListPostRevisions(post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 || len(revisions[0].CustomFields) != 1 || revisions[0].CustomFields["old"] != "" {
		t.Fatalf("changed empty-valued field keys were not snapshotted: %#v", revisions)
	}
}
