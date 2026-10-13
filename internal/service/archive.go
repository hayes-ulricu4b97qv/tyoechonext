package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

func (s *Service) ListPostsByTerms(search, categorySlug, tagSlug, month, order string, includeDrafts bool, status string, page, pageSize int) (store.PostPage, error) {
	search, err := normalizeSearchQuery(search)
	if err != nil {
		return store.PostPage{}, err
	}
	categorySlug = strings.TrimSpace(categorySlug)
	tagSlug = strings.TrimSpace(tagSlug)
	order = strings.TrimSpace(order)
	status = strings.TrimSpace(status)
	if order != "" && order != "updated" && order != "published" {
		return store.PostPage{}, fmt.Errorf("%w: 排序只能是 updated 或 published", ErrValidation)
	}
	if status != "" && status != "draft" && status != "published" {
		return store.PostPage{}, fmt.Errorf("%w: 文章状态只能是 draft 或 published", ErrValidation)
	}
	query := store.PostQuery{
		Search:        search,
		IncludeDrafts: includeDrafts,
		Status:        status,
		CategorySlug:  categorySlug,
		TagSlug:       tagSlug,
		Order:         order,
		Page:          page,
		PageSize:      pageSize,
	}
	month = strings.TrimSpace(month)
	if month != "" {
		location, err := s.siteLocation()
		if err != nil {
			return store.PostPage{}, err
		}
		query.PublishedFrom, query.PublishedBefore, err = monthWindow(month, location)
		if err != nil {
			return store.PostPage{}, err
		}
	}
	return s.repo.SearchPosts(query)
}

func FeedBlurb(excerpt, content string) string {
	if text := strings.TrimSpace(excerpt); text != "" {
		return text
	}
	runes := []rune(content)
	const limit = 280
	if len(runes) <= limit {
		return content
	}
	return string(runes[:limit]) + "…"
}

func (s *Service) MonthArchive() ([]store.MonthCount, error) {
	loc, err := s.siteLocation()
	if err != nil {
		return nil, err
	}
	posts, err := s.repo.ListPublishedMeta()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, post := range posts {
		counts[post.PublishedAt.In(loc).Format("2006-01")]++
	}
	items := make([]store.MonthCount, 0, len(counts))
	for month, count := range counts {
		items = append(items, store.MonthCount{Month: month, Count: count})
	}
	sortMonths(items)
	return items, nil
}

func (s *Service) PublishedNeighbors(post store.Post) (*store.Post, *store.Post, error) {
	if post.Status != "published" || post.PublishedAt.IsZero() {
		return nil, nil, nil
	}
	return s.repo.PublishedNeighbors(post)
}

func (s *Service) siteLocation() (*time.Location, error) {
	site, err := s.repo.Site()
	if err != nil {
		return nil, err
	}
	return locationOrUTC(site.Timezone), nil
}

func locationOrUTC(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

func monthWindow(month string, loc *time.Location) (time.Time, time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	parsed, err := time.ParseInLocation("2006-01", month, loc)
	if err != nil || parsed.Format("2006-01") != month {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: 月份格式必须是 YYYY-MM", ErrValidation)
	}
	return parsed, parsed.AddDate(0, 1, 0), nil
}

func sortMonths(items []store.MonthCount) {
	for index := 1; index < len(items); index++ {
		current := items[index]
		place := index
		for place > 0 && items[place-1].Month < current.Month {
			items[place] = items[place-1]
			place--
		}
		items[place] = current
	}
}
