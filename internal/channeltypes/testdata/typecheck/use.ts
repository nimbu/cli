import type { BlogPosts, Site } from './nimbu-channels'
import type { NimbuChannels } from 'nimbu-js-sdk/cloud'

// NimbuChannels['site'] must be the channel type, not the cloud Site export.
const siteCheck: NimbuChannels['site'] = { tagline: 'x' } satisfies Site
void siteCheck

Nimbu.Cloud.after('channel.entries.created', 'blog-posts', (req) => {
  const entry = req.object
  const title: string = entry.get('title')
  const status: 'draft' | 'published' | "it's live" = entry.get('status')
  const author = entry.get('author')
  const authorName: string = author.get('name')
  const owner = entry.get('owner')
  if (owner && typeof owner !== 'string') {
    const email: unknown = owner.email
    void email
  }
  const reviewer = entry.get('reviewer')
  const reviewerEmail = reviewer?.get('email')
  const body = entry.get('body')
  // @ts-expect-error optional fields can be null
  const len: number = body.length
  const published: boolean | undefined = entry.get('published')
  const at = entry.get('published_at')
  const year: number | undefined = at?.getFullYear()
  const related = entry.get('related')
  void [title, status, authorName, reviewerEmail, len, published, year, related]
})

Nimbu.Cloud.before('channel.entries.updated', 'site', (req) => {
  const tagline: string = req.object.get('tagline')
  // @ts-expect-error unknown field on a typed channel
  req.object.get('nope')
  void tagline
})

// The golden module covers the whole site (strictChannels), so a slug that is
// not one of its channels is a type error.
// @ts-expect-error unknown channel slug
Nimbu.Cloud.after('channel.entries.created', 'unknown-channel', () => {})
