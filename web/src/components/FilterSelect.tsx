import { useEffect, useState, useRef } from 'react'
import { Link } from 'react-router-dom'

export default function FilterSelect({
  options,
  current,
  getHref,
  onOptionClick,
}: {
  options: { label: string; value: string }[]
  current: string
  getHref: (v: string) => string
  onOptionClick?: () => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(o => !o)}
        className="w-full flex items-center justify-between px-3 py-2 rounded-md text-sm font-medium
                   bg-white/5 text-text hover:bg-white/10 transition-colors capitalize"
      >
        <span>{options.find(o => o.value === current)?.label || options[0].label}</span>
        <svg className={`w-3.5 h-3.5 text-muted transition-transform ${open ? 'rotate-180' : ''}`}
          viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <polyline points="6 9 12 15 18 9" />
        </svg>
      </button>
      {open && (
        <div className="absolute left-0 right-0 top-full mt-1 z-50 py-1 rounded-lg
                        bg-card border border-border shadow-card">
          {options.map(o => (
            <Link
              key={o.value}
              to={getHref(o.value)}
              onClick={() => { setOpen(false); onOptionClick?.() }}
              className={`block px-3 py-1.5 text-sm font-medium transition-colors
                          ${o.value === current
                            ? 'text-orange bg-orange/10'
                            : 'text-muted hover:text-text hover:bg-white/5'
                          }`}
            >
              {o.label}
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
