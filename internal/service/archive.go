package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/untesheazulchunghu-beep/typecho/internal/store"
)

func (s *Service) ListPostsByTerms(search, categorySlug, tagSlug, month, order string, includeDrafts bool, page, pageSize int) (store.PostPage, error) {
	categorySlug = strings.TrimSpace(categorySlug)
	tagSlug = strings.TrimSpace(tagSlug)
	order = strings.TrimSpace(order)
	if order != "" && order != "updated" && order != "published" {
		return store.PostPage{}, fmt.Errorf("%w: order must be updated or published", ErrValidation)
	}
	query := store.PostQuery{
		Search:        search,
		IncludeDrafts: includeDrafts,
		CategorySlug:  categorySlug,
		TagSlug:       tagSlug,
		Order:         order,
		Page:          page,
		PageSize:      pageSize,
	}
	month = strings.TrimSpace(month)
	if month != "" {
		ids, err := s.publishedIDsInMonth(month)
		if err != nil {
			return store.PostPage{}, err
		}
		query.RestrictIDs = true
		query.IDs = ids
	}
	return s.repo.SearchPosts(query)
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
	posts, err := s.repo.ListPublishedMeta()
	if err != nil {
		return nil, nil, err
	}
	var previous, next *store.Post
	for index := range posts {
		candidate := posts[index]
		if candidate.ID == post.ID {
			continue
		}
		switch comparePublished(candidate, post) {
		case -1:
			if previous == nil || comparePublished(candidate, *previous) > 0 {
				copy := candidate
				previous = &copy
			}
		case 1:
			if next == nil || comparePublished(candidate, *next) < 0 {
				copy := candidate
				next = &copy
			}
		}
	}
	return previous, next, nil
}

func (s *Service) publishedIDsInMonth(month string) ([]string, error) {
	loc, err := s.siteLocation()
	if err != nil {
		return nil, err
	}
	start, end, err := monthWindow(month, loc)
	if err != nil {
		return nil, err
	}
	posts, err := s.repo.ListPublishedMeta()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	for _, post := range posts {
		if !post.PublishedAt.Before(start) && post.PublishedAt.Before(end) {
			ids = append(ids, post.ID)
		}
	}
	return ids, nil
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
		return time.Time{}, time.Time{}, fmt.Errorf("%w: month must be YYYY-MM", ErrValidation)
	}
	return parsed, parsed.AddDate(0, 1, 0), nil
}

func comparePublished(left, right store.Post) int {
	if left.PublishedAt.Before(right.PublishedAt) {
		return -1
	}
	if left.PublishedAt.After(right.PublishedAt) {
		return 1
	}
	switch {
	case left.ID < right.ID:
		return -1
	case left.ID > right.ID:
		return 1
	default:
		return 0
	}
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
