import type {
  DateTime,
  ISODate,
  JSONField,
  MultiSelect,
  NimbuCustomer,
  NimbuFile,
  NimbuGallery,
  ReferenceMany,
  ReferenceTo,
  Select,
} from 'nimbu-js-sdk'

/**
 * Blog posts (channel `blog-posts`)
 *
 * Articles shown on the blog.
 * One entry per post.
 */
export type BlogPosts = {
  title: string
  body?: string | null
  /** Contact e-mail */
  contact?: string | null
  published?: boolean
  views?: number | null
  rating?: number | null
  publish_on?: ISODate | null
  published_at?: DateTime | null
  /**
   * Local time of day.
   * Shown on the event card.
   */
  starts_at?: DateTime | ISODate | null
  status: Select<'draft' | 'published' | 'it\'s live'>
  tags?: MultiSelect<'news' | 'a\\b'> | null
  mood?: string | null
  labels?: string[] | null
  author: ReferenceTo
  /** Related posts */
  related?: ReferenceMany<BlogPosts> | null
  legacy?: ReferenceTo | null
  owner?: JSONField | string | null
  reviewer?: NimbuCustomer | null
  /** Required when `published == true`. */
  venue?: string | null
  /** Cover *\/ image */
  cover?: NimbuFile | null
  photos?: NimbuGallery | null
  metadata?: JSONField | null
  location?: JSONField | null
  word_count?: number | null
  summary?: string | null
  score?: string | number | null
  future?: any
}
