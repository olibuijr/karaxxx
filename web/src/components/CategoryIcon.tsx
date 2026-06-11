const iconMap: Record<string, string> = {
  anal: 'M12 4c4 0 7 3 7 7 0 5-7 9-7 9s-7-4-7-9c0-4 3-7 7-7Zm0 4a3 3 0 1 0 0 6 3 3 0 0 0 0-6Z',
  teen: 'M12 3l2.5 5 5.5.8-4 4 .9 5.7-4.9-2.8-4.9 2.8.9-5.7-4-4 5.5-.8L12 3Z',
  milf: 'M12 4c2.8 0 5 2.2 5 5 0 4.2-5 8-5 8S7 13.2 7 9c0-2.8 2.2-5 5-5Zm-6 15h12',
  blowjob: 'M5 12c2.5-4 6-6 10.5-6 2 0 3.5 1.5 3.5 3.5S17.5 13 15.5 13H13l2.5 5H9l-4-6Z',
  homemade: 'M4 11 12 4l8 7v8H6v-8Zm5 8v-5h6v5',
  lesbian: 'M9 5a4 4 0 0 1 3 6.7V14h2v2h-2v3h-2v-3H8v-2h2v-2.3A4 4 0 0 1 9 5Zm6 2a4 4 0 0 1 0 8',
  bbc: 'M6 6h12v12H6z M9 9h6v6H9z',
  latina: 'M12 3c4 3 7 6 7 10a7 7 0 0 1-14 0c0-4 3-7 7-10Z',
  asian: 'M4 12c2-4 5-6 8-6s6 2 8 6c-2 4-5 6-8 6s-6-2-8-6Zm4 0h8',
  group: 'M8 11a3 3 0 1 1 0-6 3 3 0 0 1 0 6Zm8 0a3 3 0 1 1 0-6 3 3 0 0 1 0 6ZM3 20c.8-4 3-6 5-6s4.2 2 5 6m-2 0c.7-3 2.5-5 5-5s4.3 2 5 5',
  outdoor: 'M4 18 10 6l4 8 2-4 4 8H4Z',
  bdsm: 'M7 7a5 5 0 0 1 10 0v3H7V7Zm-1 3h12v10H6V10Z',
  cosplay: 'M12 3 4 8l8 5 8-5-8-5Zm-6 9 6 4 6-4v5l-6 4-6-4v-5Z',
  massage: 'M5 15c4-6 10-6 14 0M7 18h10M8 9h8',
  transgender: 'M12 5a5 5 0 1 0 0 10 5 5 0 0 0 0-10Zm4-1h4v4m0-4-4 4',
  solo: 'M12 4a4 4 0 0 1 4 4c0 2.4-1.6 4-4 4s-4-1.6-4-4a4 4 0 0 1 4-4Zm-6 16c1-4 3-6 6-6s5 2 6 6',
}

const fallbackPath = 'M12 3c4.5 0 8 3.2 8 7.5C20 16 12 21 12 21S4 16 4 10.5C4 6.2 7.5 3 12 3Zm0 4a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7Z'

export default function CategoryIcon({ category, className = 'h-4 w-4' }: { category?: string; className?: string }) {
  const key = (category || '').toLowerCase()
  const path = iconMap[key] || fallbackPath
  return (
    <svg viewBox="0 0 24 24" className={className} fill="currentColor" aria-hidden="true">
      <path d={path} />
    </svg>
  )
}
