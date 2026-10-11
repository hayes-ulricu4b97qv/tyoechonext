package store

import (
	"database/sql"
	"errors"
	"time"
)

var ErrNotFound = errors.New("record not found")

type Site struct {
	Name                      string    `json:"name"`
	URL                       string    `json:"url"`
	AdminMail                 string    `json:"adminMail"`
	Timezone                  string    `json:"timezone"`
	Description               string    `json:"description"`
	Keywords                  string    `json:"keywords"`
	PostsPerPage              int       `json:"postsPerPage"`
	CommentsEnabled           bool      `json:"commentsEnabled"`
	CommentsRequireModeration bool      `json:"commentsRequireModeration"`
	CommentsThreaded          bool      `json:"commentsThreaded"`
	CreatedAt                 time.Time `json:"createdAt"`
	UpdatedAt                 time.Time `json:"updatedAt"`
}

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"passwordHash"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Admin is the owner account stored in the users table.
type Admin = User

type Post struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Slug         string    `json:"slug"`
	Excerpt      string    `json:"excerpt"`
	Content      string    `json:"content"`
	Status       string    `json:"status"`
	AllowComment bool      `json:"allowComment"`
	Categories   []Term    `json:"categories,omitempty"`
	Tags         []Term    `json:"tags,omitempty"`
	PublishedAt  time.Time `json:"publishedAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Page struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	Excerpt     string    `json:"excerpt"`
	Content     string    `json:"content"`
	Status      string    `json:"status"`
	PublishedAt time.Time `json:"publishedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Term struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Kind      string    `json:"kind"`
	PostCount int       `json:"postCount"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type TermQuery struct {
	Kind          string
	Search        string
	IncludeDrafts bool
	Page          int
	PageSize      int
}

type TermPage struct {
	Items    []Term `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type Comment struct {
	ID          string    `json:"id"`
	PostID      string    `json:"postId"`
	ParentID    string    `json:"parentId,omitempty"`
	PostTitle   string    `json:"postTitle,omitempty"`
	PostSlug    string    `json:"postSlug,omitempty"`
	AuthorName  string    `json:"authorName"`
	AuthorEmail string    `json:"authorEmail,omitempty"`
	AuthorURL   string    `json:"authorUrl,omitempty"`
	Content     string    `json:"content"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Media struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	Size        int64     `json:"size"`
	StorageKey  string    `json:"storageKey"`
	CreatedAt   time.Time `json:"createdAt"`
}

type PostStats struct {
	Posts           int `json:"posts"`
	PublishedPosts  int `json:"publishedPosts"`
	Drafts          int `json:"drafts"`
	Comments        int `json:"comments"`
	PendingComments int `json:"pendingComments"`
}

type PostQuery struct {
	Search        string
	IncludeDrafts bool
	TermKind      string
	TermSlug      string
	CategorySlug  string
	TagSlug       string
	Order         string
	RestrictIDs   bool
	IDs           []string
	Page          int
	PageSize      int
}

type MonthCount struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

type PostPage struct {
	Items    []Post `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type PageQuery struct {
	Search        string
	IncludeDrafts bool
	Page          int
	PageSize      int
}

type PagePage struct {
	Items    []Page `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}

type CommentQuery struct {
	PostID             string
	Status             string
	IncludePending     bool
	PublishedPostsOnly bool
	Page               int
	PageSize           int
}

type CommentPage struct {
	Items    []Comment `json:"items"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
}

type MediaQuery struct {
	Page     int
	PageSize int
}

type MediaPage struct {
	Items    []Media `json:"items"`
	Total    int     `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
}

type AuditEvent struct {
	ID            string            `json:"id"`
	ActorUserID   string            `json:"actorUserId,omitempty"`
	ActorUsername string            `json:"actorUsername"`
	Action        string            `json:"action"`
	ResourceType  string            `json:"resourceType"`
	ResourceID    string            `json:"resourceId,omitempty"`
	Details       map[string]string `json:"details,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
}

type AuditQuery struct {
	Action       string
	ResourceType string
	Page         int
	PageSize     int
}

type AuditPage struct {
	Items    []AuditEvent `json:"items"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
}

type Repository interface {
	Initialized() bool
	Site() (*Site, error)
	Admin() (*Admin, error)
	UpdateAdmin(Admin) error
	Users() ([]User, error)
	User(string) (User, error)
	CreateUser(User) (User, error)
	UpdateUser(User) (User, error)
	DeleteUser(string) error
	Setup(Site, Admin) error
	UpdateSite(Site) error
	Posts() ([]Post, error)
	SearchPosts(PostQuery) (PostPage, error)
	ListPublishedMeta() ([]Post, error)
	Post(string) (Post, error)
	PostBySlug(string) (Post, error)
	SavePost(Post) (Post, error)
	DeletePost(string) error
	SearchPages(PageQuery) (PagePage, error)
	Page(string) (Page, error)
	PageBySlug(string) (Page, error)
	SavePage(Page) (Page, error)
	DeletePage(string) error
	ListTerms(TermQuery) (TermPage, error)
	SaveTerm(Term) (Term, error)
	DeleteTerm(string) error
	ListComments(CommentQuery) (CommentPage, error)
	Comment(string) (Comment, error)
	SaveComment(Comment) (Comment, error)
	UpdateCommentStatus(string, string) (Comment, error)
	DeleteComment(string) error
	ListMedia(MediaQuery) (MediaPage, error)
	Media(string) (Media, error)
	SaveMedia(Media) (Media, error)
	DeleteMedia(string) error
	RecordAudit(AuditEvent) error
	ListAudit(AuditQuery) (AuditPage, error)
	Stats() (PostStats, error)
	CreateSession(string, string, time.Time) error
	ResolveSession(string, time.Time) (string, error)
	DeleteSession(string) error
	DeleteUserSessions(string) error
	DeleteOtherUserSessions(string, string) error
	PurgeSessions(time.Time) error
	Close() error
}

type SQLiteRepository struct {
	db *sql.DB
}
