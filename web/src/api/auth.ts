import { api, postJSON } from '../api'
import type { Status, User } from '../types'

export interface SetupInput {
  siteName: string
  siteUrl: string
  adminUsername: string
  adminPassword: string
  adminEmail: string
  timezone: string
}

export interface LoginInput {
  username: string
  password: string
}

export function getStatus() {
  return api<Status>('/api/v1/system/status')
}

export function getCurrentUser() {
  return api<{ user: User }>('/api/v1/auth/me')
}

export function login(input: LoginInput) {
  return postJSON<{ user: User }>('/api/v1/auth/login', input)
}

export function logout() {
  return api<void>('/api/v1/auth/logout', { method: 'POST' })
}

export function setup(input: SetupInput) {
  return postJSON('/api/v1/setup', input)
}
