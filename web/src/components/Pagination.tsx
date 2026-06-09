import { Link } from 'react-router-dom'

interface Props {
  page: number
  totalPages: number
  sort?: string
  cat?: string
}

export default function Pagination({ page, totalPages, sort, cat }: Props) {
  if (totalPages <= 1) return null

  const href = (p: number) => {
    const sp = new URLSearchParams()
    if (p > 1) sp.set('page', String(p))
    if (sort && sort !== 'recent') sp.set('sort', sort)
    if (cat) sp.set('cat', cat)
    const qs = sp.toString()
    return qs ? `/?${qs}` : '/'
  }

  const pages: number[] = []
  for (let i = Math.max(1, page - 3); i <= Math.min(totalPages, page + 3); i++) {
    pages.push(i)
  }

  return (
    <div className="flex items-center justify-center gap-1.5 py-6 flex-wrap">
      {page > 1 && (
        <Link to={href(page - 1)} className="pag-link">prev</Link>
      )}
      {pages[0] > 1 && (
        <>
          <Link to={href(1)} className="pag-link">1</Link>
          {pages[0] > 2 && <span className="text-muted text-xs">…</span>}
        </>
      )}
      {pages.map(p => (
        p === page ? (
          <span key={p} className="pag-current">{p}</span>
        ) : (
          <Link key={p} to={href(p)} className="pag-link">{p}</Link>
        )
      ))}
      {pages[pages.length - 1] < totalPages && (
        <>
          {pages[pages.length - 1] < totalPages - 1 && <span className="text-muted text-xs">…</span>}
          <Link to={href(totalPages)} className="pag-link">{totalPages}</Link>
        </>
      )}
      {page < totalPages && (
        <Link to={href(page + 1)} className="pag-link">next</Link>
      )}
    </div>
  )
}
