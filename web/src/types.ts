export interface Site {
  name: string
  url: string
  adminMail: string
  timezone: string
}

export interface Status {
  initialized: boolean
  version: string
  site?: Site
}

export interface User {
  username: string
  email: string
}

export interface Post {
  id: string
  title: string
  slug: string
  excerpt: string
  content: string
  status: 'draft' | 'published'
  publishedAt?: string
  updatedAt: string
}

export interface PostPage {
  items: Post[]
  total: number
  page: number
  pageSize: number
}

export interface Stats {
  posts: number
  publishedPosts: number
  drafts: number
}
