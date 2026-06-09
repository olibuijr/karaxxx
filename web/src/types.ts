export interface Video {
  id: string
  slug: string
  title: string
  description: string
  categories: string[]
  tags: string[]
  uploader: string
  upload_date: string
  duration: number
  views: number
  added_at: string
  source: string
  thumb_uuid: string
  url_360: string
  url_720: string
  url_1080: string
  preview_url: string
  hls_url: string
}

export interface BrowseParams {
  page?: number
  sort?: 'recent' | 'new' | 'views' | 'duration' | 'trending'
  cat?: string
  q?: string
  uploader?: string
  source?: string
}

export interface TagCount {
  name: string
  count: number
}

export interface BrowseResponse {
  videos: Video[]
  count: number
  page: number
  total_pages: number
}

export interface CrawlProgress {
  status: string
  source: string
  scanned: number
  new_videos: number
  cached: number
  detail_done: number
  detail_total: number
  page: number
  total_count: number
  source_counts: Record<string, number>
}
