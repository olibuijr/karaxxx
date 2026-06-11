import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { fetchChangelog } from '../api'
import type { ChangelogInfo } from '../types'

function cleanMarkdown(text: string): string {
  return text.replace(/\*\*/g, '').replace(/`/g, '')
}

function releaseLine(line: string) {
  const match = line.match(/^## \[(.+)]\s+.+\s+(.+)$/)
  if (!match) return null
  return { version: match[1], date: match[2] }
}

function renderLine(line: string, index: number) {
  const trimmed = line.trim()
  if (!trimmed || trimmed === '# Changelog') return null

  const release = releaseLine(trimmed)
  if (release) {
    return (
      <div key={index} className="mt-7 first:mt-0 flex flex-wrap items-baseline gap-3 border-t border-border pt-5 first:border-t-0 first:pt-0">
        <h2 className="text-xl font-bold text-text">Version {release.version}</h2>
        <span className="text-xs font-semibold uppercase text-muted">{release.date}</span>
      </div>
    )
  }

  if (trimmed.startsWith('### ')) {
    return <h3 key={index} className="mt-5 text-sm font-bold uppercase tracking-wide text-orange">{cleanMarkdown(trimmed.slice(4))}</h3>
  }

  if (trimmed.startsWith('- ')) {
    return (
      <div key={index} className="flex gap-3 text-sm leading-6 text-muted">
        <span className="mt-2 h-1.5 w-1.5 flex-shrink-0 rounded-full bg-orange/70" aria-hidden="true" />
        <span>{cleanMarkdown(trimmed.slice(2))}</span>
      </div>
    )
  }

  return <p key={index} className="text-sm leading-6 text-muted">{cleanMarkdown(trimmed)}</p>
}

export default function Changelog() {
  const [info, setInfo] = useState<ChangelogInfo | null>(null)
  const [error, setError] = useState('')
  const navigate = useNavigate()

  useEffect(() => {
    fetchChangelog()
      .then(setInfo)
      .catch(() => setError('Could not load changelog.'))
  }, [])

  const lines = useMemo(() => {
    if (!info?.markdown) return []
    return info.markdown.split('\n').slice(0, 120)
  }, [info])

  return (
    <div className="mx-auto max-w-4xl space-y-6 p-4 md:p-8">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-wide text-orange">KaraXXX - Adult Playground</p>
          <h1 className="mt-2 text-2xl font-bold text-text">Changelog</h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-muted">
            User-visible release notes from the deploy pipeline. Current version: {info?.version ? `v${info.version}` : 'loading...'}.
          </p>
        </div>
        <button onClick={() => navigate(-1)} className="rounded-full border border-border px-3 py-1.5 text-sm text-muted hover:text-text">
          Back
        </button>
      </header>

      <section className="rounded-lg border border-border bg-card/80 p-4 md:p-6">
        {error && <p className="text-sm text-red">{error}</p>}
        {!error && !info && <p className="text-sm text-muted">Loading release notes...</p>}
        {info && <div className="space-y-2">{lines.map(renderLine)}</div>}
      </section>
    </div>
  )
}
